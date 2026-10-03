#!/usr/bin/env python3
"""Report host .env overrides that defeat a production compose default.

compose resolves `${VAR:-default}` from the host's .env, so a value that was
right before a compose change keeps beating the fixed default afterwards and
nothing in compose notices. This applies the env rules in
deploy/host-prereqs.yaml (retired_env, dead_env_values, env_allowlists) to
one or more env files.

Only key names and rule ids are printed: the files this reads are the
production ones, and their values include credentials.

Usage:
    python3 scripts/check-env-overrides.py --env-file /path/to/.env [--env-file ...]

Exit 0 when no override is dead (warnings allowed), 1 when one is, 2 when an
env file or the manifest cannot be read.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path
from typing import NamedTuple

try:
    import yaml
except ImportError:  # pragma: no cover - CI and the deploy host install it
    sys.stderr.write("check-env-overrides: PyYAML is required (pip install pyyaml)\n")
    raise SystemExit(2)

MANIFEST_PATH = Path(__file__).resolve().parent.parent / "deploy" / "host-prereqs.yaml"

ANY_KEY = "*"
ALLOWLIST_MISSING = "allowlist-missing-peer"
ALLOWLIST_EXTRA = "allowlist-extra-peer"
ALLOWLIST_REASONS = {
    ALLOWLIST_MISSING: "the override drops a peer the compose default admits, so that caller's mTLS requests are refused",
    ALLOWLIST_EXTRA: "the override admits a peer the compose default does not; unset it unless the widening is deliberate",
}
MATCHERS = ("deny_hosts", "require_hosts", "require_schemes")

_KEY = re.compile(r"[A-Za-z_][A-Za-z0-9_.-]*")
_ENDPOINT = re.compile(
    r"^(?:(?P<scheme>[A-Za-z][A-Za-z0-9+.-]*)://)?"
    r"(?:[^@/?#]*@)?"
    r"(?P<host>\[[^\]]*\]|[^:/?#@]+)"
    r"(?::(?P<port>[0-9]+))?"
)


class Finding(NamedTuple):
    severity: str
    key: str
    rule: str


class Rules(NamedTuple):
    retired: tuple[dict, ...]
    dead: tuple[dict, ...]
    allowlists: dict[str, tuple[str, ...]]

    def rule_ids(self) -> list[str]:
        ids = [r["id"] for r in self.retired] + [r["id"] for r in self.dead]
        return ids + ([ALLOWLIST_MISSING, ALLOWLIST_EXTRA] if self.allowlists else [])

    def reason(self, rule_id: str) -> str:
        for rule in (*self.retired, *self.dead):
            if rule["id"] == rule_id:
                return rule["reason"]
        return ALLOWLIST_REASONS.get(rule_id, "")

    def keys_for(self, rule_id: str) -> list[str]:
        return next((list(r["keys"]) for r in (*self.retired, *self.dead) if r["id"] == rule_id), [])

    def retired_keys(self) -> list[str]:
        return [key for rule in self.retired for key in rule["keys"]]

    def named_dead_value_keys(self) -> set[str]:
        return {key for rule in self.dead for key in rule["keys"] if key != ANY_KEY}


def _str_list(value) -> bool:
    return isinstance(value, list) and bool(value) and all(isinstance(v, str) and v for v in value)


def load_rules(manifest_path: Path = MANIFEST_PATH) -> Rules:
    """Read and validate the env sections of the host prerequisite manifest."""
    manifest = yaml.safe_load(manifest_path.read_text(encoding="utf-8")) or {}
    retired = manifest.get("retired_env") or []
    dead = manifest.get("dead_env_values") or []
    for rule in (*retired, *dead):
        label = rule.get("id") if isinstance(rule, dict) else rule
        if not isinstance(rule, dict) or not isinstance(rule.get("id"), str):
            raise ValueError(f"env rule {label!r}: needs a string id")
        if not _str_list(rule.get("keys")):
            raise ValueError(f"env rule {label}: keys must be a non-empty list of names")
        if not isinstance(rule.get("reason"), str) or not rule["reason"].strip():
            raise ValueError(f"env rule {label}: reason is empty")
    for rule in dead:
        matchers = [m for m in MATCHERS if m in rule]
        if len(matchers) != 1 or not _str_list(rule[matchers[0]]):
            raise ValueError(f"dead_env_values {rule['id']}: needs exactly one of {', '.join(MATCHERS)} as a list")
    allowlists: dict[str, tuple[str, ...]] = {}
    for entry in manifest.get("env_allowlists") or []:
        if not isinstance(entry, dict) or not isinstance(entry.get("key"), str) or not _str_list(entry.get("required")):
            raise ValueError(f"env_allowlists entry {entry!r}: needs key and a non-empty required list")
        allowlists[entry["key"]] = tuple(entry["required"])
    return Rules(tuple(retired), tuple(dead), allowlists)


def parse_env_file(text: str) -> dict[str, str]:
    """KEY=value pairs the way compose's dotenv reader sees them."""
    out: dict[str, str] = {}
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[len("export "):].lstrip()
        key, sep, value = line.partition("=")
        key = key.strip()
        if not sep or not _KEY.fullmatch(key):
            continue
        value = value.strip()
        if value and value[0] in "\"'":
            end = value.find(value[0], 1)
            value = value[1:end] if end > 0 else value[1:]
        else:
            value = re.split(r"\s+#", value, maxsplit=1)[0].rstrip()
        out[key] = value
    return out


