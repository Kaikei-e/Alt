"""Unit tests for domain value objects."""

from __future__ import annotations

import pytest

from recap_subworker.domain.value_objects import (
    GenreName,
    IdempotencyKey,
    SentenceText,
)


class TestSentenceText:
    def test_valid_sentence(self):
        s = SentenceText("Hello world")
        assert s.text == "Hello world"
        assert str(s) == "Hello world"
        assert len(s) == 11

    def test_minimum_length(self):
        s = SentenceText("ab")
        assert len(s) == 2

    def test_too_short_raises(self):
        with pytest.raises(ValueError, match="at least 2 characters"):
            SentenceText("a")

    def test_empty_raises(self):
        with pytest.raises(ValueError, match="at least 2 characters"):
            SentenceText("")

    def test_immutable(self):
        s = SentenceText("test")
        with pytest.raises(AttributeError):
            s.text = "other"  # type: ignore[misc]

    def test_equality_by_value(self):
        a = SentenceText("same")
        b = SentenceText("same")
        assert a == b

    def test_inequality(self):
        a = SentenceText("one")
        b = SentenceText("two")
        assert a != b


class TestGenreName:
    def test_valid_genre(self):
        g = GenreName("tech")
        assert g.value == "tech"
        assert str(g) == "tech"

    def test_empty_raises(self):
        with pytest.raises(ValueError, match="1-32 characters"):
            GenreName("")

    def test_too_long_raises(self):
        with pytest.raises(ValueError, match="1-32 characters"):
            GenreName("a" * 33)

    def test_max_length(self):
        g = GenreName("a" * 32)
        assert len(g.value) == 32

    def test_immutable(self):
        g = GenreName("tech")
        with pytest.raises(AttributeError):
            g.value = "other"  # type: ignore[misc]


class TestIdempotencyKey:
    def test_valid_key(self):
        k = IdempotencyKey("abc-123")
        assert k.value == "abc-123"
        assert str(k) == "abc-123"

    def test_empty_raises(self):
        with pytest.raises(ValueError, match="cannot be empty"):
            IdempotencyKey("")

    def test_equality(self):
        a = IdempotencyKey("key-1")
        b = IdempotencyKey("key-1")
        assert a == b
        assert hash(a) == hash(b)
