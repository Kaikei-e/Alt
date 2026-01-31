#!/usr/bin/env python3
"""ジャンル分類の評価を実行するCLIスクリプト。"""

import argparse
import json
import sys
from pathlib import Path

# プロジェクトルートをパスに追加
project_root = Path(__file__).parent.parent
sys.path.insert(0, str(project_root))

from recap_subworker.services.evaluation import EvaluationService


def format_report(results: dict) -> str:
    """評価結果をMarkdown形式でフォーマット。"""

    lines = []
    lines.append("# Genre Classification Evaluation Report\n")

    # サマリー
    lines.append("## Summary\n")
    lines.append(f"- **Total Samples**: {results['total_samples']}")

    # Accuracy with CI
    acc_ci = results.get("accuracy_ci", {})
    if "width" in acc_ci:
        lines.append(
            f"- **Accuracy**: {results['accuracy']:.4f} "
            f"[{acc_ci['lower']:.4f}, {acc_ci['upper']:.4f}] "
            f"(95% CI, width: {acc_ci['width']:.4f})"
        )
    else:
        lines.append(
            f"- **Accuracy**: {results['accuracy']:.4f} "
            f"[{acc_ci.get('lower', 0):.4f}, {acc_ci.get('upper', 0):.4f}] "
            f"(95% CI)"
        )

    # Macro metrics with CI
    if "macro_precision" in results and isinstance(results["macro_precision"], dict):
        macro_prec = results["macro_precision"]
        macro_rec = results["macro_recall"]
        macro_f1 = results["macro_f1"]
        lines.append(
            f"- **Macro Precision**: {macro_prec.get('point', macro_prec):.4f} "
            f"[{macro_prec.get('lower', 0):.4f}, {macro_prec.get('upper', 0):.4f}] (95% CI)"
        )
        lines.append(
            f"- **Macro Recall**: {macro_rec.get('point', macro_rec):.4f} "
            f"[{macro_rec.get('lower', 0):.4f}, {macro_rec.get('upper', 0):.4f}] (95% CI)"
        )
        lines.append(
            f"- **Macro F1**: {macro_f1.get('point', macro_f1):.4f} "
            f"[{macro_f1.get('lower', 0):.4f}, {macro_f1.get('upper', 0):.4f}] (95% CI)"
        )
    else:
        lines.append(f"- **Macro Precision**: {results.get('macro_precision', 0):.4f}")
        lines.append(f"- **Macro Recall**: {results.get('macro_recall', 0):.4f}")
        lines.append(f"- **Macro F1**: {results.get('macro_f1', 0):.4f}")

    # Micro metrics
    lines.append(f"- **Micro Precision**: {results.get('micro_precision', 0):.4f}")
    lines.append(f"- **Micro Recall**: {results.get('micro_recall', 0):.4f}")
    lines.append(f"- **Micro F1**: {results.get('micro_f1', 0):.4f}")

    # 統合スコア
    if "integrated_score" in results:
        integrated = results["integrated_score"]
        lines.append(f"- **Integrated Score**: {integrated.get('score', 0):.4f}")
        lines.append("")

    # 効果サイズ
    if "effect_size" in results:
        effect = results["effect_size"]
        lines.append("## Effect Size\n")
        lines.append(f"- **Cramér's V**: {effect.get('cramers_v', 0):.4f}")
        lines.append("")

    # 統計的検出力
    if "statistical_power" in results:
        power_info = results["statistical_power"]
        lines.append("## Statistical Power Analysis\n")
        lines.append(f"- **Current Power**: {power_info.get('current_power', 0):.4f}")
        lines.append(f"- **Current Sample Size**: {power_info.get('current_sample_size', 0)}")
        lines.append(f"- **Required Sample Size** (for 80% power): {power_info.get('required_sample_size', 0)}")
        if power_info.get('current_sample_size', 0) < power_info.get('required_sample_size', 0):
            lines.append(
                f"  - ⚠️ **Warning**: Current sample size may be insufficient for reliable results."
            )
        lines.append("")

    # Cross-Validation結果
    if "cv_accuracy" in results:
        lines.append("## Cross-Validation Results\n")
        cv_acc = results["cv_accuracy"]
        cv_macro = results.get("cv_macro_f1", {})
        cv_micro = results.get("cv_micro_f1", {})
        lines.append(f"- **Accuracy**: {cv_acc.get('mean', 0):.4f} ± {cv_acc.get('std', 0):.4f}")
        lines.append(f"- **Macro F1**: {cv_macro.get('mean', 0):.4f} ± {cv_macro.get('std', 0):.4f}")
        lines.append(f"- **Micro F1**: {cv_micro.get('mean', 0):.4f} ± {cv_micro.get('std', 0):.4f}")
        lines.append(f"- **Number of Folds**: {results.get('n_folds', 0)}")

        # CV std check (stability indicator)
        cv_std = cv_macro.get("std", 0)
        if cv_std < 0.03:
            lines.append(f"- **Stability**: OK (CV std {cv_std:.4f} < 0.03)")
        else:
            lines.append(f"- **Stability**: WARNING - high variance (CV std {cv_std:.4f} >= 0.03)")
        lines.append("")

    # Train/Test Gap (Overfitting Indicator)
    if "train_test_gap" in results:
        lines.append("## Overfitting Analysis\n")
        train_f1 = results.get("train_f1", 0)
        test_f1 = results.get("test_f1", 0)
        gap = results.get("train_test_gap", 0)
        lines.append(f"- **Train Macro F1**: {train_f1:.4f}")
        lines.append(f"- **Test Macro F1**: {test_f1:.4f}")
        lines.append(f"- **Train/Test Gap**: {gap:.4f}")
        if gap < 0.10:
            lines.append("- **Status**: OK (gap < 0.10)")
        else:
            lines.append("- **Status**: WARNING - potential overfitting (gap >= 0.10)")
        lines.append("")

    # ジャンル別メトリクス
    lines.append("## Per-Genre Metrics\n")
    lines.append(
        "| Genre | Precision | Precision CI | Recall | Recall CI | F1 | Support | Warning |"
    )
    lines.append("|-------|-----------|--------------|--------|-----------|----|---------|---------|")

    per_genre = results.get("per_genre_metrics", {})
    for genre in sorted(per_genre.keys()):
        metrics = per_genre[genre]
        prec_ci = metrics.get("precision_ci", {})
        recall_ci = metrics.get("recall_ci", {})
        support = metrics.get("support", metrics.get("tp", 0) + metrics.get("fn", 0))
        warning = "⚠️" if metrics.get("warning", False) else ""

        prec_ci_str = (
            f"[{prec_ci.get('lower', 0):.3f}, {prec_ci.get('upper', 0):.3f}]"
            if prec_ci
            else "-"
        )
        recall_ci_str = (
            f"[{recall_ci.get('lower', 0):.3f}, {recall_ci.get('upper', 0):.3f}]"
            if recall_ci
            else "-"
        )

        lines.append(
            f"| {genre} | {metrics['precision']:.4f} | {prec_ci_str} | "
            f"{metrics['recall']:.4f} | {recall_ci_str} | {metrics['f1']:.4f} | "
            f"{support} | {warning} |"
        )
    lines.append("")

    # 混同行列
    lines.append("## Confusion Matrix\n")
    cm = results.get("confusion_matrix", {})
    labels = cm.get("labels", [])
    matrix = cm.get("matrix", [])

    if labels and matrix:
        # ヘッダー
        header = "| | " + " | ".join(labels) + " |"
        lines.append(header)
        lines.append("|" + "---|" * (len(labels) + 1))

        # 各行
        for i, label in enumerate(labels):
            row = (
                f"| {label} | "
                + " | ".join(str(matrix[i][j]) for j in range(len(labels)))
                + " |"
            )
            lines.append(row)
        lines.append("")

    return "\n".join(lines)


