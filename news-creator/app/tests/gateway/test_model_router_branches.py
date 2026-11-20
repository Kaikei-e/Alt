"""Branch coverage tests for ModelRouter hysteresis and overflow."""

from __future__ import annotations

from unittest.mock import Mock

from news_creator.gateway.model_router import ModelRouter


def test_model_router_exceeds_60k_and_2x_rule_hold() -> None:
    cfg = Mock()
    cfg.model_routing_enabled = True
    cfg.oom_detection_enabled = False
    cfg.model_name = "gemma4-e4b-12k"
    cfg.model_8k_name = "gemma4-e4b-8k"
    cfg.model_60k_name = "gemma4-e4b-60k"
    cfg.llm_num_ctx = 8192
    cfg.llm_num_predict = 500
    cfg.token_safety_margin_percent = 10
    cfg.token_safety_margin_fixed = 512
    cfg.model_60k_enabled = True

    router = ModelRouter(cfg)
    huge_prompt = "a" * 300_000
    model, bucket = router.select_model(huge_prompt)
    assert model == "gemma4-e4b-60k"
    assert bucket == ModelRouter.BUCKET_60K

    held_bucket_tokens = 10_000
    router._current_bucket = held_bucket_tokens
    small_prompt = "hello"
    held_model, held_bucket = router.select_model(small_prompt)
    assert held_model == "gemma4-e4b-60k"
    assert held_bucket == held_bucket_tokens
