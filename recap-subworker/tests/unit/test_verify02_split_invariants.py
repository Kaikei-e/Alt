"""Permanent VERIFY-02 property-based invariant tests for split functions (commit 1ddd55e73).

Covers all five training entry points (`train.py`, `train_student.py`, `train_teacher.py`,
`pseudo_label.py`, `train_classifier_gpu.py`) and the accompanying split data/augmentation/
evaluation scripts from `1ddd55e73` using a deterministic Python stdlib SplitMix64 generator
(zero third-party property-testing dependencies).
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
import sys
import types
from pathlib import Path
from typing import TYPE_CHECKING

import numpy as np
import pandas as pd
import pytest
import scipy.sparse as sp
import torch
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import f1_score
from torch import nn
from torch.utils.data import DataLoader, TensorDataset
from transformers.modeling_outputs import SequenceClassifierOutput

from recap_subworker.infra.classifier import collect_data, train
from recap_subworker.learning_machine.scripts import pseudo_label, train_student

if TYPE_CHECKING:
    from collections.abc import Sequence

DEFAULT_SEED = int(os.environ.get("VERIFY02_SEED", "20261127"))
TRIALS_FAST = 60
TRIALS_MEDIUM = 50
TRIALS_TORCH = 20

SERVICE_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = SERVICE_ROOT / "scripts"
TRAIN_TEACHER_PATH = (
    SERVICE_ROOT / "recap_subworker" / "learning_machine" / "scripts" / "train_teacher.py"
)

MIN_SVD_FEATURES = 2
MIN_GENRE_SAMPLES = 20
MAX_GENRE_SAMPLES = 2000
MIN_CONFIDENCE_THRESHOLD = 0.5
DEFAULT_THRESHOLD = 0.5
MIN_BACKTRANS_CHARS = 20
MIN_SYNTH_CHARS = 100
MIN_LENGTH_RATIO = 0.5
MAX_LENGTH_RATIO = 2.0
JA_RATIO_THRESHOLD = 0.3
MIN_STRATIFY_SAMPLES = 2
BINARY_CLASS_COUNT = 2
PSEUDO_SAVE_TRIAL_LIMIT = 5
TOLERANCE = 1e-6
FOCAL_TOLERANCE = 1e-5


class SplitMix64:
    """Deterministic Python stdlib 64-bit PRNG (zero external deps, ruff S311 clean)."""

    def __init__(self, seed: int) -> None:
        self._state = seed & 0xFFFFFFFFFFFFFFFF

    def next_u64(self) -> int:
        self._state = (self._state + 0x9E3779B97F4A7C15) & 0xFFFFFFFFFFFFFFFF
        z = self._state
        z = ((z ^ (z >> 30)) * 0xBF58476D1CE4E5B9) & 0xFFFFFFFFFFFFFFFF
        z = ((z ^ (z >> 27)) * 0x94D049BB133111EB) & 0xFFFFFFFFFFFFFFFF
        return z ^ (z >> 31)

    def rand_int(self, lo: int, hi: int) -> int:
        return lo + (self.next_u64() % (hi - lo + 1))

    def rand_float(self) -> float:
        return (self.next_u64() >> 11) / float(1 << 53)

    def rand_uniform(self, lo: float, hi: float) -> float:
        return lo + (hi - lo) * self.rand_float()

    def choice[T](self, seq: Sequence[T]) -> T:
        return seq[self.next_u64() % len(seq)]

    def rand_bool(self, p_true: float = 0.5) -> bool:
        return self.rand_float() < p_true


def _load_script_module(mod_name: str, path: Path) -> types.ModuleType:
    spec = importlib.util.spec_from_file_location(mod_name, str(path))
    assert spec is not None
    assert spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


class _TinyTokenizer:
    def __init__(self, vocab_size: int = 32) -> None:
        self.vocab_size = vocab_size

    def __call__(
        self,
        text: str,
        *,
        max_length: int = 12,
        **_kwargs: object,
    ) -> dict[str, torch.Tensor]:
        seq_len = min(max_length, 10)
        digest = hashlib.sha256(str(text).encode("utf-8")).digest()
        ids = [(digest[i % len(digest)] % (self.vocab_size - 1)) + 1 for i in range(seq_len)]
        return {
            "input_ids": torch.tensor([ids], dtype=torch.long),
            "attention_mask": torch.tensor([[1] * seq_len], dtype=torch.long),
        }


class _TinyTextClassificationDataset(torch.utils.data.Dataset):
    def __init__(
        self,
        texts: list[str | None],
        labels: list[int],
        tokenizer: _TinyTokenizer,
        weights: list[float] | None = None,
        max_length: int = 12,
    ) -> None:
        self.texts = texts
        self.labels = labels
        self.tokenizer = tokenizer
        self.weights = weights if weights is not None else [1.0] * len(texts)
        self.max_length = max_length

    def __len__(self) -> int:
        return len(self.texts)

    def __getitem__(self, idx: int) -> dict[str, torch.Tensor]:
        enc = self.tokenizer(
            "" if self.texts[idx] is None else str(self.texts[idx]),
            max_length=self.max_length,
        )
        return {
            "input_ids": enc["input_ids"].flatten(),
            "attention_mask": enc["attention_mask"].flatten(),
            "labels": torch.tensor(self.labels[idx], dtype=torch.long),
            "weights": torch.tensor(self.weights[idx], dtype=torch.float),
        }


class _TinySeqModel(nn.Module):
    def __init__(
        self,
        model_name: str,
        num_labels: int,
        label_map: dict[str, int] | None = None,
        *,
        init_seed: int = 7,
    ) -> None:
        super().__init__()
        self.model_name = model_name
        self.num_labels = num_labels
        self.label_map = label_map
        self.tokenizer = _TinyTokenizer(vocab_size=32)
        gen = torch.Generator().manual_seed(init_seed)
        self.emb = nn.Embedding(32, 6)
        self.head = nn.Linear(6, num_labels)
        with torch.no_grad():
            self.emb.weight.copy_(torch.randn(32, 6, generator=gen) * 0.2)
            self.head.weight.copy_(torch.randn(num_labels, 6, generator=gen) * 0.2)
            self.head.bias.copy_(torch.randn(num_labels, generator=gen) * 0.1)

    def forward(
        self,
        input_ids: torch.Tensor,
        attention_mask: torch.Tensor,
        labels: torch.Tensor | None = None,
    ) -> SequenceClassifierOutput:
        mask = attention_mask.unsqueeze(-1).float()
        pooled = (self.emb(input_ids) * mask).sum(dim=1) / mask.sum(dim=1).clamp(min=1.0)
        logits = self.head(pooled)
        loss = nn.functional.cross_entropy(logits, labels) if labels is not None else None
        return SequenceClassifierOutput(loss=loss, logits=logits)

    def save_pretrained(self, save_directory: str) -> None:
        out = Path(save_directory)
        out.mkdir(parents=True, exist_ok=True)
        (out / "saved.marker").write_text(self.model_name, encoding="utf-8")


# =====================================================================
# 1. recap_subworker/infra/classifier/train.py
# =====================================================================


def test_train_clamp_svd_components_properties() -> None:
    rng = SplitMix64(DEFAULT_SEED + 1)
    for _ in range(TRIALS_FAST):
        requested = rng.rand_int(-5, 300)
        n_samples = rng.rand_int(-2, 250)
        n_features = rng.rand_int(-2, 250)

        should_raise = n_features < MIN_SVD_FEATURES or requested < 1 or n_samples < 1
        if should_raise:
            with pytest.raises(ValueError, match=r"TruncatedSVD|svd_components"):
                train.clamp_svd_components(requested, n_samples=n_samples, n_features=n_features)
        else:
            got = train.clamp_svd_components(requested, n_samples=n_samples, n_features=n_features)
            assert 1 <= got <= min(requested, n_samples, n_features)
            assert got == min(requested, n_samples, n_features)


def test_train_load_dataframe_and_optimize_thresholds_properties(tmp_path: Path) -> None:
    rng = SplitMix64(DEFAULT_SEED + 2)
    genres = ["ai_data", "sports", "film_tv", "rare_a"]

    for trial in range(TRIALS_MEDIUM):
        rows: list[dict[str, str | None]] = []
        expected_kept: set[str] = set()
        for genre in genres:
            count = rng.rand_int(20, 26) if genre != "rare_a" else rng.rand_int(1, 12)
            rows.extend(
                {"content": f"{genre} body {trial} {idx}", "genre": genre} for idx in range(count)
            )
            if count >= MIN_GENRE_SAMPLES:
                expected_kept.add(genre)
        # Inject NaNs that must be dropped before genre counting.
        rows.append({"content": None, "genre": "ai_data"})
        rows.append({"content": "missing genre", "genre": None})

        base_csv = tmp_path / f"base_{trial}.csv"
        pd.DataFrame(rows).to_csv(base_csv, index=False)

        aug_paths: list[Path] = []
        if rng.rand_bool(0.5):
            aug_csv = tmp_path / f"aug_{trial}.csv"
            aug_rows = [{"content": f"aug ai {trial} {i}", "genre": "ai_data"} for i in range(4)]
            pd.DataFrame(aug_rows).to_csv(aug_csv, index=False)
            aug_paths.append(aug_csv)
            aug_paths.append(tmp_path / f"missing_{trial}.csv")

        loaded = train._load_training_dataframe(base_csv, aug_paths or None)
        assert not loaded["content"].isna().any()
        assert not loaded["genre"].isna().any()
        assert set(loaded["genre"].unique()) == expected_kept
        assert (loaded["genre"].value_counts() >= MIN_GENRE_SAMPLES).all()

    # Property on _optimize_thresholds: chosen threshold achieves optimal grid F1.
    class _StubClf:
        def __init__(self, probs: np.ndarray, classes: list[str]) -> None:
            self._probs = probs
            self.classes_ = np.array(classes)

        def predict_proba(self, _x: np.ndarray) -> np.ndarray:
            return self._probs

    cls_names = ["ai_data", "sports", "film_tv"]
    grid = np.arange(0.1, 0.95, 0.05)
    for _ in range(TRIALS_MEDIUM):
        n_rows = rng.rand_int(15, 40)
        raw = np.array(
            [[rng.rand_uniform(0.01, 1.0) for _ in cls_names] for _ in range(n_rows)],
            dtype=np.float64,
        )
        probs = raw / raw.sum(axis=1, keepdims=True)
        y_test = pd.Series([cls_names[rng.rand_int(0, len(cls_names) - 1)] for _ in range(n_rows)])
        clf = _StubClf(probs, cls_names)

        thresholds = train._optimize_thresholds(clf, np.zeros((n_rows, 2)), y_test)  # type: ignore[arg-type]
        dummies = pd.get_dummies(y_test)
        assert set(thresholds.keys()) == set(dummies.columns)
        for i, cls in enumerate(cls_names):
            if cls not in thresholds:
                continue
            chosen_t = thresholds[cls]
            y_bin = dummies[cls]
            chosen_f1 = float(f1_score(y_bin, (probs[:, i] >= chosen_t).astype(int)))
            grid_max = max(float(f1_score(y_bin, (probs[:, i] >= t).astype(int))) for t in grid)
            if grid_max > 0.0:
                assert abs(chosen_f1 - grid_max) < TOLERANCE
            else:
                assert chosen_t == DEFAULT_THRESHOLD


# =====================================================================
# 2. recap_subworker/learning_machine/scripts/train_student.py
# =====================================================================


def test_train_student_load_jsonl_properties(tmp_path: Path) -> None:
    rng = SplitMix64(DEFAULT_SEED + 3)
    genres = ["ai_data", "sports", "film_tv", "law_crime"]
    label2id = {g: i for i, g in enumerate(genres)}

    for trial in range(TRIALS_MEDIUM):
        jsonl_path = tmp_path / f"student_{trial}.jsonl"
        lines: list[str] = []
        expected_valid = 0
        for i in range(rng.rand_int(4, 15)):
            if rng.rand_bool(0.15):
                lines.append("{not valid json")
                continue
            g = rng.choice([*genres, "unknown_genre"])
            text = f"sample {trial}-{i}" if rng.rand_bool(0.9) else ""
            rec: dict[str, object] = {
                "content" if rng.rand_bool(0.5) else "text": text,
                "lang": rng.choice(["ja", "en"]),
            }
            if rng.rand_bool(0.5):
                rec["label"] = g if rng.rand_bool(0.7) else [g]
            else:
                rec["labels"] = [g]
            if rng.rand_bool(0.4):
                rec["logits"] = [round(rng.rand_uniform(-2.0, 2.0), 3) for _ in genres]
            lines.append(json.dumps(rec))
            if text and g in label2id:
                expected_valid += 1
        jsonl_path.write_text("\n".join(lines) + "\n", encoding="utf-8")

        loaded = train_student.load_jsonl(jsonl_path, label2id)
        assert len(loaded) == expected_valid
        for item in loaded:
            assert 0 <= item["label"] < len(genres)
            assert isinstance(item["text"], str)
            assert item["text"] != ""


def test_train_student_language_defaults_properties(monkeypatch: pytest.MonkeyPatch) -> None:
    rng = SplitMix64(DEFAULT_SEED + 30)
    monkeypatch.setattr(train_student.torch.cuda, "is_available", lambda: False)

    for _ in range(TRIALS_MEDIUM):
        lang = rng.choice(["ja", "en"])
        explicit_epochs = rng.choice([None, 3, 7])
        explicit_alpha = rng.choice([None, 0.25, 0.8])
        ns = argparse.Namespace(
            language=lang,
            seed=rng.rand_int(1, 5000),
            epochs=explicit_epochs,
            batch_size=None,
            lr=None,
            alpha=explicit_alpha,
            temperature=None,
            weight_decay=None,
            warmup_steps=None,
            max_length=None,
        )
        train_student._apply_language_defaults_and_seed(ns)
        assert ns.epochs == (
            explicit_epochs if explicit_epochs is not None else (10 if lang == "ja" else 5)
        )
        assert ns.alpha == (
            explicit_alpha if explicit_alpha is not None else (0.3 if lang == "ja" else 0.5)
        )
        assert ns.batch_size == (16 if lang == "ja" else 32)
        assert ns.warmup_steps == (100 if lang == "ja" else 0)


def test_train_student_splits_and_epoch_step_properties(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    rng = SplitMix64(DEFAULT_SEED + 4)
    genres = ["ai_data", "sports", "film_tv"]
    label2id = {g: i for i, g in enumerate(genres)}
    monkeypatch.setattr(train_student.torch.cuda, "is_available", lambda: False)

    for trial in range(TRIALS_TORCH):
        work = tmp_path / f"st_run_{trial}"
        data_dir = work / "recap_subworker" / "learning_machine" / "data"
        data_dir.mkdir(parents=True)
        lang = rng.choice(["ja", "en"])
        other = "en" if lang == "ja" else "ja"

        n_gold = rng.rand_int(6, 12)
        gold_lines = [
            json.dumps(
                {
                    "content": f"gold-{trial}-{i}",
                    "label": genres[i % len(genres)],
                    "lang": lang,
                    "logits": [round(rng.rand_uniform(-1.0, 1.0), 3) for _ in genres]
                    if rng.rand_bool(0.5)
                    else None,
                }
            )
            for i in range(n_gold)
        ]
        gold_lines.append(json.dumps({"content": "other-lang", "label": genres[0], "lang": other}))
        (data_dir / "gold_seed.jsonl").write_text("\n".join(gold_lines) + "\n", encoding="utf-8")

        n_ext = rng.rand_int(1, 4)
        ext_lines = [
            json.dumps(
                {"content": f"ext-{trial}-{i}", "label": genres[i % len(genres)], "lang": lang}
            )
            for i in range(n_ext)
        ]
        (data_dir / "silver_external.jsonl").write_text(
            "\n".join(ext_lines) + "\n", encoding="utf-8"
        )

        n_pseudo = rng.rand_int(0, 3)
        if n_pseudo > 0:
            p_lines = [
                json.dumps(
                    {
                        "content": f"pseudo-{trial}-{i}",
                        "label": genres[i % len(genres)],
                        "logits": [0.1, 0.2, 0.7],
                        "lang": lang,
                    }
                )
                for i in range(n_pseudo)
            ]
            (data_dir / f"silver_teacher_v0_{lang}.jsonl").write_text(
                "\n".join(p_lines) + "\n", encoding="utf-8"
            )

        args = argparse.Namespace(
            language=lang,
            seed=rng.rand_int(1, 5000),
            epochs=1,
            batch_size=3,
            lr=1e-3,
            alpha=rng.choice([0.0, 0.5]),
            temperature=2.0,
            weight_decay=0.01,
            warmup_steps=0,
            max_length=12,
            output_dir=str(work / "out"),
            model_name="tiny-student",
        )
        monkeypatch.chdir(work)
        train_items, val_items = train_student._load_student_splits(args, label2id)
        assert all(it["lang"] == lang for it in train_items)
        assert all(it["lang"] == lang for it in val_items)
        val_texts = {it["text"] for it in val_items}
        train_texts = {it["text"] for it in train_items}
        assert val_texts.isdisjoint(train_texts)
        assert len(train_items) + len(val_items) == n_gold + n_ext + n_pseudo

        monkeypatch.setattr(
            train_student,
            "StudentDistilBERT",
            lambda name, n_lbl, _s=trial: _TinySeqModel(name, n_lbl, init_seed=_s + 1),
        )
        bundle = train_student._setup_student_training(
            args, train_items, val_items, len(genres), torch.device("cpu")
        )
        loss = train_student._train_one_epoch(bundle, args, torch.device("cpu"), epoch=0)
        macro_f1, weighted_f1 = train_student._evaluate_student(
            bundle.student, bundle.val_loader, torch.device("cpu")
        )
        assert np.isfinite(loss)
        assert loss >= 0.0
        assert 0.0 <= macro_f1 <= 1.0
        assert 0.0 <= weighted_f1 <= 1.0


# =====================================================================
# 3. recap_subworker/learning_machine/scripts/train_teacher.py
# =====================================================================


def test_train_teacher_items_and_training_stage_properties(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    fake_data_pkg = types.ModuleType("recap_subworker.learning_machine.data")
    fake_dataset_mod = types.ModuleType("recap_subworker.learning_machine.data.dataset")
    fake_dataset_mod.TextClassificationDataset = _TinyTextClassificationDataset  # type: ignore[attr-defined]
    fake_data_pkg.dataset = fake_dataset_mod  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "recap_subworker.learning_machine.data", fake_data_pkg)
    monkeypatch.setitem(
        sys.modules, "recap_subworker.learning_machine.data.dataset", fake_dataset_mod
    )

    teacher_mod = _load_script_module("verify02_unit_train_teacher", TRAIN_TEACHER_PATH)
    monkeypatch.setattr(teacher_mod.torch.cuda, "is_available", lambda: False)

    rng = SplitMix64(DEFAULT_SEED + 5)
    genres = ["ai_data", "sports", "film_tv"]
    label2id = {g: i for i, g in enumerate(genres)}

    for trial in range(TRIALS_TORCH):
        work = tmp_path / f"teacher_{trial}"
        data_dir = work / "recap_subworker" / "learning_machine" / "data"
        data_dir.mkdir(parents=True)
        lang = rng.choice(["ja", "en"])
        other = "en" if lang == "ja" else "ja"

        n_gold = rng.rand_int(6, 10)
        gold_lines = []
        for i in range(n_gold):
            text_val: str | None = None if i == 0 and rng.rand_bool(0.5) else f"t-{trial}-{i}"
            gold_lines.append(
                json.dumps({"content": text_val, "labels": [genres[i % len(genres)]], "lang": lang})
            )
        gold_lines.append(json.dumps({"content": "empty-lbl", "labels": [], "lang": lang}))
        gold_lines.append(json.dumps({"content": "other", "labels": [genres[0]], "lang": other}))
        gold_path = data_dir / "gold_seed.jsonl"
        gold_path.write_text("\n".join(gold_lines) + "\n", encoding="utf-8")

        use_ext = rng.rand_bool(0.6)
        n_ext = rng.rand_int(2, 5)
        if use_ext:
            ext_lines = [
                json.dumps(
                    {"text": f"ext-{trial}-{i}", "label": genres[i % len(genres)], "lang": lang}
                )
                for i in range(n_ext)
            ]
            (data_dir / "silver_external.jsonl").write_text(
                "\n".join(ext_lines) + "\n", encoding="utf-8"
            )

        args = argparse.Namespace(
            epochs=1,
            batch_size=2,
            lr=1e-3,
            weight_decay=0.01,
            warmup_steps=rng.choice([0, 2]),
            max_length=12,
            output_dir=str(work / "out"),
            gold_path=str(gold_path),
            use_external=use_ext,
            language=lang,
            model_name=None,
        )
        monkeypatch.chdir(work)
        items = teacher_mod._load_teacher_items(args, label2id)
        expected_total = n_gold + (n_ext if use_ext else 0)
        assert len(items) == expected_total
        assert sum(1 for it in items if it["weight"] == 1.0) == n_gold
        assert sum(1 for it in items if it["weight"] == pytest.approx(0.2)) == (
            n_ext if use_ext else 0
        )

        created_models: list[str] = []

        def _make_teacher(
            model_name: str,
            num_labels: int,
            lbl_map: dict[str, int],
            _s: int = trial,
            _log: list[str] = created_models,
        ) -> _TinySeqModel:
            _log.append(model_name)
            return _TinySeqModel(model_name, num_labels, lbl_map, init_seed=_s + 11)

        monkeypatch.setattr(teacher_mod, "TeacherBERT", _make_teacher)
        bundle = teacher_mod._setup_teacher_training(args, items, label2id, torch.device("cpu"))
        expected_model = "tohoku-nlp/bert-base-japanese-v3" if lang == "ja" else "bert-base-uncased"
        assert created_models == [expected_model]

        tr_loss = teacher_mod._train_one_epoch(bundle, torch.device("cpu"), epoch=0)
        val_loss, val_f1 = teacher_mod._evaluate_teacher(
            bundle.teacher, bundle.val_loader, torch.device("cpu")
        )
        assert np.isfinite(tr_loss)
        assert tr_loss >= 0.0
        assert np.isfinite(val_loss)
        assert val_loss >= 0.0
        assert 0.0 <= val_f1 <= 1.0


# =====================================================================
# 4. recap_subworker/learning_machine/scripts/pseudo_label.py
# =====================================================================


def test_pseudo_label_language_detection_invariants() -> None:
    rng = SplitMix64(DEFAULT_SEED + 6)
    for _ in range(TRIALS_FAST):
        min_chars = rng.rand_int(10, 60)
        short = "a" * (min_chars - 1)
        assert pseudo_label.detect_language_simple(short, min_chars=min_chars) == "unknown"

        ja_text = "あ" * (min_chars + 5) + "abc"
        en_text = "hello " * (min_chars // 5 + 3)
        assert pseudo_label.detect_language_simple(ja_text, min_chars=min_chars) == "ja"
        assert pseudo_label.detect_language_simple(en_text, min_chars=min_chars) == "en"


def test_pseudo_label_selection_and_build_invariants(tmp_path: Path) -> None:
    rng = SplitMix64(DEFAULT_SEED + 60)
    labels = ["ai_data", "sports", "film_tv", "law_crime"]
    for trial in range(TRIALS_FAST):
        n_items = rng.rand_int(10, 45)
        confs = np.array([rng.rand_uniform(0.25, 0.99) for _ in range(n_items)], dtype=np.float64)
        margins = np.array([rng.rand_uniform(0.0, float(c)) for c in confs], dtype=np.float64)
        candidates = [
            {
                "item": {"id": f"doc-{i}", "content": f"body {i}"},
                "max_prob": float(confs[i]),
                "margin": float(margins[i]),
                "pred_label": labels[rng.rand_int(0, len(labels) - 1)],
                "logits": [0.1, 0.2, 0.3, 0.4],
            }
            for i in range(n_items)
        ]

        mode = rng.choice(["top_percent", "min_margin", "threshold", "default"])
        cap = rng.choice([None, 1, 2, 4])
        top_pct = rng.choice([10.0, 25.0, 50.0]) if mode == "top_percent" else None
        min_margin = rng.choice([0.05, 0.2, 0.5]) if mode == "min_margin" else None
        threshold = rng.choice([0.4, 0.65, 0.85]) if mode == "threshold" else None
        args = argparse.Namespace(
            accept_top_percent=top_pct,
            min_margin=min_margin,
            threshold=threshold,
            per_class_cap=cap,
        )

        selected, method, _eff = pseudo_label._select_candidates(candidates, confs, margins, args)
        selected_list = [int(i) for i in selected]
        assert len(selected_list) == len(set(selected_list))
        assert all(0 <= idx < n_items for idx in selected_list)

        if mode == "threshold" and threshold is not None:
            assert method == "threshold"
            assert all(confs[idx] >= threshold for idx in selected_list)
        elif mode == "min_margin" and min_margin is not None:
            assert method == "margin"
            assert all(margins[idx] >= min_margin for idx in selected_list)
        elif mode == "top_percent" and top_pct is not None:
            assert method == "top_percent"
            if cap is None:
                assert len(selected_list) == int(n_items * top_pct / 100.0)
                chosen_confs = [confs[idx] for idx in selected_list]
                assert chosen_confs == sorted(chosen_confs, reverse=True)
        else:
            assert method == "top_percent (default 5%)"
            if cap is None:
                assert len(selected_list) == max(1, int(n_items * 5 / 100.0))

        if cap is not None:
            counts: dict[str, int] = {}
            for idx in selected_list:
                lbl = str(candidates[idx]["pred_label"])
                counts[lbl] = counts.get(lbl, 0) + 1
            assert all(c <= cap for c in counts.values())

        lang = rng.choice(["ja", "en"])
        built = pseudo_label._build_pseudo_labeled_items(candidates, selected, lang)
        assert len(built) == len(selected_list)
        for item in built:
            assert item["source"] == f"teacher_v0_{lang}_pseudo"
            assert item["lang"] == lang
            assert "logits" in item
        assert all("source" not in c["item"] for c in candidates)

        if trial < PSEUDO_SAVE_TRIAL_LIMIT:
            out_file = tmp_path / f"pseudo_{trial}.jsonl"
            pseudo_label._save_pseudo_labels(out_file, built, n_items)
            lines = [ln for ln in out_file.read_text(encoding="utf-8").splitlines() if ln.strip()]
            assert len(lines) == len(built)


# =====================================================================
# 5. scripts/train_classifier_gpu.py
# =====================================================================


def test_train_classifier_gpu_stage_helpers_properties(tmp_path: Path) -> None:
    gpu_mod = _load_script_module(
        "verify02_unit_train_gpu", SCRIPTS_DIR / "train_classifier_gpu.py"
    )
    rng = SplitMix64(DEFAULT_SEED + 7)
    cpu = torch.device("cpu")

    # _to_float_tensor dense vs sparse equivalence + _compute_pos_weights + FocalLoss
    for _ in range(TRIALS_MEDIUM):
        n_rows = rng.rand_int(4, 20)
        n_cls = rng.rand_int(2, 6)
        n_cols = rng.rand_int(n_cls, 12)
        dense = np.array(
            [[rng.rand_uniform(-2.0, 2.0) for _ in range(n_cols)] for _ in range(n_rows)],
            dtype=np.float32,
        )
        sparse = sp.csr_matrix(dense)
        t_dense = gpu_mod._to_float_tensor(dense)
        t_sparse = gpu_mod._to_float_tensor(sparse)
        assert torch.allclose(t_dense, t_sparse)
        assert t_dense.dtype == torch.float32

        y_idx = np.array([rng.rand_int(0, n_cls - 1) for _ in range(n_rows)], dtype=np.int64)
        weights = gpu_mod._compute_pos_weights(y_idx, n_cls, cpu)
        assert weights.shape == (n_cls,)
        for c in range(n_cls):
            cnt = int(np.sum(y_idx == c))
            expected = (n_rows - cnt) / cnt if cnt > 0 else 1.0
            assert abs(float(weights[c].item()) - expected) < TOLERANCE

        # FocalLoss(gamma=0) == BCEWithLogitsLoss and FocalLoss(gamma>0) <= BCE
        logits = torch.tensor(dense[:, :n_cls], dtype=torch.float32)
        targets = torch.zeros(n_rows, n_cls, dtype=torch.float32)
        targets.scatter_(1, torch.tensor(y_idx).unsqueeze(1), 1.0)
        bce = nn.BCEWithLogitsLoss(pos_weight=weights)(logits, targets)
        focal_0 = gpu_mod.FocalLoss(alpha=weights, gamma=0.0)(logits, targets)
        focal_2 = gpu_mod.FocalLoss(alpha=weights, gamma=2.0)(logits, targets)
        assert abs(float(focal_0.item()) - float(bce.item())) < FOCAL_TOLERANCE
        assert 0.0 <= float(focal_2.item()) <= float(bce.item()) + FOCAL_TOLERANCE

    # End-to-end helper chain: _train_model -> _export_and_evaluate -> _optimize_and_save_thresholds
    for trial in range(TRIALS_TORCH):
        n_cls = rng.choice([BINARY_CLASS_COUNT, 3, 4])
        n_feat = rng.rand_int(4, 8)
        classes = [f"c{i}" for i in range(n_cls)]
        np_rng = np.random.default_rng(rng.next_u64() & 0xFFFFFFFF)
        x_tr = torch.tensor(np_rng.normal(size=(12, n_feat)), dtype=torch.float32)
        y_tr = torch.tensor([i % n_cls for i in range(12)], dtype=torch.long)
        x_va = torch.tensor(np_rng.normal(size=(8, n_feat)), dtype=torch.float32)
        y_va_idx = np.array([i % n_cls for i in range(8)], dtype=np.int64)
        y_va_lbl = np.array([classes[i] for i in y_va_idx])
        x_te = torch.tensor(np_rng.normal(size=(8, n_feat)), dtype=torch.float32)
        y_te_lbl = np.array([classes[i % n_cls] for i in range(8)])

        model = nn.Linear(n_feat, n_cls)
        tdata = gpu_mod.TrainingData(
            train_loader=DataLoader(TensorDataset(x_tr, y_tr), batch_size=4, shuffle=False),
            x_valid_tensor=x_va,
            y_valid_tensor=torch.tensor(y_va_idx, dtype=torch.long),
            y_valid_idx=y_va_idx,
            num_classes=n_cls,
            device=cpu,
            pos_weight_tensor=None,
        )
        args = argparse.Namespace(
            epochs=2,
            lr=0.02,
            weight_decay=0.0,
            loss_type=rng.choice(["bce", "focal"]),
            scheduler=rng.choice(["none", "onecycle"]),
            output_model=str(tmp_path / f"gpu_m_{trial}.joblib"),
            output_thresholds=str(tmp_path / f"gpu_t_{trial}.json"),
        )
        gpu_mod._train_model(model, tdata, args)
        sk_model, sk_preds = gpu_mod._export_and_evaluate(model, classes, x_te, y_te_lbl, args)
        assert isinstance(sk_model, LogisticRegression)
        assert len(sk_preds) == len(y_te_lbl)
        np.testing.assert_allclose(sk_model.coef_, model.weight.detach().numpy(), rtol=1e-6)
        np.testing.assert_allclose(sk_model.intercept_, model.bias.detach().numpy(), rtol=1e-6)

        gpu_mod._optimize_and_save_thresholds(sk_model, x_va, y_va_lbl, args.output_thresholds)
        saved_th = json.loads(Path(args.output_thresholds).read_text(encoding="utf-8"))
        assert set(saved_th.keys()) == set(classes)
        assert all(0.0 <= float(v) <= 1.0 for v in saved_th.values())


# =====================================================================
# 6. Companion split data / augmentation / evaluation scripts (1ddd55e73)
# =====================================================================


def test_collect_data_decide_genre_and_balance_properties() -> None:
    rng = SplitMix64(DEFAULT_SEED + 8)
    known_tags = list(collect_data.TAG_TO_GENRE.keys())

    for _ in range(TRIALS_FAST):
        tags = [
            rng.choice(known_tags) if rng.rand_bool(0.75) else f"noise_{i}"
            for i in range(rng.rand_int(0, 8))
        ]
        genre, conf = collect_data.decide_genre(tags)
        assert 0.0 <= conf <= 1.0
        if genre is not None:
            assert conf >= MIN_CONFIDENCE_THRESHOLD
        else:
            assert conf < MIN_CONFIDENCE_THRESHOLD

    for _ in range(25):
        n_rare = rng.rand_int(1, MIN_GENRE_SAMPLES - 1)
        n_ok = rng.rand_int(MIN_GENRE_SAMPLES, 40)
        rows = [
            *({"content": f"rare {i}", "genre": "rare_g"} for i in range(n_rare)),
            *({"content": f"ok {i}", "genre": "ok_g"} for i in range(n_ok)),
        ]
        balanced = collect_data._balance_genres(pd.DataFrame(rows))
        assert set(balanced["genre"].unique()) == {"ok_g"}
        assert MIN_GENRE_SAMPLES <= len(balanced) <= MAX_GENRE_SAMPLES


def test_merge_augmented_dedup_and_balance_properties() -> None:
    merge_mod = _load_script_module(
        "verify02_unit_merge_aug", SCRIPTS_DIR / "merge_augmented_data.py"
    )
    rng = SplitMix64(DEFAULT_SEED + 9)

    for _ in range(TRIALS_MEDIUM):
        rows = []
        base_texts = [f"Article   Body {i}" for i in range(rng.rand_int(3, 8))]
        for txt in base_texts:
            rows.append(
                {
                    "content": txt,
                    "genre": "ai_data",
                    "is_augmented": False,
                    "augmentation_method": "original",
                }
            )
            # Duplicate with different casing/whitespace marked as augmented.
            rows.append(
                {
                    "content": "  " + txt.lower().replace("   ", " ") + " ",
                    "genre": "ai_data",
                    "is_augmented": True,
                    "augmentation_method": "eda",
                }
            )
        extra_aug = rng.rand_int(2, 6)
        rows.extend(
            {
                "content": f"Unique augmented content {j}",
                "genre": "ai_data",
                "is_augmented": True,
                "augmentation_method": "eda",
            }
            for j in range(extra_aug)
        )

        deduped = merge_mod._deduplicate_df(pd.DataFrame(rows))
        assert len(deduped) == len(base_texts) + extra_aug
        # Every original row survived deduplication.
        assert int((~deduped["is_augmented"]).sum()) == len(base_texts)

        cap = rng.rand_int(2, len(deduped))
        balanced = merge_mod._balance_classes(deduped, cap, None, seed=42)
        assert len(balanced) == cap
        if len(base_texts) >= cap:
            assert (~balanced["is_augmented"]).all()


def test_prepare_dataset_extract_and_split_properties() -> None:
    prep_mod = _load_script_module("verify02_unit_prepare", SCRIPTS_DIR / "prepare_dataset.py")
    rng = SplitMix64(DEFAULT_SEED + 10)

    for _ in range(TRIALS_FAST):
        ja = " 日本語記事 " if rng.rand_bool(0.5) else ""
        en = " English text " if rng.rand_bool(0.5) else ""
        has_content_key = rng.rand_bool(0.5)
        fallback = " Generic text " if (has_content_key and rng.rand_bool(0.7)) else ""
        item: dict[str, str] = {"content_ja": ja, "content_en": en}
        if has_content_key:
            item["content"] = fallback

        assert prep_mod._extract_item_text(item, "ja") == ja.strip()
        assert prep_mod._extract_item_text(item, "en") == en.strip()
        both = prep_mod._extract_item_text(item, "both")
        if ja:
            expected_both = ja.strip()
        elif has_content_key:
            # Preserves pre-split semantics: key presence of "content" takes precedence over content_en.
            expected_both = fallback.strip()
        elif en:
            expected_both = en.strip()
        else:
            expected_both = ""
        assert both == expected_both

    for _ in range(TRIALS_MEDIUM):
        n_rows = rng.rand_int(4, 24)
        df = pd.DataFrame(
            {
                "text": [f"doc {i}" for i in range(n_rows)],
                "label": [f"g{i % 3}" for i in range(n_rows)],
            }
        )
        tr, va, te = prep_mod._split_dataset(df)
        assert len(tr) >= 1
        assert len(va) >= 1
        assert len(te) >= 1
        if len(df) - len(tr) >= MIN_STRATIFY_SAMPLES:
            assert len(tr) + len(va) + len(te) == n_rows


def test_validate_augmentation_and_evaluate_and_augment_scripts_properties() -> None:
    val_mod = _load_script_module("verify02_unit_valaug", SCRIPTS_DIR / "validate_augmentation.py")
    eval_mod = _load_script_module("verify02_unit_eval", SCRIPTS_DIR / "evaluate_genres.py")
    bt_mod = _load_script_module("verify02_unit_bt", SCRIPTS_DIR / "augment_backtrans.py")
    eda_mod = _load_script_module("verify02_unit_eda", SCRIPTS_DIR / "augment_eda.py")
    synth_mod = _load_script_module("verify02_unit_synth", SCRIPTS_DIR / "augment_llm_synthetic.py")

    rng = SplitMix64(DEFAULT_SEED + 11)

    for _ in range(TRIALS_FAST):
        # validate_augmentation._validate_lengths
        orig_len = rng.rand_int(20, 100)
        aug_len = rng.rand_int(5, 250)
        aug_df = pd.DataFrame([{"content": "x" * aug_len, "genre": "ai_data"}])
        mask = np.array([True])
        val_mod._validate_lengths(
            aug_df, {"ai_data": ["y" * orig_len]}, mask, MIN_LENGTH_RATIO, MAX_LENGTH_RATIO
        )
        ratio = aug_len / orig_len
        assert bool(mask[0]) == (MIN_LENGTH_RATIO <= ratio <= MAX_LENGTH_RATIO)

        # augment_backtrans.validate_backtranslation
        orig_s = "a" * orig_len
        assert not bt_mod.validate_backtranslation(orig_s, orig_s)
        assert not bt_mod.validate_backtranslation(orig_s, "b" * (MIN_BACKTRANS_CHARS - 1))
        cand_s = "b" * aug_len
        expected_bt = (aug_len >= MIN_BACKTRANS_CHARS) and (
            MIN_LENGTH_RATIO <= (aug_len / orig_len) <= MAX_LENGTH_RATIO
        )
        assert bt_mod.validate_backtranslation(orig_s, cand_s) == expected_bt

        # augment_llm_synthetic.validate_synthetic_sample
        assert not synth_mod.validate_synthetic_sample("short")
        assert not synth_mod.validate_synthetic_sample("Article body: " + "x" * MIN_SYNTH_CHARS)
        assert not synth_mod.validate_synthetic_sample("I cannot comply " + "x" * MIN_SYNTH_CHARS)
        assert synth_mod.validate_synthetic_sample("Real news report " + "x" * MIN_SYNTH_CHARS)

        # augment_eda.detect_language
        n_ja = rng.rand_int(0, 20)
        n_en = rng.rand_int(1, 20)
        mixed = ("あ" * n_ja) + ("a" * n_en)
        expected_lang = "ja" if (n_ja / (n_ja + n_en)) > JA_RATIO_THRESHOLD else "en"
        assert eda_mod.detect_language(mixed) == expected_lang

    # evaluate_genres.format_report_by_language structural invariants
    for _ in range(TRIALS_MEDIUM):
        labels = ["ai_data", "sports", "film_tv"]
        metrics = {
            lbl: {
                "precision": round(rng.rand_uniform(0.1, 1.0), 4),
                "recall": round(rng.rand_uniform(0.1, 1.0), 4),
                "f1": round(rng.rand_uniform(0.1, 1.0), 4),
                "support": rng.rand_int(1, 50),
                "warning": rng.rand_bool(0.5),
            }
            for lbl in labels
        }
        res = {
            "ja": {
                "total_samples": 30,
                "accuracy": 0.85,
                "accuracy_ci": {"lower": 0.8, "upper": 0.9, "width": 0.1},
                "macro_precision": 0.8,
                "macro_recall": 0.8,
                "macro_f1": 0.8,
                "micro_precision": 0.85,
                "micro_recall": 0.85,
                "micro_f1": 0.85,
                "per_genre_metrics": metrics,
                "confusion_matrix": {
                    "labels": labels,
                    "matrix": [[rng.rand_int(0, 10) for _ in labels] for _ in labels],
                },
            }
        }
        md = eval_mod.format_report_by_language(res)
        assert "## Language: JA" in md
        assert "## Per-Genre Metrics" in md
        assert "## Confusion Matrix" in md
        for lbl in labels:
            assert f"| {lbl} |" in md