def format_report_by_language(results: dict) -> str:
    lines = []
    lines.append("# Genre Classification Evaluation Report (By Language)\n")

    for lang in ["ja", "en"]:
        if lang in results:
            lines.append(f"## Language: {lang.upper()}\n")
            lang_res = results[lang]
            if "error" in lang_res:
                lines.append(f"Error: {lang_res['error']}\n")
                continue

            # Re-use existing formatter logic by "stripping" headers or just calling it
            # But existing formatter adds headers. Let's just append the body.
            # Simplified version of calling format_report on sub-result
            sub_report = format_report(lang_res)
            # Remove title if present
            sub_report = sub_report.replace("# Genre Classification Evaluation Report\n", "")
            lines.append(sub_report)
            lines.append("\n---\n")

    return "\n".join(lines)

def main():
    """メイン関数。"""
    parser = argparse.ArgumentParser(
        description="Evaluate genre classification on golden dataset"
    )
    parser.add_argument(
        "--golden-data",
        type=str,
        default="/app/data/golden_classification.json",
        help="Path to golden dataset JSON file",
    )
    parser.add_argument(
        "--weights",
        type=str,
        default=None,
        help="Path to default genre classifier weights JSON file",
    )
    # JA/EN Specific Args
    parser.add_argument("--weights-ja", type=str, default="/app/data/genre_classifier_ja.joblib", help="JA weights")
    parser.add_argument("--weights-en", type=str, default="/app/data/genre_classifier_en.joblib", help="EN weights")
    parser.add_argument("--vectorizer-ja", type=str, default="/app/data/dataset/ja/tfidf_vectorizer.joblib", help="JA vectorizer")
    parser.add_argument("--vectorizer-en", type=str, default="/app/data/dataset/en/tfidf_vectorizer.joblib", help="EN vectorizer")
    parser.add_argument("--thresholds-ja", type=str, default="/app/data/genre_thresholds_ja.json", help="JA thresholds")
    parser.add_argument("--thresholds-en", type=str, default="/app/data/genre_thresholds_en.json", help="EN thresholds")

    parser.add_argument(
        "--output",
        type=str,
        default=None,
        help="Output file path (default: stdout)",
    )
    parser.add_argument(
        "--bootstrap",
        action="store_true",
        default=True,
        help="Use Bootstrap method for confidence intervals (default: True)",
    )
    parser.add_argument(
        "--no-bootstrap",
        dest="bootstrap",
        action="store_false",
        help="Disable Bootstrap method",
    )
    parser.add_argument(
        "--n-bootstrap",
        type=int,
        default=1000,
        help="Number of bootstrap resamples (default: 1000)",
    )
    parser.add_argument(
        "--cross-validation",
        action="store_true",
        help="Use Stratified K-Fold Cross-Validation",
    )
    parser.add_argument(
        "--n-folds",
        type=int,
        default=5,
        help="Number of folds for cross-validation (default: 5)",
    )

    args = parser.parse_args()

    # 評価を実行
    try:
        service = EvaluationService(
            weights_path=args.weights,
            weights_ja_path=args.weights_ja,
            weights_en_path=args.weights_en,
            vectorizer_ja_path=args.vectorizer_ja,
            vectorizer_en_path=args.vectorizer_en,
            thresholds_ja_path=args.thresholds_ja,
            thresholds_en_path=args.thresholds_en,
            use_bootstrap=args.bootstrap,
            n_bootstrap=args.n_bootstrap,
            use_cross_validation=args.cross_validation,
            n_folds=args.n_folds,
        )

        # Evaluate by language (Dual Model)
        results = service.evaluate_by_language(args.golden_data)

        # レポートを生成
        report = format_report_by_language(results)

        # 出力
        if args.output:
            with open(args.output, "w", encoding="utf-8") as f:
                f.write(report)
            print(f"Report written to {args.output}")
        else:
            print(report)

        # JSON形式でも出力（デバッグ用）
        if args.output:
            json_output = args.output.replace(".md", ".json")
            if json_output == args.output:
                json_output = args.output + ".json"
            with open(json_output, "w", encoding="utf-8") as f:
                json.dump(results, f, indent=2, ensure_ascii=False)
            print(f"JSON results written to {json_output}")

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        import traceback

        traceback.print_exc()
        sys.exit(1)

if __name__ == "__main__":
    main()
