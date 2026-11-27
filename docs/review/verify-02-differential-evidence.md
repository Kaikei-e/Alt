# VERIFY-02 Differential Property-Based Verification Evidence

- **Scope**: VERIFY-02 (Differential property-based testing for split long functions) per `docs/ADR/001028.md` (pure Go stdlib-only under `scripts/altharness`, zero external Go dependencies) and Learnings §5 (`1ddd55e73`, `a3d5e270b`).
- **Pre-split Base Commit (`recap-subworker`)**: `1ddd55e73~1` (`a0428c6c52c19806b4feee0e7e3451805fbdf59f`)
- **Post-split Verified Commit**: `HEAD` (`f6598d3a7` + commit `9b820bce5` error-boundary restorations)
- **Verification Harness**: `scripts/altharness/internal/verifydiff` (`Run(ctx, args, stdout, stderr) int`)
- **Permanent Invariant Suite**:
  - Python (stdlib SplitMix64 seeded property generators, zero new deps): `recap-subworker/tests/unit/test_verify02_split_invariants.py` (13 test suites, 50–60 randomized trials per pure/data helper, 20 trials per PyTorch training loop)
  - Go (`testing/quick` stdlib property tests): `scripts/altharness/internal/verifydiff/verifydiff_test.go` (`TestQuickPropertySanitizeLabelAndSplitEquivalence`, 200 + 500 `quick.Check` trials)

---

## 1. Inventory of Unverified Split Functions (Learnings §5 — `1ddd55e73`)

Commit `1ddd55e73` recorded:
> *"The five training entry points are split into stage helpers called in the original order. Only `train.py` has an end-to-end smoke test; the other four are covered by review only."*

All **5 training entry points** plus all **8 companion split data/augmentation/evaluation scripts** from `1ddd55e73` (13 scripts total, 650 old-vs-new randomized differential trials) were verified by extracting `1ddd55e73~1:<path>` into an isolated temporary directory via `verify-diff run` (`git show`), executing both pre-split and post-split entry points under identical seeded randomized inputs, asserting exact output equivalence, and removing the temporary directory via `defer`.

### 1.1 Five Training Entry Points

| # | File | Split Helpers Extracted in `1ddd55e73` | Seed | Trials | Duration | Result | Output SHA-256 (prefix) | Evidence Pointer |
|---|---|---|---|---:|---:|---|---|---|
| 1 | `recap-subworker/recap_subworker/infra/classifier/train.py` | `_load_training_dataframe`, `_extract_features`, `_filter_label_issues`, `_train_and_evaluate`, `_optimize_thresholds`, `_save_training_artifacts` | `20261101` | 50 | 54.71s | **PASS** | `897091cc654b` | `.harness/evidence/verify-02/train-py.json` |
| 2 | `recap-subworker/recap_subworker/learning_machine/scripts/train_student.py` | `_append_if_exists`, `_discover_ldconfig_cuda_dirs`, `_parse_args`, `_apply_language_defaults_and_seed`, `_resolve_device`, `_load_student_splits`, `_setup_student_training`, `_train_one_epoch`, `_evaluate_student` | `20261102` | 50 | 12.98s | **PASS** | `a17e50260731` | `.harness/evidence/verify-02/train-student-py.json` |
| 3 | `recap-subworker/recap_subworker/learning_machine/scripts/train_teacher.py` | `_append_if_exists`, `_discover_ldconfig_cuda_dirs`, `_parse_args`, `_resolve_device`, `_load_teacher_items`, `_setup_teacher_training`, `_train_one_epoch`, `_evaluate_teacher` | `20261103` | 50 | 12.06s | **PASS** | `ca7c19ab3ea3` | `.harness/evidence/verify-02/train-teacher-py.json` |
| 4 | `recap-subworker/recap_subworker/learning_machine/scripts/pseudo_label.py` | `_append_if_exists`, `_discover_ldconfig_cuda_dirs`, `_is_japanese_char`, `detect_language_simple`, `_parse_args`, `_resolve_device`, `_load_teacher_model`, `_load_and_filter_articles`, `_run_inference`, `_select_candidates`, `_build_pseudo_labeled_items`, `_log_pseudo_label_statistics`, `_save_pseudo_labels` | `20261104` | 50 | 13.40s | **PASS** | `bd7ee91d619f` | `.harness/evidence/verify-02/pseudo-label-py.json` |
| 5 | `recap-subworker/scripts/train_classifier_gpu.py` | `_parse_args`, `_to_float_tensor`, `_compute_pos_weights`, `_train_model`, `_load_split_datasets`, `_export_and_evaluate`, `_optimize_and_save_thresholds` | `20261105` | 50 | 4.21s | **PASS** | `577014104dcb` | `.harness/evidence/verify-02/train-classifier-gpu-py.json` |

### 1.2 Eight Companion Split Data / Augmentation / Evaluation Scripts (`1ddd55e73`)

