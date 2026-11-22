#!/usr/bin/env python3
"""Branch-coverage tests for uncovered decision paths in guard_ops_commands.py."""

from __future__ import annotations

import importlib.util
import pathlib
import sys
from unittest.mock import patch

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
GUARD_PATH = REPO_ROOT / "scripts" / "agent-harness" / "guard_ops_commands.py"

spec = importlib.util.spec_from_file_location("guard_ops_commands", GUARD_PATH)
if spec is None or spec.loader is None:
    msg = f"cannot load {GUARD_PATH}"
    raise RuntimeError(msg)
guard = importlib.util.module_from_spec(spec)
with patch.dict(sys.modules, {"guard_ops_commands": guard}):
    spec.loader.exec_module(guard)
Token = guard.Token


def _reference_split_shell_segments(command: str) -> list[list[Token]]:  # noqa: PLR0912, PLR0915 - frozen reference copy of the pre-refactor scanner
    """Tokenize a shell command and split it on ; && || | & ( ) and newlines."""
    segments: list[list[Token]] = []
    current_seg: list[Token] = []
    buf: list[str] = []
    has_token = False
    has_quote = False
    state = "NORMAL"
    i = 0
    n = len(command)

    def flush_token() -> None:
        nonlocal has_token, has_quote
        if has_token:
            current_seg.append(Token("".join(buf), has_quote))
            buf.clear()
            has_token = False
            has_quote = False

    def flush_segment() -> None:
        flush_token()
        if current_seg:
            segments.append(list(current_seg))
            current_seg.clear()

    while i < n:
        ch = command[i]
        if state == "SINGLE":
            if ch == "'":
                state = "NORMAL"
            else:
                buf.append(ch)
            i += 1
            continue

        if state == "DOUBLE":
            if ch == "\\":
                if i + 1 < n and command[i + 1] == "\n":
                    i += 2
                    continue
                if i + 1 < n:
                    buf.append(command[i + 1])
                    i += 2
                    continue
            elif ch == '"':
                state = "NORMAL"
                i += 1
                continue
            buf.append(ch)
            i += 1
            continue

        # NORMAL state
        if ch == "'":
            state = "SINGLE"
            has_token = True
            has_quote = True
            i += 1
        elif ch == '"':
            state = "DOUBLE"
            has_token = True
            has_quote = True
            i += 1
        elif ch == "\\":
            if i + 1 < n and command[i + 1] == "\n":
                i += 2
            elif i + 1 < n:
                buf.append(command[i + 1])
                has_token = True
                i += 2
            else:
                i += 1
        elif ch == "#" and not has_token:
            while i < n and command[i] != "\n":
                i += 1
        elif ch in ("\n", ";", "(", ")"):
            flush_segment()
            i += 1
        elif ch in ("&", "|"):
            flush_segment()
            if i + 1 < n and command[i + 1] == ch:
                i += 2
            else:
                i += 1
        elif ch.isspace():
            flush_token()
            i += 1
        else:
            buf.append(ch)
            has_token = True
            i += 1

    flush_segment()
    return segments


def _reference_extract_substitutions(command: str) -> list[str]:  # noqa: PLR0912, PLR0915 - frozen reference copy of the pre-refactor scanner
    """Extract inner command strings from $(...) and `...` outside single quotes."""
    subs: list[str] = []
    i = 0
    n = len(command)
    in_double = False

    while i < n:
        ch = command[i]
        if ch == "\\" and i + 1 < n:
            i += 2
            continue
        if not in_double and ch == "'":
            i += 1
            while i < n and command[i] != "'":
                i += 1
            i += 1
            continue
        if ch == '"':
            in_double = not in_double
            i += 1
            continue
        if ch == "$" and i + 1 < n and command[i + 1] == "(":
            start = i + 2
            j = start
            depth = 1
            sub_state = "NORMAL"
            while j < n and depth > 0:
                c = command[j]
                if sub_state == "SINGLE":
                    if c == "'":
                        sub_state = "NORMAL"
                    j += 1
                    continue
                if sub_state == "DOUBLE":
                    if c == "\\" and j + 1 < n:
                        j += 2
                        continue
                    if c == '"':
                        sub_state = "NORMAL"
                    j += 1
                    continue
                if c == "\\" and j + 1 < n:
                    j += 2
                    continue
                if c == "'":
                    sub_state = "SINGLE"
                elif c == '"':
                    sub_state = "DOUBLE"
                elif c == "(":
                    depth += 1
                elif c == ")":
                    depth -= 1
                    if depth == 0:
                        break
                j += 1
            subs.append(command[start:j])
            i = j + 1
            continue
        if ch == "`":
            start = i + 1
            j = start
            while j < n:
                if command[j] == "\\" and j + 1 < n:
                    j += 2
                    continue
                if command[j] == "`":
                    break
                j += 1
            subs.append(command[start:j])
            i = j + 1
            continue
        i += 1

    return subs


