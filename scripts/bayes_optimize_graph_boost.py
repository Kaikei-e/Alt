#!/usr/bin/env python3
"""Bayes最適化を使ってGraph Boost関連の閾値を探索するためのスクリプト。"""

from __future__ import annotations

import argparse
import pathlib
import sys

from graph_boost_utils import (
    find_latest_snapshot,
    load_snapshot,
    prepare_dataframe,
    run_bayes_optimization,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Graph Boostの閾値チューニング（ベイズ最適化）")
    parser.add_argument(
        "--snapshot-dir",
        default=pathlib.Path(__file__).resolve().parents[1] / "analytics" / "graph_boost_snapshot",
        type=pathlib.Path,
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
        help="乱数シード",
    )
    return parser.parse_args()


def _format_params(params: tuple[float, float, int]) -> str:  # pragma: no cover (print helper)
    graph_margin, boost_threshold, tag_count_threshold = params
    return (
        f"(graph_margin={graph_margin:.3f}, boost_threshold={boost_threshold:.3f}, "
        f"tag_count_threshold={tag_count_threshold})"
    )


def main(args: argparse.Namespace | None = None) -> None:
    args = args or parse_args()
    snapshot_path = find_latest_snapshot(args.snapshot_dir)
    df = prepare_dataframe(load_snapshot(snapshot_path))

    sys.stdout.write(f"using snapshot: {snapshot_path}\n")
    sys.stdout.write(
        f"graph_boost vs weighted_score 件数 {df['strategy'].value_counts().to_dict()}\n"
    )

    # top_boost の分布を確認
    has_boost = (df["top_boost"] > 0).any()
    if not has_boost:
        sys.stdout.write("\n⚠️  警告: スナップショットの top_boost がすべて 0 です。\n")
        sys.stdout.write("   boost_threshold の最適化は意味を持ちません。\n")
        sys.stdout.write("   graph_margin と tag_count_threshold のみが有効です。\n")
    else:
        boost_count = (df["top_boost"] > 0).sum()
        sys.stdout.write(
            f"\n✓ top_boost > 0 の件数: {boost_count} / {len(df)} ({boost_count / len(df) * 100:.1f}%)\n"
        )

    summary = run_bayes_optimization(df, args.iterations, args.seed)

    sys.stdout.write("\n=== 最適解 ===\n")
    sys.stdout.write(f"- graph_margin ≒ {summary.best_params.graph_margin:.3f}\n")
    if has_boost:
        sys.stdout.write(f"- boost_threshold ≒ {summary.best_params.boost_threshold:.3f}\n")
    else:
        sys.stdout.write(
            f"- boost_threshold ≒ {summary.best_params.boost_threshold:.3f} (無視されます: top_boost がすべて 0)\n"
        )
    sys.stdout.write(f"- tag_count_threshold ≒ {summary.best_params.tag_count_threshold}\n")
    sys.stdout.write(f"- 推定 accuracy {summary.best_accuracy:.4f}\n")

    sys.stdout.write("\nBayes最適化の履歴（accuracy を上げたい場合は 1-戻り値）\n")
    for idx, (accuracy, params) in enumerate(summary.history, start=1):
        formatted = _format_params(
            (params.graph_margin, params.boost_threshold, params.tag_count_threshold)
        )
        sys.stdout.write(f"{idx:02d}: accuracy={accuracy:.4f} params={formatted}\n")


if __name__ == "__main__":
    main()