| # | File | Split Helpers Extracted in `1ddd55e73` | Seed | Trials | Duration | Result | Output SHA-256 (prefix) | Evidence Pointer |
|---|---|---|---|---:|---:|---|---|---|
| 6 | `recap-subworker/recap_subworker/infra/classifier/collect_data.py` | `_load_alt_export_df`, `_load_golden_df`, `_load_external_df`, `_balance_genres` | `20261106` | 50 | 2.49s | **PASS** | `8e0d39c4ba36` | `.harness/evidence/verify-02/collect-data-py.json` |
| 7 | `recap-subworker/scripts/merge_augmented_data.py` | `_parse_args`, `_load_augmented`, `_deduplicate_df`, `_balance_classes` | `20261107` | 50 | 2.94s | **PASS** | `d2f24e26089e` | `.harness/evidence/verify-02/merge-augmented-data-py.json` |
| 8 | `recap-subworker/scripts/prepare_dataset.py` | `_parse_args`, `_extract_item_text`, `_load_records`, `_split_dataset`, `_build_vectorizer`, `_embed_texts` | `20261108` | 50 | 12.33s | **PASS** | `12b81978ca52` | `.harness/evidence/verify-02/prepare-dataset-py.json` |
| 9 | `recap-subworker/scripts/validate_augmentation.py` | `_parse_args`, `_load_augmented_data`, `_validate_lengths`, `_validate_similarities`, `_validate_labels`, `_report_and_save` | `20261109` | 50 | 12.46s | **PASS** | `62284e1ef462` | `.harness/evidence/verify-02/validate-augmentation-py.json` |
| 10 | `recap-subworker/scripts/evaluate_genres.py` | `_append_summary_lines`, `_append_cv_and_gap_lines`, `_append_per_genre_lines`, `_append_confusion_matrix_lines` | `20261110` | 50 | 12.56s | **PASS** | `b2aa5f231a11` | `.harness/evidence/verify-02/evaluate-genres-py.json` |
| 11 | `recap-subworker/scripts/augment_backtrans.py` | `_parse_args`, `_augment_genres`, `_write_output` | `20261111` | 50 | 2.91s | **PASS** | `d771a4d079ed` | `.harness/evidence/verify-02/augment-backtrans-py.json` |
| 12 | `recap-subworker/scripts/augment_eda.py` | `_parse_args`, `_init_augmenters`, `_augment_target_genres`, `_write_output` | `20261112` | 50 | 3.88s | **PASS** | `60c365e29c43` | `.harness/evidence/verify-02/augment-eda-py.json` |
| 13 | `recap-subworker/scripts/augment_llm_synthetic.py` | `_parse_args`, `_resolve_target_genres`, `_generate_genre_samples`, `_write_output` | `20261113` | 50 | 2.84s | **PASS** | `52a2c8b117eb` | `.harness/evidence/verify-02/augment-llm-synthetic-py.json` |

### 1.3 Audit of `recap-worker` Pure Split Helpers (`a3d5e270b`)

Commit `a3d5e270b` split cognitive-complexity hotspots in `recap-worker/recap-worker` only after adding 63 characterization tests run against both the pre-split and post-split code (864 -> 927 library tests passing):

- `src/pipeline/select/filtering.rs`: `group_by_primary_genre`, `build_embedding_texts`, `filter_genre_outliers`, `filter_genre_by_embeddings`, `compute_centroid`, `non_negative_f64_to_usize`, `compute_p80_distance`, `compute_effective_min`, `select_valid_assignments` — covered by `src/pipeline/select.rs` unit/async tests.
- `src/pipeline/select/clustering.rs`: `group_assignments_by_genre`, `build_subcluster_texts`, `split_large_genre`, `apply_subgenre_labels` — covered by `src/pipeline/select.rs` unit/async tests.
- `src/eval/metrics.rs`: `format_selection_markdown`, `format_quality_markdown`, `format_execution_markdown`, `format_drop_markdown` — covered by 20 unit tests including byte-identical Markdown rendering (`test_to_markdown_renders_full_report_byte_identically`).

No split-induced divergence was found in `a3d5e270b`.

---

## 2. Pre-Existing Behavioral Findings Surfaced During Differential Verification

Zero split-induced divergences exist between `1ddd55e73~1` and `HEAD` across all 650 differential trials. Differential execution surfaced five pre-existing edge behaviors that are identical on both `1ddd55e73~1` and `HEAD` (documented here rather than silently altered):

