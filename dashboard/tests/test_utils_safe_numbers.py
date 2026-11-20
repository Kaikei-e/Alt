"""Characterization tests pinning HEAD behavior of utils.safe_int and utils.safe_float."""

import math
from decimal import Decimal

import numpy as np
import pandas as pd
import pytest

import utils


class TestSafeIntCharacterization:
    @pytest.mark.parametrize(
        ("value", "expected"),
        [
            (None, 0),
            (float("nan"), 0),
            (np.float64("nan"), 0),
            (pd.NA, 0),
            (pd.NaT, 0),
            ("nan", 0),
            ("abc", 0),
            ("", 0),
            ("1.5", 1),
            (" 2 ", 2),
            (True, 1),
            (3, 3),
            (2.9, 2),
            (Decimal("1.25"), 1),
            (np.int64(7), 7),
            (b"4", 4),
            ([1], 0),
        ],
    )
    def test_matches_head_values(self, value: object, expected: int) -> None:
        assert utils.safe_int(value) == expected

    def test_inf_raises_overflow_error(self) -> None:
        with pytest.raises(OverflowError):
            utils.safe_int(float("inf"))

    def test_series_raises_ambiguous_truth_value(self) -> None:
        with pytest.raises(ValueError, match="ambiguous"):
            utils.safe_int(pd.Series([1.0]))


class TestSafeFloatCharacterization:
    @pytest.mark.parametrize(
        ("value", "expected"),
        [
            (None, 0.0),
            (float("nan"), 0.0),
            (np.float64("nan"), 0.0),
            (pd.NA, 0.0),
            (pd.NaT, 0.0),
            ("abc", 0.0),
            ("", 0.0),
            ("1.5", 1.5),
            (" 2 ", 2.0),
            (True, 1.0),
            (3, 3.0),
            (2.9, 2.9),
            (Decimal("1.25"), 1.25),
            (np.int64(7), 7.0),
            (b"4", 4.0),
            ([1], 0.0),
        ],
    )
    def test_matches_head_values(self, value: object, expected: float) -> None:
        assert utils.safe_float(value) == expected

    def test_string_nan_returns_nan(self) -> None:
        assert math.isnan(utils.safe_float("nan"))

    def test_inf_returns_inf(self) -> None:
        result = utils.safe_float(float("inf"))
        assert math.isinf(result)
        assert result > 0

    def test_series_raises_ambiguous_truth_value(self) -> None:
        with pytest.raises(ValueError, match="ambiguous"):
            utils.safe_float(pd.Series([1.0]))
