"""Genre classification evaluation endpoints."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel, Field
from uuid import UUID

from ...db.dao import SubworkerDAO
from ...db.session import get_session
from ...infra.config import Settings
from ...services.evaluation import EvaluationService
from ..deps import get_settings_dep

router = APIRouter(prefix="/v1/evaluation", tags=["evaluation"])

# 許可されたベースディレクトリ
ALLOWED_BASE_DIRS = [
    Path("/app/data"),
    Path("/app/resources"),
]


def validate_path(user_path: str, base_dirs: list[Path]) -> Path:
    """ユーザー入力のパスを検証し、安全なPathオブジェクトを返す。

    Args:
        user_path: ユーザーが指定したパス
        base_dirs: 許可されたベースディレクトリのリスト

    Returns:
        検証済みのPathオブジェクト

    Raises:
        HTTPException: パスが許可されたディレクトリ外にある場合
    """
    # パスを正規化
    normalized = os.path.normpath(user_path)

    # 絶対パスに変換
    if os.path.isabs(normalized):
        full_path = Path(normalized)
    else:
        # 相対パスの場合は最初の許可ディレクトリをベースとして使用
        full_path = Path(base_dirs[0]) / normalized
        full_path = Path(os.path.normpath(str(full_path)))

    # 各許可ディレクトリに対して、パスがそのディレクトリ内にあるか確認
    for base_dir in base_dirs:
        base_dir_resolved = base_dir.resolve()
        full_path_resolved = full_path.resolve()

        try:
            # パスがベースディレクトリ内にあるか確認
            full_path_resolved.relative_to(base_dir_resolved)
            return full_path_resolved
        except ValueError:
            # このベースディレクトリには含まれていない
            continue

    # どの許可ディレクトリにも含まれていない
    raise HTTPException(
        status_code=400,
        detail=f"Path '{user_path}' is not within allowed directories: {[str(d) for d in base_dirs]}",
    )


class EvaluateRequest(BaseModel):
    """評価リクエスト。"""

    golden_data_path: Optional[str] = Field(
        None, description="Golden dataset JSONファイルのパス（デフォルト: /app/data/golden_classification.json）"
    )
    weights_path: Optional[str] = Field(None, description="重みファイルのパス（オプション）")
    use_bootstrap: bool = Field(True, description="Bootstrap法を使用するか")
    n_bootstrap: int = Field(1000, description="Bootstrapリサンプリング回数", ge=100, le=10000)
    use_cross_validation: bool = Field(False, description="Cross-Validationを使用するか")
    n_folds: int = Field(5, description="Cross-ValidationのFold数", ge=2, le=10)
    save_to_db: bool = Field(True, description="評価結果をデータベースに保存するか")


class ConfidenceInterval(BaseModel):
    """信頼区間。"""

    point: float = Field(..., description="点推定値")
    lower: float = Field(..., description="下限")
    upper: float = Field(..., description="上限")
    width: Optional[float] = Field(None, description="信頼区間の幅")


class PerGenreMetric(BaseModel):
    """ジャンル別メトリクス。"""

    tp: int = Field(..., description="True Positives")
    fp: int = Field(..., description="False Positives")
    fn: int = Field(..., description="False Negatives")
    support: int = Field(..., description="サポート数（TP + FN）")
    precision: float = Field(..., description="Precision")
    precision_ci: Optional[ConfidenceInterval] = Field(None, description="Precisionの信頼区間")
    recall: float = Field(..., description="Recall")
    recall_ci: Optional[ConfidenceInterval] = Field(None, description="Recallの信頼区間")
    f1: float = Field(..., description="F1スコア")
    warning: bool = Field(False, description="サンプル数不足の警告")


class MacroMetrics(BaseModel):
    """Macro平均メトリクス。"""

    precision: float = Field(..., description="Macro Precision")
    precision_ci: Optional[ConfidenceInterval] = Field(None, description="Precisionの信頼区間")
    recall: float = Field(..., description="Macro Recall")
    recall_ci: Optional[ConfidenceInterval] = Field(None, description="Recallの信頼区間")
    f1: float = Field(..., description="Macro F1")
    f1_ci: Optional[ConfidenceInterval] = Field(None, description="F1の信頼区間")


class StatisticalPower(BaseModel):
    """統計的検出力情報。"""

    current_power: float = Field(..., description="現在の検出力")
    required_sample_size: int = Field(..., description="目標検出力達成に必要なサンプルサイズ")
    current_sample_size: int = Field(..., description="現在のサンプルサイズ")


class IntegratedScore(BaseModel):
    """統合評価スコア。"""

    score: float = Field(..., description="統合スコア")
    weights: dict[str, float] = Field(..., description="各メトリクスの重み")


class EvaluateResponse(BaseModel):
    """評価レスポンス。"""

    run_id: Optional[UUID] = Field(None, description="データベースに保存されたrun_id")
    accuracy: float = Field(..., description="Accuracy")
    accuracy_ci: ConfidenceInterval = Field(..., description="Accuracyの信頼区間")
    macro_precision: float = Field(..., description="Macro Precision")
    macro_recall: float = Field(..., description="Macro Recall")
    macro_f1: float = Field(..., description="Macro F1")
    macro_metrics: Optional[MacroMetrics] = Field(None, description="Macro平均メトリクス（詳細）")
    micro_precision: float = Field(..., description="Micro Precision")
    micro_recall: float = Field(..., description="Micro Recall")
    micro_f1: float = Field(..., description="Micro F1")
    per_genre_metrics: dict[str, PerGenreMetric] = Field(..., description="ジャンル別メトリクス")
    confusion_matrix: dict[str, list] = Field(..., description="混同行列")
    effect_size: Optional[dict[str, float]] = Field(None, description="効果サイズ")
    statistical_power: Optional[StatisticalPower] = Field(None, description="統計的検出力")
    integrated_score: Optional[IntegratedScore] = Field(None, description="統合評価スコア")
    total_samples: int = Field(..., description="総サンプル数")
    total_tp: int = Field(..., description="総True Positives")
    total_fp: int = Field(..., description="総False Positives")
    total_fn: int = Field(..., description="総False Negatives")
    cv_results: Optional[dict] = Field(None, description="Cross-Validation結果")


@router.post("/genres", response_model=EvaluateResponse)
async def evaluate_genres(
    request: EvaluateRequest,
    settings: Settings = Depends(get_settings_dep),
    session=Depends(get_session),
) -> EvaluateResponse:
    """ジャンル分類の評価を実行。

    Golden Datasetを使用してジャンル分類器の精度を評価し、
    統計的に厳密な評価結果を返します。
    評価結果はrecap-dbに保存されます。
    """
    # デフォルトパスの設定
    if request.golden_data_path is None:
        golden_data_path = Path("/app/data/golden_classification.json")
    else:
        # ユーザー入力のパスを検証
        golden_data_path = validate_path(request.golden_data_path, ALLOWED_BASE_DIRS)

    if not golden_data_path.exists():
        raise HTTPException(
            status_code=404,
            detail=f"Golden dataset file not found: {golden_data_path}",
        )

    if request.weights_path:
        # ユーザー入力のパスを検証
        weights_path = validate_path(request.weights_path, ALLOWED_BASE_DIRS)
        if not weights_path.exists():
            raise HTTPException(
                status_code=404,
                detail=f"Weights file not found: {request.weights_path}",
            )
    else:
        weights_path = None

    try:
        # 評価サービスを初期化
        service = EvaluationService(
            weights_path=str(weights_path) if weights_path else None,
            use_bootstrap=request.use_bootstrap,
            n_bootstrap=request.n_bootstrap,
            use_cross_validation=request.use_cross_validation,
            n_folds=request.n_folds,
        )

        # 評価を実行
        results = service.evaluate(str(golden_data_path))

        # データベースに保存
        run_id: UUID | None = None
        if request.save_to_db:
            try:
                dao = SubworkerDAO(session)
                # ジャンル別メトリクスをリスト形式に変換
                per_genre_list = [
                    {
                        "genre": genre,
                        "tp": metrics["tp"],
                        "fp": metrics["fp"],
                        "fn": metrics["fn"],
                        "precision": metrics["precision"],
                        "recall": metrics["recall"],
                        "f1": metrics["f1"],
                    }
                    for genre, metrics in results["per_genre_metrics"].items()
                ]

                macro_metrics = results.get("macro_metrics", {})
                run_id = await dao.save_genre_evaluation(
                    dataset_path=str(golden_data_path),
                    total_items=results["total_samples"],
                    macro_precision=results["macro_precision"],
                    macro_recall=results["macro_recall"],
                    macro_f1=results["macro_f1"],
                    summary_tp=results["total_tp"],
                    summary_fp=results["total_fp"],
                    summary_fn=results["total_fn"],
                    micro_precision=results.get("micro_precision"),
                    micro_recall=results.get("micro_recall"),
                    micro_f1=results.get("micro_f1"),
                    weighted_f1=None,  # 現在の実装では計算していない
                    macro_f1_valid=None,  # 現在の実装では計算していない
                    valid_genre_count=None,
                    undefined_genre_count=None,
                    per_genre_metrics=per_genre_list,
                )
            except Exception as e:
                # データベース保存に失敗しても評価結果は返す
                import structlog

                logger = structlog.get_logger(__name__)
                logger.warning("Failed to save evaluation results to database", error=str(e))

        # レスポンスを構築
        macro_metrics_dict = results.get("macro_metrics", {})
        return EvaluateResponse(
            run_id=run_id,
            accuracy=results["accuracy"],
            accuracy_ci=ConfidenceInterval(**results["accuracy_ci"]),
            macro_precision=results["macro_precision"],
            macro_recall=results["macro_recall"],
            macro_f1=results["macro_f1"],
            macro_metrics=(
                MacroMetrics(
                    precision=macro_metrics_dict["precision"],
                    precision_ci=ConfidenceInterval(**macro_metrics_dict["precision_ci"]),
                    recall=macro_metrics_dict["recall"],
                    recall_ci=ConfidenceInterval(**macro_metrics_dict["recall_ci"]),
                    f1=macro_metrics_dict["f1"],
                    f1_ci=ConfidenceInterval(**macro_metrics_dict["f1_ci"]),
                )
                if macro_metrics_dict and "precision_ci" in macro_metrics_dict
                else None
            ),
            micro_precision=results["micro_precision"],
            micro_recall=results["micro_recall"],
            micro_f1=results["micro_f1"],
            per_genre_metrics={
                genre: PerGenreMetric(
                    tp=metrics["tp"],
                    fp=metrics["fp"],
                    fn=metrics["fn"],
                    support=metrics.get("support", metrics["tp"] + metrics["fn"]),
                    precision=metrics["precision"],
                    precision_ci=(
                        ConfidenceInterval(**metrics["precision_ci"])
                        if "precision_ci" in metrics
                        else None
                    ),
                    recall=metrics["recall"],
                    recall_ci=(
                        ConfidenceInterval(**metrics["recall_ci"])
                        if "recall_ci" in metrics
                        else None
                    ),
                    f1=metrics["f1"],
                    warning=metrics.get("warning", False),
                )
                for genre, metrics in results["per_genre_metrics"].items()
            },
            confusion_matrix=results["confusion_matrix"],
            effect_size=results.get("effect_size"),
            statistical_power=(
                StatisticalPower(**results["statistical_power"])
                if "statistical_power" in results
                else None
            ),
            integrated_score=(
                IntegratedScore(**results["integrated_score"])
                if "integrated_score" in results
                else None
            ),
            total_samples=results["total_samples"],
            total_tp=results["total_tp"],
            total_fp=results["total_fp"],
            total_fn=results["total_fn"],
            cv_results=(
                {
                    "cv_accuracy": results["cv_accuracy"],
                    "cv_macro_f1": results["cv_macro_f1"],
                    "cv_micro_f1": results["cv_micro_f1"],
                    "n_folds": results["n_folds"],
                }
                if "cv_accuracy" in results
                else None
            ),
        )
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Evaluation failed: {str(e)}")

