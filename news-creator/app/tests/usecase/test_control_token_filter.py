"""Tests for streaming control-token boundary buffer filter."""

from news_creator.usecase.control_token_filter import ControlTokenFilter


def test_exact_match_tokens_are_dropped():
    f = ControlTokenFilter()
    assert f.push("<|turn>") == ""
    assert f.push("hello") == "hello"
    assert f.flush() == ""


def test_split_control_token_across_chunks_is_buffered():
    f = ControlTokenFilter()
    assert f.push("<|tu") == ""
    assert f.push("rn>") == ""
    assert f.push("ok") == "ok"
    assert f.flush() == ""


def test_partial_prefix_that_is_not_token_is_released():
    f = ControlTokenFilter()
    assert f.push("<|tu") == ""
    assert f.push("rnX") == "<|turnX"
    assert f.flush() == ""


def test_flush_releases_buffered_prefix():
    f = ControlTokenFilter()
    assert f.push("<|tu") == ""
    assert f.flush() == "<|tu"
