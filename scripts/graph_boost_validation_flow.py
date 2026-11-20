#!/usr/bin/env python3
"""Automate Graph Boost threshold tuning and verification workflow."""

from __future__ import annotations

import argparse
import os
import pathlib
import shutil
import subprocess
import sys
import textwrap
from collections import OrderedDict
from collections.abc import Mapping, Sequence
from dataclasses import dataclass

import yaml

from graph_boost_utils import (
    GraphBoostParams,
    OptimizationSummary,
    find_latest_snapshot,
    load_snapshot,
    prepare_dataframe,
    run_bayes_optimization,
)

ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_SNAPSHOT_DIR = ROOT / "analytics" / "graph_boost_snapshot"
DEFAULT_GRAPH_CONFIG = ROOT / "recap-worker" / "recap-worker" / "config" / "graph.local.yaml"
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


def _print_summary(summary: OptimizationSummary, snapshot_path: pathlib.Path) -> None:
    sys.stdout.write(f"using snapshot: {snapshot_path}\n")
    sys.stdout.write(f"- best params: {_format_params(summary.best_params)}\n")
    sys.stdout.write(f"- accuracy: {summary.best_accuracy:.4f}\n")
    sys.stdout.write("\noptimization history:\n")
    for idx, (accuracy, params) in enumerate(summary.history, start=1):
        sys.stdout.write(f"{idx:02d}: accuracy={accuracy:.4f} params={_format_params(params)}\n")


def _read_yaml(path: pathlib.Path) -> Mapping[str, object]:
    if not path.exists():
        return {}
    try:
        content = path.read_text()
    except OSError as err:  # pragma: no cover (filesystem)
        msg = f"failed to read {path}: {err}"
        raise RuntimeError(msg) from err
    try:
        return yaml.safe_load(content) or {}
    except yaml.YAMLError as err:
        msg = f"failed to parse {path}: {err}"
        raise RuntimeError(msg) from err


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
        sys.stdout.write("graph config already matches the optimized thresholds\n")
        return
    sys.stdout.write(f"writing optimized thresholds to {path}\n")
    if dry_run:
        sys.stdout.write("(dry-run) -- skipping file write\n")
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(new_content)


def _resolve_cargo(cargo_bin: str) -> str:
    resolved = shutil.which(cargo_bin)
    if resolved is None:
        msg = f"cargo executable not found: {cargo_bin}"
        raise FileNotFoundError(msg)
    return resolved


def _run_subprocess(command: Sequence[str], env: Mapping[str, str], cwd: pathlib.Path) -> None:
    sys.stdout.write(f"→ running: {' '.join(command)}\n")
    subprocess.run(command, check=True, env=env, cwd=cwd)  # noqa: S603 -- executable resolved via shutil.which with fixed cargo subcommand args


@dataclass(frozen=True)
class _CargoContext:
    """Shared execution context for running Cargo subcommands."""

    cargo_bin: str
    env: Mapping[str, str]
    root: pathlib.Path
    dry_run: bool


def _run_tests(ctx: _CargoContext) -> None:
    if ctx.dry_run:
        sys.stdout.write("(dry-run) skipping cargo test\n")
        return
    recap_worker_dir = ctx.root / "recap-worker" / "recap-worker"
    _run_subprocess(
        [_resolve_cargo(ctx.cargo_bin), "test", "--", "pipeline::genre_refine"],
        ctx.env,
        recap_worker_dir,
    )


def _run_replay(ctx: _CargoContext, dataset: pathlib.Path, dsn: str) -> None:
    if ctx.dry_run:
        sys.stdout.write("(dry-run) skipping replay_genre_pipeline\n")
        return
    recap_worker_dir = ctx.root / "recap-worker" / "recap-worker"
    command = [
        _resolve_cargo(ctx.cargo_bin),
        "run",
        "--bin",
        "replay_genre_pipeline",
        "--",
        "--dataset",
        str(dataset),
        "--dry-run",
    ]
    replay_env = dict(ctx.env)
    replay_env["RECAP_DB_DSN"] = dsn
    _run_subprocess(command, replay_env, recap_worker_dir)


def main() -> None:
    args = parse_args()
    snapshot_path = find_latest_snapshot(args.snapshot_dir)
    df = prepare_dataframe(load_snapshot(snapshot_path))
    summary = run_bayes_optimization(df, args.iterations, args.seed)
    _print_summary(summary, snapshot_path)

    if args.update_config:
        existing_config = _read_yaml(args.graph_config)
        _write_graph_config(args.graph_config, summary.best_params, existing_config, args.dry_run)
    else:
        sys.stdout.write(
            "graph config update skipped (pass --update-config to persist the thresholds)\n"
        )

    env = os.environ.copy()
    env["GRAPH_CONFIG"] = str(args.graph_config)

    cargo_ctx = _CargoContext(
        cargo_bin=args.cargo_bin,
        env=env,
        root=ROOT,
        dry_run=args.dry_run,
    )
    if args.run_tests:
        _run_tests(cargo_ctx)

    if args.run_replay:
        dataset = args.replay_dataset
        if dataset is None:
            msg = "--run-replay requires --replay-dataset"
            raise SystemExit(msg)
        dsn = args.replay_dsn or os.environ.get("RECAP_DB_DSN")
        if not dsn:
            msg = "RECAP_DB_DSN must be set via --replay-dsn or environment for replay"
            raise SystemExit(msg)
        _run_replay(cargo_ctx, dataset, dsn)


if __name__ == "__main__":
    main()
