"""Generators for the evaluation harness.

A generator feeds an EvalCase into a pipeline and returns the four arguments
``run_eval.run`` expects: ``(body, source_map, articles_by_id,
evidence_by_short_id)``. The harness ships two generators out of the box:

- :mod:`evaluation.generators.recorded_fixture` replays JSON fixtures for
  deterministic CI runs.
- :mod:`evaluation.generators.db_replay` reads already-produced reports from
  the acolyte/alt databases so we can measure ``lang_mix_ratio`` on the
  current production state without regenerating anything.

Live Connect-RPC driving is intentionally not packaged here; it belongs in a
future ``live_acolyte`` generator that runs under the operator's mTLS
context.
"""
