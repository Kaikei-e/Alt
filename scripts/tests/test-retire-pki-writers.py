#!/usr/bin/env python3
"""Exercise the actual retirement script with an isolated fake Docker CLI."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / "scripts/retire-alt-pki-agent-leftovers.sh"
PARENTS = ["knowledge-sovereign", "recap-evaluator"]
FORMER_WRITERS = ["pki-agent-knowledge-sovereign", "pki-agent-recap-evaluator"]
FAKE_DOCKER = r'''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
p = Path(os.environ["FAKE_DOCKER_STATE"])
s = json.loads(p.read_text())
args = sys.argv[1:]
if args[0] == "ps":
    for cid, service in s["containers"].items():
        print(cid + "\t" + service)
elif args[0] in {"stop", "rm"}:
    ids = args[args.index("--") + 1:]
    s["calls"].append([args[0], ids])
    if args[0] == "rm" and not s.get("keep_leftovers"):
        for cid in ids:
            s["containers"].pop(cid, None)
    p.write_text(json.dumps(s))
else:
    raise SystemExit("unexpected fake Docker command")
'''


class RetirementTests(unittest.TestCase):
    def run_retirement(self, services, **options):
        with tempfile.TemporaryDirectory() as td:
            directory = Path(td)
            stub = directory / "docker"
            stub.write_text(FAKE_DOCKER)
            stub.chmod(0o700)
            state = directory / "state.json"
            state.write_text(json.dumps({
                "containers": {f"id{i}": svc for i, svc in enumerate(services)},
                "calls": [], **options,
            }))
            env = dict(os.environ, DOCKER_BIN=str(stub), FAKE_DOCKER_STATE=str(state))
            env.pop("ALT_ACK_FRESH_INSTALL", None)
            env.pop("COMPOSE_PROJECT", None)
            result = subprocess.run(["bash", str(SCRIPT)], env=env,
                                    capture_output=True, text=True, timeout=10)
            return result, json.loads(state.read_text())

    def test_steady_state_without_sidecars_is_a_no_op(self):
        result, state = self.run_retirement(["step-ca", *PARENTS])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(state["calls"], [])
        self.assertEqual(set(state["containers"].values()), {"step-ca", *PARENTS})

    def test_former_sovereign_and_evaluator_writers_are_retired(self):
        # Both parents enroll in-process now; a surviving sidecar on the same
        # cert volume is a dual writer like every other pki-agent-* leftover.
        result, state = self.run_retirement(["step-ca", *PARENTS, *FORMER_WRITERS])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(state["calls"], [
            ["stop", ["id3", "id4"]], ["rm", ["id3", "id4"]],
        ])
        self.assertEqual(set(state["containers"].values()), {"step-ca", *PARENTS})

    def test_legacy_and_unknown_writers_are_retired_exactly(self):
        result, state = self.run_retirement([
            "step-ca", *PARENTS, "pki-agent-alt-backend", "pki-agent-unregistered",
        ])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(state["calls"], [
            ["stop", ["id3", "id4"]], ["rm", ["id3", "id4"]],
        ])
        self.assertEqual(set(state["containers"].values()), {"step-ca", *PARENTS})

    def test_surviving_former_writer_fails_closed(self):
        result, state = self.run_retirement([
            "step-ca", *PARENTS, "pki-agent-recap-evaluator",
        ], keep_leftovers=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("leftovers still running", result.stderr)
        self.assertEqual(state["calls"], [["stop", ["id3"]], ["rm", ["id3"]]])

    def test_surviving_legacy_writer_fails_closed(self):
        result, state = self.run_retirement([
            "step-ca", *PARENTS, "pki-agent-alt-backend",
        ], keep_leftovers=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("leftovers still running", result.stderr)
        self.assertEqual(state["calls"], [["stop", ["id3"]], ["rm", ["id3"]]])

    def test_missing_service_label_fails_before_mutation(self):
        result, state = self.run_retirement(["step-ca", ""])
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(state["calls"], [])


if __name__ == "__main__":
    unittest.main()