1. **`pseudo_label.py` Python 3.14 `argparse` unescaped `%`**: `--accept_top_percent` defines `help="Accept top P% of samples by confidence (alternative to threshold)"`. Python 3.14 eagerly validates `%` in `ArgumentParser._check_help` during `add_argument`, raising `ValueError: badly formed help string` on every invocation (not just `--help`). Stubbing `ArgumentParser._check_help` identically on both versions verified 100% pipeline equivalence across all 4 selection modes and `per_class_cap`.
2. **`train_teacher.py` missing `recap_subworker.learning_machine.data.dataset`**: Both `1ddd55e73~1` and `HEAD` import `TextClassificationDataset` from an untracked module. Injecting a deterministic `Dataset` via `mock.patch.dict(sys.modules, ...)` verified 100% training/checkpoint equivalence across 50 trials, including `None`-text preservation restored in `9b820bce5`.
3. **`train_classifier_gpu.py` unused `OneCycleLR` scheduler**: Both `1ddd55e73~1` and `HEAD` construct `optim.lr_scheduler.OneCycleLR(...)` without stepping it inside the epoch loop. Differential verification confirmed identical weights, intercepts, and thresholds across both `--scheduler none` and `--scheduler onecycle`.
4. **`prepare_dataset.py` `"content"` key-presence precedence**: In `--language both` mode, both `1ddd55e73~1` and `HEAD` test `if "content" in item:` (presence, not truthiness), so an empty `"content": ""` returns `""` without falling back to `content_en`. Pinned in `test_prepare_dataset_extract_and_split_properties`.
5. **`train.py` optional `cleanlab` import placement**: Both `1ddd55e73~1` and `HEAD` execute `from cleanlab.filter import find_label_issues` outside the `try:` block, raising `ModuleNotFoundError` when the optional `[train]` extra is uninstalled and falling back only on runtime exceptions from `find_label_issues`/`cross_val_predict`. Both paths verified identically across 50 trials.

---

## 3. LOOP-03 Decision Log Records (`decisions.tsv`)

Original Wave-1 differential verification runs recorded during implementation (note: following Carbon High review, `alt-harness verify-diff` now writes through `guards.AppendDecisionToFile` using the canonical LOOP-03/LOOP-06 closed result vocabulary `info` on pass / `mismatch` on failure, and defaults to `.harness/evidence/verify-02/decisions.tsv` when `.harness/runs/<branch>/decisions.tsv` has not yet been initialized via `alt-harness harness-run init`):

```tsv
ts	phase	decision	why	evidence	result
2026-11-26T17:49:07Z	verify-02	verify-diff recap-subworker/recap_subworker/infra/classifier/train.py (seed=20261101, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/train-py.json	pass
2026-11-26T17:49:20Z	verify-02	verify-diff recap-subworker/recap_subworker/learning_machine/scripts/train_student.py (seed=20261102, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/train-student-py.json	pass
2026-11-26T17:49:32Z	verify-02	verify-diff recap-subworker/recap_subworker/learning_machine/scripts/train_teacher.py (seed=20261103, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/train-teacher-py.json	pass
2026-11-26T17:49:46Z	verify-02	verify-diff recap-subworker/recap_subworker/learning_machine/scripts/pseudo_label.py (seed=20261104, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/pseudo-label-py.json	pass
2026-11-26T17:49:50Z	verify-02	verify-diff recap-subworker/scripts/train_classifier_gpu.py (seed=20261105, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/train-classifier-gpu-py.json	pass
2026-11-26T17:49:52Z	verify-02	verify-diff recap-subworker/recap_subworker/infra/classifier/collect_data.py (seed=20261106, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/collect-data-py.json	pass
2026-11-26T17:49:55Z	verify-02	verify-diff recap-subworker/scripts/merge_augmented_data.py (seed=20261107, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/merge-augmented-data-py.json	pass
2026-11-26T17:50:08Z	verify-02	verify-diff recap-subworker/scripts/prepare_dataset.py (seed=20261108, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/prepare-dataset-py.json	pass
2026-11-26T17:50:20Z	verify-02	verify-diff recap-subworker/scripts/validate_augmentation.py (seed=20261109, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/validate-augmentation-py.json	pass
2026-11-26T17:50:33Z	verify-02	verify-diff recap-subworker/scripts/evaluate_genres.py (seed=20261110, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/evaluate-genres-py.json	pass
2026-11-26T17:50:36Z	verify-02	verify-diff recap-subworker/scripts/augment_backtrans.py (seed=20261111, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/augment-backtrans-py.json	pass
2026-11-26T17:50:40Z	verify-02	verify-diff recap-subworker/scripts/augment_eda.py (seed=20261112, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/augment-eda-py.json	pass
2026-11-26T17:50:43Z	verify-02	verify-diff recap-subworker/scripts/augment_llm_synthetic.py (seed=20261113, base=a0428c6c52c1)	VERIFY-02 differential equivalence check against 1ddd55e73~1	.harness/evidence/verify-02/augment-llm-synthetic-py.json	pass
```

---

## 4. Verification Commands & Clean-Tree Confirmation

```bash
# Go verifydiff package (pure Go stdlib, t.Parallel(), testing/quick property checks)
gofmt -s -l scripts/altharness
go -C scripts/altharness vet ./...
go -C scripts/altharness test -race -count=1 ./internal/verifydiff

# Permanent Python property-based invariant suite (recap-subworker)
cd recap-subworker && uv run --offline ruff check .
cd recap-subworker && uv run --offline ruff format --check .
cd recap-subworker && uv run --offline pyrefly check
cd recap-subworker && uv run --offline pytest -o addopts="" -v tests/unit/test_verify02_split_invariants.py

# Clean-tree check (once wired into alt-harness main.go)
./scripts/altharness/alt-harness verify-diff check-clean
# Verified output: VERIFY-02 check-clean OK (8825 tracked files scanned)
```
