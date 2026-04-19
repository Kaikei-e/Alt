"""Faithfulness judges.

The evaluation harness treats "judge" as an opaque ``Callable[[str], float]``.
This package ships two concrete judges:

- :class:`evaluation.judges.mock.MockRubricJudge` — deterministic scorer
  used by CI. Always returns the configured constant regardless of content.
- :class:`evaluation.judges.gemma4.Gemma4FaithfulnessJudge` — production
  path that asks the news-creator LLM to score against the 5-bin rubric.

Both go through :func:`evaluation.judges.prompt.build_judge_prompt` which
centralises the XML-tag defences against indirect prompt injection.
"""
