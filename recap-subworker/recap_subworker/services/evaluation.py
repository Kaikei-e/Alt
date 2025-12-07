"""ジャンル分類の評価サービス。"""

import json
import os
from collections import defaultdict
from pathlib import Path
from typing import Callable, Dict, List, Optional, Tuple

import numpy as np
from sklearn.metrics import (
    accuracy_score,
    confusion_matrix,
    f1_score,
    precision_score,
    recall_score,
)
from sklearn.model_selection import StratifiedKFold

from ..domain.analysis.stats import (
    bootstrap_confidence_interval,
    calculate_confidence_interval,
    calculate_required_sample_size,
    calculate_statistical_power,
    clopper_pearson_interval,
    cohens_d,
    cramers_v,
)
from ..domain.classification import (
    ClassificationLanguage,
    GenreClassifier,
    TokenPipeline,
)

# 許可されたベースディレクトリ
ALLOWED_BASE_DIRS = [
    Path("/app/data"),
    Path("/app/resources"),
]


from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import text

class EvaluationService:
    """評価サービス。"""

    def __init__(
        self,
        weights_path: Optional[str] = None,
        use_bootstrap: bool = True,
        n_bootstrap: int = 1000,
        use_cross_validation: bool = False,
        n_folds: int = 5,
    ):
        """初期化。

        Args:
            weights_path: 重みファイルのパス（オプション）
            use_bootstrap: Bootstrap法を使用するか（デフォルト: True）
            n_bootstrap: Bootstrapリサンプリング回数（デフォルト: 1000）
            use_cross_validation: Cross-Validationを使用するか（デフォルト: False）
            n_folds: Cross-ValidationのFold数（デフォルト: 5）
        """
        self.classifier = GenreClassifier(weights_path=weights_path)
        self.token_pipeline = TokenPipeline()
        self.use_bootstrap = use_bootstrap
        self.n_bootstrap = n_bootstrap
        self.use_cross_validation = use_cross_validation
        self.n_folds = n_folds

    def evaluate(
        self, golden_dataset_path: str
    ) -> Dict:
        """Golden Datasetを使用して評価を実行。

        Args:
            golden_dataset_path: Golden Dataset JSONファイルのパス（既に検証済みであることを想定）

        Returns:
            評価結果の辞書
        """
        # パスを検証（念のため）
        path_obj = Path(golden_dataset_path)
        if path_obj.is_absolute():
            path_resolved = path_obj.resolve()
            # 許可されたディレクトリ内にあるか確認
            is_allowed = False
            for base_dir in ALLOWED_BASE_DIRS:
                base_dir_resolved = base_dir.resolve()
                try:
                    path_resolved.relative_to(base_dir_resolved)
                    is_allowed = True
                    break
                except ValueError:
                    continue

            if not is_allowed:
                raise ValueError(
                    f"Path '{golden_dataset_path}' is not within allowed directories: "
                    f"{[str(d) for d in ALLOWED_BASE_DIRS]}"
                )

        # Golden Datasetを読み込み
        with open(golden_dataset_path, "r", encoding="utf-8") as f:
            golden_data = json.load(f)

        if not golden_data:
            raise ValueError("Golden dataset is empty")

        # 新しいスキーマ（items配列あり）とレガシースキーマ（直接配列）の両方に対応
        if isinstance(golden_data, dict) and "items" in golden_data:
            items = golden_data["items"]
        elif isinstance(golden_data, list):
            items = golden_data  # レガシー形式
        else:
            raise ValueError("Golden dataset has invalid structure")

        if not items:
            raise ValueError("Golden dataset contains no items")

        if self.use_cross_validation:
            return self._evaluate_with_cv(items)
        else:
            return self._evaluate_single(items)

    def _evaluate_single(self, golden_data: List[Dict]) -> Dict:
        """単一の評価を実行。"""
        # 評価を実行
        all_expected = []
        all_predicted = []
        per_genre_tp = defaultdict(int)
        per_genre_fp = defaultdict(int)
        per_genre_fn = defaultdict(int)
        sample_results = []  # Bootstrap用

        for item in golden_data:
            result = self._evaluate_item(item)
            if result is None:
                continue

            expected_genres, predicted_genres, is_correct = result
            sample_results.append(1 if is_correct else 0)

            # マルチラベル評価のため、各ジャンルについてTP/FP/FNを計算
            expected_set = set(g.lower() for g in expected_genres)
            predicted_set = set(g.lower() for g in predicted_genres)

            all_genres = expected_set | predicted_set

            for genre in all_genres:
                expected_has = genre in expected_set
                predicted_has = genre in predicted_set

                if expected_has and predicted_has:
                    per_genre_tp[genre] += 1
                elif not expected_has and predicted_has:
                    per_genre_fp[genre] += 1
                elif expected_has and not predicted_has:
                    per_genre_fn[genre] += 1

            # 全体の精度計算用（主要ジャンルを比較）
            primary_expected = expected_genres[0].lower() if expected_genres else "other"
            primary_predicted = (
                predicted_genres[0].lower() if predicted_genres else "other"
            )
            all_expected.append(primary_expected)
            all_predicted.append(primary_predicted)

        # メトリクスを計算
        accuracy = accuracy_score(all_expected, all_predicted)

        # 信頼区間の計算
        if self.use_bootstrap and len(sample_results) >= 10:
            accuracy_data = np.array(sample_results, dtype=float)
            accuracy_ci = bootstrap_confidence_interval(
                accuracy_data,
                lambda x: np.mean(x),
                confidence=0.95,
                n_bootstrap=self.n_bootstrap,
                method="bca",
            )
        else:
            accuracy_ci = calculate_confidence_interval(
                int(accuracy * len(all_expected)), len(all_expected)
            )

        # ジャンル別メトリクス（Bootstrap法とClopper-Pearson intervalを使用）
        per_genre_metrics = self._calculate_per_genre_metrics(
            per_genre_tp, per_genre_fp, per_genre_fn
        )

        # Macro平均（Bootstrap法を使用）
        macro_metrics = self._calculate_macro_metrics(per_genre_metrics)

        # Micro平均
        micro_metrics = self._calculate_micro_metrics(per_genre_tp, per_genre_fp, per_genre_fn)

        # 混同行列
        unique_genres = sorted(set(all_expected) | set(all_predicted))
        cm = confusion_matrix(all_expected, all_predicted, labels=unique_genres)

        # 効果サイズ
        effect_size = cramers_v(cm)

        # 統計的検出力
        total_samples = len(golden_data)
        if total_samples > 0:
            # 効果サイズ0.2（小さい効果）を検出するための検出力
            power = calculate_statistical_power(0.2, total_samples)
            required_n = calculate_required_sample_size(0.2, power=0.8)
        else:
            power = 0.0
            required_n = 0

        # 統合スコア
        integrated_score = self._calculate_integrated_score(
            accuracy, macro_metrics, micro_metrics
        )

        return {
            "accuracy": accuracy,
            "accuracy_ci": {
                "point": accuracy_ci[0],
                "lower": accuracy_ci[1],
                "upper": accuracy_ci[2],
                "width": accuracy_ci[2] - accuracy_ci[1],
            },
            "macro_precision": macro_metrics["precision"],
            "macro_recall": macro_metrics["recall"],
            "macro_f1": macro_metrics["f1"],
            "macro_metrics": macro_metrics,
            "micro_precision": micro_metrics["precision"],
            "micro_recall": micro_metrics["recall"],
            "micro_f1": micro_metrics["f1"],
            "per_genre_metrics": per_genre_metrics,
            "confusion_matrix": {
                "labels": unique_genres,
                "matrix": cm.tolist(),
            },
            "effect_size": {
                "cramers_v": effect_size,
            },
            "statistical_power": {
                "current_power": power,
                "required_sample_size": required_n,
                "current_sample_size": total_samples,
            },
            "integrated_score": integrated_score,
            "total_samples": total_samples,
            "total_tp": sum(per_genre_tp.values()),
            "total_fp": sum(per_genre_fp.values()),
            "total_fn": sum(per_genre_fn.values()),
        }

    def _evaluate_with_cv(self, golden_data: List[Dict]) -> Dict:
        """Stratified K-Fold Cross-Validationを使用して評価を実行。"""
        # データを準備
        items = []
        labels = []
        for item in golden_data:
            expected_genres = item.get("expected_genres", [])
            if not expected_genres:
                continue
            items.append(item)
            labels.append(expected_genres[0].lower())  # 主要ジャンル

        if len(items) < self.n_folds:
            # サンプル数が少ない場合は通常の評価にフォールバック
            return self._evaluate_single(golden_data)

        # Stratified K-Fold
        skf = StratifiedKFold(n_splits=self.n_folds, shuffle=True, random_state=42)
        fold_results = []

        for fold_idx, (train_idx, test_idx) in enumerate(skf.split(items, labels)):
            test_items = [items[i] for i in test_idx]
            fold_result = self._evaluate_single(test_items)
            fold_results.append(fold_result)

        # 各Foldの結果を集約
        return self._aggregate_cv_results(fold_results)

    def _evaluate_item(
        self, item: Dict
    ) -> Optional[Tuple[List[str], List[str], bool]]:
        """単一アイテムを評価。"""
        # データ形式の確認（content_en優先、次にcontent_ja、最後にcontent）
        content = None
        if "content_en" in item and item.get("content_en"):
            content = item["content_en"]
        elif "content_ja" in item and item.get("content_ja"):
            content = item["content_ja"]
        elif "content" in item:
            content = item["content"]

        if content:
            title = ""
            body = content
        elif "title" in item and "body" in item:
            title = item.get("title", "")
            body = item["body"]
        else:
            return None

        expected_genres = item.get("expected_genres", [])
        if not expected_genres:
            return None

        # 分類を実行
        language = ClassificationLanguage.UNKNOWN
        normalized = self.token_pipeline.preprocess(title, body, language)
        feature_vector = self.classifier.feature_extractor.extract(normalized.tokens)
        predictions = self.classifier.predict(feature_vector)

        # トップジャンルを取得（スコアが高い順）
        predicted_genres = [pred[0] for pred in predictions[:3]]  # 上位3つ

        # 主要ジャンルが一致するか
        primary_expected = expected_genres[0].lower()
        primary_predicted = predicted_genres[0].lower() if predicted_genres else "other"
        is_correct = primary_expected == primary_predicted

        return (expected_genres, predicted_genres, is_correct)

    def _calculate_per_genre_metrics(
        self, per_genre_tp: Dict, per_genre_fp: Dict, per_genre_fn: Dict
    ) -> Dict:
        """ジャンル別メトリクスを計算（Bootstrap法とClopper-Pearson intervalを使用）。"""
        per_genre_metrics = {}
        all_genres = set(per_genre_tp.keys()) | set(per_genre_fp.keys()) | set(
            per_genre_fn.keys()
        )

        for genre in all_genres:
            tp = per_genre_tp[genre]
            fp = per_genre_fp[genre]
            fn = per_genre_fn[genre]

            precision = tp / (tp + fp) if (tp + fp) > 0 else 0.0
            recall = tp / (tp + fn) if (tp + fn) > 0 else 0.0
            f1 = (
                2 * precision * recall / (precision + recall)
                if (precision + recall) > 0
                else 0.0
            )

            # 信頼区間の計算
            # サンプル数が少ない場合はClopper-Pearson intervalを使用
            support = tp + fn
            if support < 30:
                # Clopper-Pearson interval（小サンプル用）
                recall_ci = clopper_pearson_interval(tp, support)
                precision_ci = (
                    clopper_pearson_interval(tp, tp + fp) if (tp + fp) > 0 else (0.0, 0.0, 0.0)
                )
            else:
                # Bootstrap法
                if self.use_bootstrap:
                    # Bootstrap用のデータを準備（簡易版）
                    recall_data = np.array([1.0] * tp + [0.0] * fn)
                    precision_data = (
                        np.array([1.0] * tp + [0.0] * fp) if (tp + fp) > 0 else np.array([])
                    )

                    if len(recall_data) >= 10:
                        recall_ci = bootstrap_confidence_interval(
                            recall_data,
                            lambda x: np.mean(x),
                            n_bootstrap=self.n_bootstrap,
                            method="bca",
                        )
                    else:
                        recall_ci = clopper_pearson_interval(tp, support)

                    if len(precision_data) >= 10:
                        precision_ci = bootstrap_confidence_interval(
                            precision_data,
                            lambda x: np.mean(x),
                            n_bootstrap=self.n_bootstrap,
                            method="bca",
                        )
                    else:
                        precision_ci = (
                            clopper_pearson_interval(tp, tp + fp)
                            if (tp + fp) > 0
                            else (0.0, 0.0, 0.0)
                        )
                else:
                    recall_ci = clopper_pearson_interval(tp, support)
                    precision_ci = (
                        clopper_pearson_interval(tp, tp + fp) if (tp + fp) > 0 else (0.0, 0.0, 0.0)
                    )

            per_genre_metrics[genre] = {
                "tp": tp,
                "fp": fp,
                "fn": fn,
                "support": support,
                "precision": precision,
                "precision_ci": {
                    "point": precision_ci[0],
                    "lower": precision_ci[1],
                    "upper": precision_ci[2],
                },
                "recall": recall,
                "recall_ci": {
                    "point": recall_ci[0],
                    "lower": recall_ci[1],
                    "upper": recall_ci[2],
                },
                "f1": f1,
                "warning": support < 30,  # サンプル数が少ない場合の警告
            }

        return per_genre_metrics

    def _calculate_macro_metrics(self, per_genre_metrics: Dict) -> Dict:
        """Macro平均メトリクスを計算（Bootstrap法を使用）。"""
        if not per_genre_metrics:
            return {"precision": 0.0, "recall": 0.0, "f1": 0.0}

        precisions = [m["precision"] for m in per_genre_metrics.values()]
        recalls = [m["recall"] for m in per_genre_metrics.values()]
        f1_scores = [m["f1"] for m in per_genre_metrics.values()]

        macro_precision = np.mean(precisions)
        macro_recall = np.mean(recalls)
        macro_f1 = np.mean(f1_scores)

        # Bootstrap法で信頼区間を計算
        if self.use_bootstrap and len(precisions) >= 10:
            precision_ci = bootstrap_confidence_interval(
                np.array(precisions),
                lambda x: np.mean(x),
                n_bootstrap=self.n_bootstrap,
                method="bca",
            )
            recall_ci = bootstrap_confidence_interval(
                np.array(recalls),
                lambda x: np.mean(x),
                n_bootstrap=self.n_bootstrap,
                method="bca",
            )
            f1_ci = bootstrap_confidence_interval(
                np.array(f1_scores),
                lambda x: np.mean(x),
                n_bootstrap=self.n_bootstrap,
                method="bca",
            )
        else:
            # 簡易的な信頼区間
            precision_std = np.std(precisions) / np.sqrt(len(precisions))
            recall_std = np.std(recalls) / np.sqrt(len(recalls))
            f1_std = np.std(f1_scores) / np.sqrt(len(f1_scores))
            z = 1.96  # 95%信頼区間

            precision_ci = (macro_precision, macro_precision - z * precision_std, macro_precision + z * precision_std)
            recall_ci = (macro_recall, macro_recall - z * recall_std, macro_recall + z * recall_std)
            f1_ci = (macro_f1, macro_f1 - z * f1_std, macro_f1 + z * f1_std)

        return {
            "precision": macro_precision,
            "precision_ci": {
                "point": precision_ci[0],
                "lower": precision_ci[1],
                "upper": precision_ci[2],
            },
            "recall": macro_recall,
            "recall_ci": {
                "point": recall_ci[0],
                "lower": recall_ci[1],
                "upper": recall_ci[2],
            },
            "f1": macro_f1,
            "f1_ci": {
                "point": f1_ci[0],
                "lower": f1_ci[1],
                "upper": f1_ci[2],
            },
        }

    def _calculate_micro_metrics(
        self, per_genre_tp: Dict, per_genre_fp: Dict, per_genre_fn: Dict
    ) -> Dict:
        """Micro平均メトリクスを計算。"""
        total_tp = sum(per_genre_tp.values())
        total_fp = sum(per_genre_fp.values())
        total_fn = sum(per_genre_fn.values())

        micro_precision = (
            total_tp / (total_tp + total_fp) if (total_tp + total_fp) > 0 else 0.0
        )
        micro_recall = (
            total_tp / (total_tp + total_fn) if (total_tp + total_fn) > 0 else 0.0
        )
        micro_f1 = (
            2 * micro_precision * micro_recall / (micro_precision + micro_recall)
            if (micro_precision + micro_recall) > 0
            else 0.0
        )

        return {
            "precision": micro_precision,
            "recall": micro_recall,
            "f1": micro_f1,
        }

    def _calculate_integrated_score(
        self, accuracy: float, macro_metrics: Dict, micro_metrics: Dict
    ) -> Dict:
        """統合評価スコアを計算。

        重み: Accuracy 0.3, Macro F1 0.4, Micro F1 0.3
        """
        weights = {"accuracy": 0.3, "macro_f1": 0.4, "micro_f1": 0.3}
        integrated = (
            weights["accuracy"] * accuracy
            + weights["macro_f1"] * macro_metrics["f1"]
            + weights["micro_f1"] * micro_metrics["f1"]
        )

        return {
            "score": integrated,
            "weights": weights,
        }

    def _aggregate_cv_results(self, fold_results: List[Dict]) -> Dict:
        """Cross-Validationの結果を集約。"""
        if not fold_results:
            return {}

        # 各メトリクスの平均と標準偏差を計算
        accuracies = [r["accuracy"] for r in fold_results]
        macro_f1s = [r["macro_f1"] for r in fold_results]
        micro_f1s = [r["micro_f1"] for r in fold_results]

        # ベースとなる結果（最初のFold）
        base_result = fold_results[0].copy()

        # 平均と標準偏差を追加
        base_result["cv_accuracy"] = {
            "mean": np.mean(accuracies),
            "std": np.std(accuracies),
        }
        base_result["cv_macro_f1"] = {
            "mean": np.mean(macro_f1s),
            "std": np.std(macro_f1s),
        }
        base_result["cv_micro_f1"] = {
            "mean": np.mean(micro_f1s),
            "std": np.std(micro_f1s),
        }
        base_result["n_folds"] = self.n_folds

        return base_result

    async def save_metrics(
        self,
        metrics: Dict,
        session: AsyncSession,
        job_id: Optional[str] = None,
        metric_type: str = "classification_eval",
    ) -> None:
        """評価指標をデータベースに保存。

        Args:
            metrics: 評価結果の辞書
            session: データベースセッション
            job_id: 関連するジョブID（オプション）
            metric_type: メトリクスタイプ（デフォルト: classification_eval）
        """
        stmt = text(
            """
            INSERT INTO recap_system_metrics (job_id, metric_type, metrics, timestamp)
            VALUES (:job_id, :metric_type, :metrics, NOW())
            """
        )

        await session.execute(
            stmt,
            {
                "job_id": job_id,
                "metric_type": metric_type,
                "metrics": json.dumps(metrics),
            },
        )
        await session.commit()
