#!/usr/bin/env python3
"""Automate Graph Boost threshold tuning and verification workflow."""

from __future__ import annotations

import argparse
import os
import pathlib
import subprocess
import textwrap
from collections import OrderedDict
from typing import Mapping, Sequence

import yaml

from graph_boost_utils import (
    find_latest_snapshot,
    load_snapshot,
    prepare_dataframe,
    run_bayes_optimization,
    GraphBoostParams,
)

ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_SNAPSHOT_DIR = ROOT / "analytics" / "graph_boost_snapshot"
DEFAULT_GRAPH_CONFIG = (
    ROOT / "recap-worker" / "recap-worker" / "config" / "graph.local.yaml"
)
DEFAULT_CARGO_BIN = "cargo"
CONFIG_COMMENT = "# Local override for Graph Boost thresholds (Phase 3 tuning)\n"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Bayes最適化とローカル確認を組み合わせたGraph Boost検証フロー"
    )
    parser.add_argument(
        "--snapshot-dir",
        type=pathlib.Path,
        default=DEFAULT_SNAPSHOT_DIR,
        help="Graph Boost snapshot の格納ディレクトリ",
    )
    parser.add_argument(
        "--iterations",
        type=int,
        default=30,
        help="ベイズ最適化の反復数（デフォルト: 30）",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=42,
        help="乱数シード（再現のため）",
    )
    parser.add_argument(
        "--graph-config",
        type=pathlib.Path,
        default=DEFAULT_GRAPH_CONFIG,
        help="ローカル override ファイルへのパス（GRAPH_CONFIGと同じ値）",
    )
    parser.add_argument(
        "--run-tests",
        action="store_true",
        help="cargo test を実行して refineq の変更をガード",
    )
    parser.add_argument(
        "--run-replay",
        action="store_true",
        help="replay_genre_pipeline を dry-run で再実行（--replay-dataset が必須）",
    )
    parser.add_argument(
        "--replay-dataset",
        type=pathlib.Path,
        help="replay_genre_pipeline に渡す JSON データセット（--run-replay 時は必須）",
    )
    parser.add_argument(
        "--replay-dsn",
        type=str,
        help="replay で使う RECAP_DB_DSN（省略時: 環境変数 RECAP_DB_DSN）",
    )
    parser.add_argument(
        "--cargo-bin",
        type=str,
        default=DEFAULT_CARGO_BIN,
        help="cargo バイナリのパス（例: cargo, /usr/local/bin/cargo）",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="ファイル書き出しやコマンド実行をせずに内容を表示",
    )
    parser.add_argument(
        "--update-config",
        action="store_true",
        help="最適化結果をグラフ設定ファイルに書き戻す",
    )
    return parser.parse_args()


def _format_params(params: GraphBoostParams) -> str:
    return (
        f"(graph_margin={params.graph_margin:.3f}, boost_threshold={params.boost_threshold:.3f}, "
        f"tag_count_threshold={params.tag_count_threshold})"
    )


def _print_summary(summary, snapshot_path: pathlib.Path) -> None:
    print(f"using snapshot: {snapshot_path}")
    print(f"- best params: {_format_params(summary.best_params)}")
    print(f"- accuracy: {summary.best_accuracy:.4f}")
    print("\noptimization history:")
    for idx, (accuracy, params) in enumerate(summary.history, start=1):
        print(f"{idx:02d}: accuracy={accuracy:.4f} params={_format_params(params)}")


def _read_yaml(path: pathlib.Path) -> Mapping[str, object]:
    if not path.exists():
        return {}
    try:
        content = path.read_text()
    except OSError as err:  # pragma: no cover (filesystem)
        raise RuntimeError(f"failed to read {path}: {err}") from err
    try:
        return yaml.safe_load(content) or {}
    except yaml.YAMLError as err:
        raise RuntimeError(f"failed to parse {path}: {err}") from err


def _build_config(existing: Mapping[str, object], params: GraphBoostParams) -> OrderedDict:
    ordered = OrderedDict()
    ordered["graph_margin"] = params.graph_margin
    ordered["weighted_tie_break_margin"] = existing.get("weighted_tie_break_margin", 0.05)
    ordered["tag_confidence_gate"] = existing.get("tag_confidence_gate", 0.6)
    ordered["boost_threshold"] = params.boost_threshold
    ordered["tag_count_threshold"] = params.tag_count_threshold
    for key, value in existing.items():
        if key not in ordered:
            ordered[key] = value
    return ordered


def _write_graph_config(
    path: pathlib.Path, params: GraphBoostParams, existing: Mapping[str, object], dry_run: bool
) -> None:
    updated = _build_config(existing, params)
    body = yaml.safe_dump(dict(updated), sort_keys=False)
    new_content = textwrap.dedent(f"""{CONFIG_COMMENT}{body}""")
    if path.exists() and path.read_text() == new_content:
        print("graph config already matches the optimized thresholds")
        return
    print(f"writing optimized thresholds to {path}")
    if dry_run:
        print("(dry-run) -- skipping file write")
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(new_content)


def _run_subprocess(command: Sequence[str], env: Mapping[str, str], cwd: pathlib.Path) -> None:
    print(f"→ running: {' '.join(command)}")
    subprocess.run(command, check=True, env=env, cwd=cwd)


def _run_tests(cargo_bin: str, env: Mapping[str, str], root: pathlib.Path, dry_run: bool) -> None:
    if dry_run:
        print("(dry-run) skipping cargo test")
        return
    recap_worker_dir = root / "recap-worker" / "recap-worker"
    _run_subprocess(
        [cargo_bin, "test", "--", "pipeline::genre_refine"],
        env,
        recap_worker_dir,
    )


def _run_replay(
    cargo_bin: str,
    env: Mapping[str, str],
    root: pathlib.Path,
    dataset: pathlib.Path,
    dsn: str,
    dry_run: bool,
) -> None:
    if dry_run:
        print("(dry-run) skipping replay_genre_pipeline")
        return
    recap_worker_dir = root / "recap-worker" / "recap-worker"
    command = [
        cargo_bin,
        "run",
        "--bin",
        "replay_genre_pipeline",
        "--",
        "--dataset",
        str(dataset),
        "--dsn",
        dsn,
        "--dry-run",
    ]
    _run_subprocess(command, env, recap_worker_dir)


def main() -> None:
    args = parse_args()
    snapshot_path = find_latest_snapshot(args.snapshot_dir)
    df = prepare_dataframe(load_snapshot(snapshot_path))
    summary = run_bayes_optimization(df, args.iterations, args.seed)
    _print_summary(summary, snapshot_path)

    if args.update_config:
        existing_config = _read_yaml(args.graph_config)
        _write_graph_config(
            args.graph_config, summary.best_params, existing_config, args.dry_run
        )
    else:
        print(
            "graph config update skipped (pass --update-config to persist the thresholds)"
        )

    env = os.environ.copy()
    env["GRAPH_CONFIG"] = str(args.graph_config)

    repo_root = ROOT
    if args.run_tests:
        _run_tests(args.cargo_bin, env, repo_root, args.dry_run)

    if args.run_replay:
        dataset = args.replay_dataset
        if dataset is None:
            raise SystemExit("--run-replay requires --replay-dataset")
        dsn = args.replay_dsn or os.environ.get("RECAP_DB_DSN")
        if not dsn:
            raise SystemExit("RECAP_DB_DSN must be set via --replay-dsn or environment for replay")
        _run_replay(args.cargo_bin, env, repo_root, dataset, dsn, args.dry_run)


if __name__ == "__main__":
    main()