def check(counts: list[int], name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        counts[0] += 1
    else:
        suffix = f" ({detail})" if detail else ""
        sys.stdout.write(f"  FAIL  {name}{suffix}\n")
        counts[1] += 1


# _split_shell_segments branches
CASES: list[tuple[str, str, str | None]] = [
    # DOUBLE state escapes and line continuations
    ("double quote escaped quote", 'sh -c "docker rm \\"x\\""', "O2"),
    ("double quote line continuation", 'sh -c "docker rm \\\nx"', "O2"),
    # NORMAL state backslash escapes, line continuations, trailing backslash
    ("normal backslash line continuation", "docker \\\nrm x", "O2"),
    ("normal escaped space in arg", "docker rm my\\ container", "O2"),
    (
        "normal escaped semicolon stops split (today verdict: O2 via token scan)",
        "echo a\\; docker rm x",
        "O2",
    ),
    ("normal trailing backslash", "docker ps\\", None),
    # NORMAL state comments and mid-token '#'
    ("comment line skips rm then allows ps", "# docker rm x\ndocker ps", None),
    ("comment line followed by rm asks O2", "# benign comment\ndocker rm x", "O2"),
    ("mid-token hash preserves token", "docker rm foo#1", "O2"),
    # NORMAL state subshell parens
    ("subshell parens split docker rm", "(docker rm x)", "O2"),
    # _extract_substitutions branches
    (
        "escaped dollar-paren still split by parens (today verdict: O2)",
        "echo \\$(docker rm x)",
        "O2",
    ),
    ("single-quoted dollar-paren is not extracted", "echo '$(docker rm x)'", None),
    ("single quote inside $(...) protects inner quotes", "echo $(sh -c 'docker rm x')", "O2"),
    ("double quote + escape inside $(...)", 'echo $(sh -c "docker rm \\"x\\"")', "O2"),
    ("backslash escape inside $(...)", "echo $(echo \\) && docker rm x)", "O2"),
    ("nested parens inside $(...)", "echo $((1+1)) && echo $( (docker rm x) )", "O2"),
    ("backtick substitution", "echo `docker rm x`", "O2"),
    ("escaped backtick inside backticks", "echo `echo \\` && docker rm x`", "O2"),
    # _classify_compose branches
    ("compose -- stops before subcmd (today verdict: allow)", "docker compose -- up -d", None),
    ("compose -- after up hides --build (today verdict: O3)", "docker compose up -- --build", "O3"),
    ("compose dangling -p falls back to default (today verdict: O3)", "docker compose up -p", "O3"),
    ("compose -p=alt-test allows", "docker compose -p=alt-test up -d", None),
    ("compose -p=alt asks O3", "docker compose -p=alt up -d", "O3"),
    ("compose empty -p='' falls back to default O3", 'docker compose -p "" up -d', "O3"),
    (
        "compose empty -p='' falls back to env staging (today verdict: allow)",
        'COMPOSE_PROJECT_NAME=staging docker compose -p "" up -d',
        None,
    ),
    ("compose down --build is O3 not O4", "docker compose -p alt down --build", "O3"),
    # _classify_docker_segment branches
    ("docker -- stops before subcmd (today verdict: allow)", "docker -- rm x", None),
    ("docker boolean global flag before rm", "docker --debug rm x", "O2"),
    ("bare docker without subcmd allows", "docker", None),
    ("docker --help without subcmd allows", "docker --help", None),
    (
        "docker container -- stops before action (today verdict: allow)",
        "docker container -- rm x",
        None,
    ),
    ("docker container flag before action", "docker container --help", None),
    ("docker container prune asks O1", "docker container prune -f", "O1"),
    ("docker volume prune asks O1", "docker volume prune -f", "O1"),
    ("docker network remove asks O1", "docker network remove alt-net", "O1"),
    ("docker container kill asks O2", "docker container kill alt-backend", "O2"),
]

# Command strings statically collected from scripts/tests/test-agent-harness.py
HARNESS_TEST_COMMANDS: list[str] = [
    "docker --context default system prune -af",
    "docker ps && sudo docker image prune -a -f",
    "docker volume rm old_certs; docker rmi alt-backend:old",
    "DOCKER_HOST=unix:///var/run/docker.sock docker image rm myimg",
    "docker network prune -f",
    "docker builder prune --all",
    "docker network rm alt-net",
    "docker image remove myimg:old",
    "docker volume remove old_certs",
    "docker buildx prune -f",
    'sh -c "docker system prune -af"',
    "docker -l debug rm -f pki-agent",
    "echo checking && docker container stop -t 5 alt-backend",
    "sudo -E docker kill alt-data-hub",
    "docker container rm alt-backend",
    "docker container remove alt-backend",
    "docker compose -f compose/compose.yaml -p alt stop alt-backend",
    "docker compose kill alt-backend",
    "for c in $(docker ps -aq --filter name=alt-staging-); do docker rm -f $c; done",
    "while read c; do docker stop $c; done < ids.txt",
    "docker ps -q --filter name=alt-staging- | xargs -r docker rm -f",
    "xargs docker rm",
    "{ docker rm x; }",
    'echo "$(docker rm x)"',
    "sudo -iu root docker rm x",
    "docker compose -f compose/compose.yaml up -d alt-backend",
    "docker compose -f compose/compose.yaml -p alt down",
    "COMPOSE_PROJECT_NAME=alt docker-compose rm -f",
    "git status || docker compose --project-name alt up -d",
    "if true; then docker compose -p alt down; fi",
    "bash -c 'docker compose -p alt down'",
    "time docker compose -p alt down",
    "docker compose -p $PROJ down",
    'docker compose -p "$COMPOSE_PROJECT_NAME" up -d',
    "docker compose up --build -d alt-backend",
    "docker compose -f compose/compose.yaml -p alt up -d --no-deps --force-recreate alt-backend",
    "echo start\nCOMPOSE_PROJECT_NAME=alt docker-compose -f compose/compose.yaml up --build -d",
    "timeout 300 docker compose -f compose/compose.yaml -p alt up -d --build alt-backend",
    "nohup docker compose -p alt up -d --build &",
    "docker ps -a",
    "docker logs --tail 50 alt-backend",
    "docker inspect alt-backend",
    "docker images",
    "docker version",
    "docker exec alt-backend /alt-backend healthcheck",
    "docker compose -f compose/compose.yaml config",
    "docker compose -f compose/compose.yaml -p alt ps",
    "docker compose -p alt logs -f alt-backend",
    "docker compose -f compose/compose.yaml -p alt-test up --build -d",
    "docker compose -f compose/compose.yaml up --build -d -p alt-test",
    "docker compose --project-name=e2e down",
    "COMPOSE_PROJECT_NAME=staging docker compose up --force-recreate -d",
    "COMPOSE_PROJECT_NAME=alt docker compose -p test-proj up -d",
    "docker compose -p alt-test stop alt-backend",
    'echo "docker system prune -af && docker compose up --build"',
    "git commit -m 'PM-2026-031: never run docker compose up --build -d alt-backend'",
    "",
    "   \n  ",
    "docker system prune -af",
    "docker rm -f alt-backend",
    "docker compose -p alt down",
    "docker compose -p alt up --build -d alt-backend",
    "echo hi",
    "a; docker rm x",
]

ADVERSARIAL_CORPUS: list[str] = [
    # Unbalanced quotes
    "docker rm 'unclosed",
    'docker rm "unclosed',
    "'",
    '"',
    "'unclosed\\",
    '"unclosed\\',
    'echo "a\\" && docker rm x',
    "echo 'a\\' && docker rm x",
    "docker 'rm' \"x\"",
    "''",
    '""',
    "'a''b'",
    '"a""b"',
    "'a'b\"c\"",
    # Quote opened inside $(...)
    'echo $(docker rm "unclosed)',
    "echo $(docker rm 'unclosed)",
    'echo $(echo ")" && docker rm x)',
    "echo $(echo ')' && docker rm x)",
    'echo $(echo "\\")"; docker rm x)',
    'echo "$(echo "x")"',
    "echo \"$(echo 'x')\"",
    "echo '$(echo \"x\")'",
    "echo $(sh -c 'echo )' && docker rm x)",
    'echo $(sh -c "echo )" && docker rm x)',
    # Nested $( $( ) ) and arithmetic/subshell parens
    "echo $(echo $(docker rm x))",
    "echo $( $(docker rm x) )",
    "echo $(a $(b $(c)))",
    "echo $(unclosed $(nested)",
    "echo $((1 + (2 * 3)))",
    "echo $(docker ps) $(docker rm x)",
    '"$(echo $(docker rm x))"',
    "$()",
    "$( )",
    "$(docker rm x",
    "$",
    "$ ",
    "$$",
    'echo "$"',
    'echo "$$(docker rm x)"',
    # Backticks inside double quotes and elsewhere
    'echo "`docker rm x`"',
    'echo "`echo \\`docker rm x\\``"',
    'echo "`unclosed backtick"',
    "echo '`docker rm x`'",
    "echo \\`docker rm x\\`",
    "echo ``",
    "echo `docker rm x` `docker stop y`",
    'echo "$(echo `docker rm x`)"',
    "`docker rm x",
    "echo `docker rm x\\",
    # Backslash at end of input and line continuations
    "\\",
    "docker\\",
    '"\\"',
    '"abc\\"',
    "'abc\\'",
    "docker \\\n\\\nrm x",
    'sh -c "docker \\\n\\\nrm x"',
    "docker \\ ",
    "\\$",
    "\\$(docker rm x)",
    'echo "\\$(docker rm x)"',
    'echo "\\\\$(docker rm x)"',
    "echo $(docker rm x\\",
    "echo a\\nb",
    'echo "a\\nb"',
    # '#' after whitespace vs mid-token vs tabs vs quotes
    "# only comment",
    "   # indented comment\ndocker rm x",
    "\t# tab before hash\ndocker rm x",
    "docker ps # inline comment; docker rm x",
    "docker ps;#comment after semicolon",
    "docker rm foo#bar",
    "''#comment",
    '""#comment',
    "# comment without newline",
    "# comment with \\\ndocker rm x",
    "echo a #comment\n#second\ndocker ps",
    # '&' vs '&&', '|' vs '||', ';', '(', ')'
    "docker ps & docker rm x",
    "docker ps && docker rm x",
    "docker ps &&& docker rm x",
    "docker ps | docker rm x",
    "docker ps || docker rm x",
    "docker ps ||| docker rm x",
    "docker ps ;;; docker rm x",
    "((docker rm x))",
    "( ( docker rm x ) )",
    ")(",
    ";&|()",
    "&",
    "&&",
    "|",
    "||",
    ";",
    # Empty, whitespace, CRLF
    " ",
    "\t\n\r\v\f",
    "docker ps\r\ndocker rm x\r\n",
    "docker \\\r\nrm x",
    "# comment\r\ndocker rm x",
    'sh -c "docker rm \\\r\nx"',
]

DIFFERENTIAL_CORPUS: list[str] = list(
    dict.fromkeys([cmd for _, cmd, _ in CASES] + HARNESS_TEST_COMMANDS + ADVERSARIAL_CORPUS)
)


def _check_differential_parity(counts: list[int]) -> None:
    mismatches: list[str] = []
    for idx, cmd in enumerate(DIFFERENTIAL_CORPUS):
        ref_segs = _reference_split_shell_segments(cmd)
        live_segs = guard._split_shell_segments(cmd)
        if ref_segs != live_segs:
            mismatches.append(f"[{idx}] split mismatch for {cmd!r}: {ref_segs!r} != {live_segs!r}")
        ref_subs = _reference_extract_substitutions(cmd)
        live_subs = guard._extract_substitutions(cmd)
        if ref_subs != live_subs:
            mismatches.append(f"[{idx}] subs mismatch for {cmd!r}: {ref_subs!r} != {live_subs!r}")
    detail = mismatches[0] if mismatches else ""
    check(
        counts,
        f"differential scanner parity ({len(DIFFERENTIAL_CORPUS)} strings)",
        not mismatches,
        detail,
    )


def main() -> int:
    counts = [0, 0]
    sys.stdout.write("guard_ops_commands uncovered branch suite\n")
    for label, cmd, expected in CASES:
        got = guard.classify_command(cmd)
        check(counts, label, got == expected, f"cmd={cmd!r} expected={expected!r} got={got!r}")

    # Direct non-docker invocation to _classify_docker_segment
    got_non_docker = guard._classify_docker_segment(["podman", "rm", "x"], {})
    check(counts, "non-docker exe returns None", got_non_docker is None, f"got={got_non_docker!r}")

    _check_differential_parity(counts)

    sys.stdout.write(f"\n{counts[0]} passed, {counts[1]} failed\n")
    return 1 if counts[1] else 0


if __name__ == "__main__":
    sys.exit(main())