def _endpoints(value: str) -> list[tuple[str | None, str, str | None]]:
    found = []
    for token in re.split(r"[\s,]+", value):
        m = _ENDPOINT.match(token) if token else None
        if m:
            scheme = m.group("scheme")
            found.append((scheme.lower() if scheme else None, m.group("host").lower(), m.group("port")))
    return found


def _is_dead(value: str, rule: dict) -> bool:
    endpoints = _endpoints(value)
    if "deny_hosts" in rule:
        deny = {h.lower() for h in rule["deny_hosts"]}
        return any(host in deny or (port and f"{host}:{port}" in deny) for _, host, port in endpoints)
    if "require_hosts" in rule:
        allowed = {h.lower() for h in rule["require_hosts"]}
        return not endpoints or any(host not in allowed for _, host, _ in endpoints)
    allowed = {s.lower() for s in rule["require_schemes"]}
    return not endpoints or any(scheme not in allowed for scheme, _, _ in endpoints)


def evaluate(env: dict[str, str], rules: Rules) -> list[Finding]:
    found: list[Finding] = []
    for key, value in env.items():
        found.extend(Finding("warning", key, r["id"]) for r in rules.retired if key in r["keys"])
        # `${VAR:-default}` treats an empty value as unset: the default wins.
        if value == "":
            continue
        found.extend(
            Finding("error", key, r["id"])
            for r in rules.dead
            if (ANY_KEY in r["keys"] or key in r["keys"]) and _is_dead(value, r)
        )
        required = rules.allowlists.get(key)
        if required is not None:
            peers = {p.strip() for p in value.split(",") if p.strip()}
            if set(required) - peers:
                found.append(Finding("error", key, ALLOWLIST_MISSING))
            if peers - set(required):
                found.append(Finding("warning", key, ALLOWLIST_EXTRA))
    return sorted(found)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--env-file", action="append", required=True, type=Path, help="host env file (repeatable)")
    parser.add_argument("--manifest", type=Path, default=MANIFEST_PATH, help=argparse.SUPPRESS)
    args = parser.parse_args(argv)

    try:
        rules = load_rules(args.manifest)
    except (OSError, ValueError, yaml.YAMLError) as exc:
        print(f"check-env-overrides: cannot load rules from {args.manifest}: {exc}", file=sys.stderr)
        return 2

    errors = warnings = 0
    triggered: set[str] = set()
    for path in args.env_file:
        try:
            env = parse_env_file(path.read_text(encoding="utf-8"))
        except OSError as exc:
            print(f"check-env-overrides: cannot read {path}: {exc.strerror}", file=sys.stderr)
            return 2
        except UnicodeDecodeError:
            print(f"check-env-overrides: {path} is not UTF-8 text", file=sys.stderr)
            return 2
        findings = evaluate(env, rules)
        print(f"{path}: {len(env)} keys checked")
        for finding in findings:
            print(f"  {finding.severity:<7}  {finding.key}  {finding.rule}")
            triggered.add(finding.rule)
        errors += sum(f.severity == "error" for f in findings)
        warnings += sum(f.severity == "warning" for f in findings)

    if triggered:
        print("\nrules (deploy/host-prereqs.yaml):")
        for rule_id in sorted(triggered):
            print(f"  {rule_id}: {' '.join(rules.reason(rule_id).split())}")
        print("\nDelete each listed key so compose's own value applies, or correct it deliberately.")
    print(f"\n{errors} error(s), {warnings} warning(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
