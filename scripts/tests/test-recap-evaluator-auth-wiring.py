#!/usr/bin/env python3
"""recap-evaluator bearer-auth wiring contract.

The evaluator refuses to start unless /run/secrets/evaluator_api_token is
mounted (EVALUATOR_AUTH=disabled is the only opt-out). Pin the secret
declaration, the evaluator's mount, the absence of a production opt-out, and
that every compose service pointed at the evaluator carries the same token.
Static parse only; never starts Compose and never reads secret bytes.

Run:
    python3 scripts/tests/test-recap-evaluator-auth-wiring.py
"""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))

from compose_include import load_yaml, production_compose_files  # noqa: E402

SECRET = "evaluator_api_token"
SECRET_PATH = f"/run/secrets/{SECRET}"
TOKEN_ENV = "EVALUATOR_API_TOKEN_FILE"
EVALUATOR = "recap-evaluator"


def environment(service: dict) -> dict[str, str]:
    raw = service.get("environment") or {}
    if isinstance(raw, dict):
        return {str(k): str(v) for k, v in raw.items() if v is not None}
    return dict(item.split("=", 1) for item in raw if isinstance(item, str) and "=" in item)


def secret_names(service: dict) -> set[str]:
    names: set[str] = set()
    for item in service.get("secrets") or []:
        if isinstance(item, str):
            names.add(item)
        elif isinstance(item, dict) and item.get("source"):
            names.add(str(item["source"]))
    return names


class RecapEvaluatorAuthWiring(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.services: dict[str, dict] = {}
        for path in production_compose_files():
            for name, svc in (load_yaml(path).get("services") or {}).items():
                if isinstance(svc, dict):
                    cls.services[name] = svc

    def test_secret_is_declared_in_base(self) -> None:
        base = load_yaml(ROOT / "compose" / "base.yaml")
        spec = (base.get("secrets") or {}).get(SECRET)
        self.assertIsNotNone(spec, f"compose/base.yaml must declare {SECRET}")
        self.assertEqual(spec.get("file"), f"../secrets/{SECRET}.txt")

    def test_evaluator_mounts_the_token(self) -> None:
        svc = self.services[EVALUATOR]
        self.assertIn(SECRET, secret_names(svc))
        self.assertEqual(environment(svc).get(TOKEN_ENV), SECRET_PATH)

    def test_evaluator_reads_the_same_env_name(self) -> None:
        source = (
            ROOT / "recap-evaluator" / "src" / "recap_evaluator" / "infra" / "bearer_auth.py"
        ).read_text(encoding="utf-8")
        self.assertIn(f'"{TOKEN_ENV}"', source)
        self.assertIn(f'"{SECRET_PATH}"', source)

    def test_no_production_service_disables_evaluator_auth(self) -> None:
        for name, svc in self.services.items():
            with self.subTest(service=name):
                self.assertNotEqual(
                    environment(svc).get("EVALUATOR_AUTH", "").strip().lower(), "disabled"
                )

    def test_every_evaluator_caller_carries_the_token(self) -> None:
        callers = [
            name
            for name, svc in self.services.items()
            if name != EVALUATOR
            and any(f"{EVALUATOR}:8080" in value for value in environment(svc).values())
        ]
        for name in callers:
            with self.subTest(caller=name):
                svc = self.services[name]
                self.assertIn(SECRET, secret_names(svc))
                self.assertEqual(environment(svc).get(TOKEN_ENV), SECRET_PATH)


if __name__ == "__main__":
    unittest.main()
