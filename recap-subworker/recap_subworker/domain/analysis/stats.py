"""統計分析ユーティリティ（信頼区間、McNemar's Testなど）。"""

from typing import Callable, Dict, List, Optional, Tuple

import numpy as np
from scipy import stats
from statsmodels.stats.contingency_tables import mcnemar


def calculate_confidence_interval(
    n_success: int, n_total: int, confidence: float = 0.95
) -> Tuple[float, float, float]:
    """Wilson score intervalを使用して信頼区間を計算。

    Args:
        n_success: 成功数
        n_total: 総数
        confidence: 信頼度（デフォルト: 0.95）

    Returns:
        (点推定値, 下限, 上限) のタプル
    """
    if n_total == 0:
        return (0.0, 0.0, 0.0)

    z = stats.norm.ppf((1 + confidence) / 2.0)
    p = n_success / n_total
    n = n_total

    # Wilson score interval
    denominator = 1 + (z**2 / n)
    center = (p + (z**2 / (2 * n))) / denominator
    margin = (z / denominator) * np.sqrt((p * (1 - p) / n) + (z**2 / (4 * n**2)))

    lower = max(0.0, center - margin)
    upper = min(1.0, center + margin)

    return (p, lower, upper)


def compare_models_mcnemar(
    results_model_a: List[Tuple[bool, bool]], results_model_b: List[Tuple[bool, bool]]
) -> Tuple[float, float, np.ndarray]:
    """McNemar's Testを使用して2つのモデルを比較。

    Args:
        results_model_a: モデルAの結果リスト [(expected, predicted), ...]
        results_model_b: モデルBの結果リスト [(expected, predicted), ...]

    Returns:
        (統計量, p値, 混同行列) のタプル
    """
    if len(results_model_a) != len(results_model_b):
        raise ValueError("Results lists must have the same length")

    # 混同行列を構築
    # a: モデルAが正しく、モデルBが間違っている
    # b: モデルAが間違っていて、モデルBが正しい
    # c: 両方とも正しい
    # d: 両方とも間違っている
    a = 0  # A正、B誤
    b = 0  # A誤、B正
    c = 0  # 両方正
    d = 0  # 両方誤

    for (exp_a, pred_a), (exp_b, pred_b) in zip(results_model_a, results_model_b):
        correct_a = exp_a == pred_a
        correct_b = exp_b == pred_b

        if correct_a and not correct_b:
            a += 1
        elif not correct_a and correct_b:
            b += 1
        elif correct_a and correct_b:
            c += 1
        else:
            d += 1

    # 混同行列
    contingency_table = np.array([[c, b], [a, d]])

    # McNemar's test
    result = mcnemar(contingency_table, exact=False, correction=True)

    return (result.statistic, result.pvalue, contingency_table)


def bootstrap_confidence_interval(
    data: np.ndarray,
    statistic_func: Callable[[np.ndarray], float],
    confidence: float = 0.95,
    n_bootstrap: int = 1000,
    method: str = "percentile",
) -> Tuple[float, float, float]:
    """Bootstrap法による信頼区間を計算。

    Args:
        data: データ配列
        statistic_func: 統計量を計算する関数
        confidence: 信頼度（デフォルト: 0.95）
        n_bootstrap: ブートストラップリサンプリング回数（デフォルト: 1000）
        method: 信頼区間の計算方法（"percentile", "bca"）

    Returns:
        (点推定値, 下限, 上限) のタプル
    """
    if len(data) == 0:
        return (0.0, 0.0, 0.0)

    # 元の統計量を計算
    original_stat = statistic_func(data)

    # Bootstrapリサンプリング
    bootstrap_stats = []
    n = len(data)
    rng = np.random.default_rng()

    for _ in range(n_bootstrap):
        # リサンプリング（置換あり）
        bootstrap_sample = rng.choice(data, size=n, replace=True)
        bootstrap_stat = statistic_func(bootstrap_sample)
        bootstrap_stats.append(bootstrap_stat)

    bootstrap_stats = np.array(bootstrap_stats)

    # 信頼区間を計算
    alpha = 1 - confidence
    lower_percentile = (alpha / 2) * 100
    upper_percentile = (1 - alpha / 2) * 100

    if method == "percentile":
        lower = np.percentile(bootstrap_stats, lower_percentile)
        upper = np.percentile(bootstrap_stats, upper_percentile)
    elif method == "bca":
        # Bias-corrected and accelerated (BCa) bootstrap
        lower, upper = _bca_bootstrap_interval(
            data, bootstrap_stats, original_stat, confidence
        )
    else:
        raise ValueError(f"Unknown method: {method}")

    return (original_stat, lower, upper)


def _bca_bootstrap_interval(
    data: np.ndarray,
    bootstrap_stats: np.ndarray,
    original_stat: float,
    confidence: float,
) -> Tuple[float, float]:
    """BCa (Bias-corrected and accelerated) bootstrap信頼区間を計算。

    Args:
        data: 元のデータ
        bootstrap_stats: Bootstrap統計量の配列
        original_stat: 元の統計量
        confidence: 信頼度

    Returns:
        (下限, 上限) のタプル
    """
    n = len(data)
    alpha = 1 - confidence

    # Bias correction (z0)
    bias = np.mean(bootstrap_stats < original_stat)
    # biasが0または1の場合、z0がinf/-infになる可能性があるため、クリップ
    bias = np.clip(bias, 1e-10, 1 - 1e-10)
    z0 = stats.norm.ppf(bias)
    # NaNやInfの場合は0にフォールバック
    if not np.isfinite(z0):
        z0 = 0.0

    # Acceleration (a) - jackknife estimate
    jackknife_stats = []
    for i in range(n):
        jackknife_sample = np.delete(data, i)
        if len(jackknife_sample) > 0:
            # 簡易的な統計量計算（平均など）
            jackknife_stats.append(np.mean(jackknife_sample))
    if len(jackknife_stats) > 0:
        jackknife_mean = np.mean(jackknife_stats)
        numerator = np.sum((jackknife_mean - np.array(jackknife_stats)) ** 3)
        denominator = 6 * (np.sum((jackknife_mean - np.array(jackknife_stats)) ** 2)) ** 1.5
        a = numerator / denominator if denominator > 0 else 0.0
    else:
        a = 0.0

    # BCa調整されたパーセンタイル
    z_alpha_2 = stats.norm.ppf(alpha / 2)
    z_1_minus_alpha_2 = stats.norm.ppf(1 - alpha / 2)

    def bca_percentile(z):
        num = z0 + z
        denom = 1 - a * (z0 + z)
        if denom == 0 or not np.isfinite(denom):
            return 0.5
        result = stats.norm.cdf(z0 + num / denom)
        # NaNやInfの場合は0.5にフォールバック
        if not np.isfinite(result):
            return 0.5
        return result

    lower_p = bca_percentile(z_alpha_2)
    upper_p = bca_percentile(z_1_minus_alpha_2)

    # NaNやInfの場合はデフォルト値にフォールバック
    if not np.isfinite(lower_p):
        lower_p = alpha / 2
    if not np.isfinite(upper_p):
        upper_p = 1 - alpha / 2

    # パーセンタイルを0-100の範囲にクリップ
    lower_p_clipped = np.clip(lower_p * 100, 0, 100)
    upper_p_clipped = np.clip(upper_p * 100, 0, 100)

    lower = np.percentile(bootstrap_stats, lower_p_clipped)
    upper = np.percentile(bootstrap_stats, upper_p_clipped)

    return (lower, upper)


def clopper_pearson_interval(
    n_success: int, n_total: int, confidence: float = 0.95
) -> Tuple[float, float, float]:
    """Clopper-Pearson信頼区間を計算（二項分布の正確な信頼区間）。

    小サンプルサイズや不均衡データに適している。

    Args:
        n_success: 成功数
        n_total: 総数
        confidence: 信頼度（デフォルト: 0.95）

    Returns:
        (点推定値, 下限, 上限) のタプル
    """
    if n_total == 0:
        return (0.0, 0.0, 0.0)

    p = n_success / n_total
    alpha = 1 - confidence

    # Clopper-Pearson interval
    if n_success == 0:
        lower = 0.0
    else:
        lower = stats.beta.ppf(alpha / 2, n_success, n_total - n_success + 1)

    if n_success == n_total:
        upper = 1.0
    else:
        upper = stats.beta.ppf(1 - alpha / 2, n_success + 1, n_total - n_success)

    return (p, lower, upper)


def calculate_statistical_power(
    effect_size: float,
    sample_size: int,
    alpha: float = 0.05,
    test_type: str = "two_sided",
) -> float:
    """統計的検出力を計算。

    Args:
        effect_size: 効果サイズ（Cohen's dなど）
        sample_size: サンプルサイズ
        alpha: 有意水準（デフォルト: 0.05）
        test_type: 検定の種類（"two_sided", "one_sided"）

    Returns:
        検出力（0-1の範囲）
    """
    from statsmodels.stats.power import TTestPower

    power_analysis = TTestPower()
    if test_type == "two_sided":
        power = power_analysis.power(effect_size, sample_size, alpha, alternative="two-sided")
    else:
        power = power_analysis.power(effect_size, sample_size, alpha, alternative="larger")

    return power


def calculate_required_sample_size(
    effect_size: float,
    power: float = 0.8,
    alpha: float = 0.05,
    test_type: str = "two_sided",
) -> int:
    """目標とする検出力を達成するために必要なサンプルサイズを計算。

    Args:
        effect_size: 効果サイズ（Cohen's dなど）
        power: 目標とする検出力（デフォルト: 0.8）
        alpha: 有意水準（デフォルト: 0.05）
        test_type: 検定の種類（"two_sided", "one_sided"）

    Returns:
        必要なサンプルサイズ
    """
    from statsmodels.stats.power import TTestPower

    power_analysis = TTestPower()
    if test_type == "two_sided":
        n = power_analysis.solve_power(
            effect_size, power=power, alpha=alpha, alternative="two-sided"
        )
    else:
        n = power_analysis.solve_power(
            effect_size, power=power, alpha=alpha, alternative="larger"
        )

    return int(np.ceil(n))


def cohens_d(group1: np.ndarray, group2: np.ndarray) -> float:
    """Cohen's d（効果サイズ）を計算。

    Args:
        group1: 第1群のデータ
        group2: 第2群のデータ

    Returns:
        Cohen's d
    """
    n1, n2 = len(group1), len(group2)
    var1, var2 = np.var(group1, ddof=1), np.var(group2, ddof=1)

    # Pooled standard deviation
    pooled_std = np.sqrt(((n1 - 1) * var1 + (n2 - 1) * var2) / (n1 + n2 - 2))

    if pooled_std == 0:
        return 0.0

    d = (np.mean(group1) - np.mean(group2)) / pooled_std
    return d


def cramers_v(confusion_matrix: np.ndarray) -> float:
    """Cramér's V（効果サイズ）を計算。

    Args:
        confusion_matrix: 混同行列

    Returns:
        Cramér's V（0-1の範囲）
    """
    from scipy.stats import chi2_contingency

    n = np.sum(confusion_matrix)
    if n == 0:
        return 0.0

    # ゼロの行や列を削除
    # 行の合計がゼロでない行のみを保持
    row_sums = np.sum(confusion_matrix, axis=1)
    col_sums = np.sum(confusion_matrix, axis=0)
    non_zero_rows = row_sums > 0
    non_zero_cols = col_sums > 0

    if not np.any(non_zero_rows) or not np.any(non_zero_cols):
        return 0.0

    # ゼロでない行と列のみを含む混同行列を作成
    filtered_cm = confusion_matrix[non_zero_rows][:, non_zero_cols]

    # フィルタリング後のサイズをチェック
    if filtered_cm.size == 0:
        return 0.0

    min_dim = min(filtered_cm.shape) - 1
    if min_dim <= 0:
        return 0.0

    try:
        chi2, _, _, _ = chi2_contingency(filtered_cm)
        # chi2がNaNやInfの場合は0.0を返す
        if not np.isfinite(chi2) or chi2 < 0:
            return 0.0
    except (ValueError, ZeroDivisionError) as e:
        # 期待度数がゼロなどの場合、エラーをキャッチして0.0を返す
        return 0.0

    v = np.sqrt(chi2 / (n * min_dim))
    return min(v, 1.0)


def wilcoxon_signed_rank_test(
    group1: np.ndarray, group2: np.ndarray
) -> Tuple[float, float]:
    """Wilcoxon signed-rank testを実行（対応のある2群の比較）。

    Args:
        group1: 第1群のデータ
        group2: 第2群のデータ

    Returns:
        (統計量, p値) のタプル
    """
    if len(group1) != len(group2):
        raise ValueError("Groups must have the same length for paired test")

    statistic, pvalue = stats.wilcoxon(group1, group2)
    return (statistic, pvalue)


def mann_whitney_u_test(
    group1: np.ndarray, group2: np.ndarray, alternative: str = "two-sided"
) -> Tuple[float, float]:
    """Mann-Whitney U testを実行（独立した2群の比較）。

    Args:
        group1: 第1群のデータ
        group2: 第2群のデータ
        alternative: 対立仮説（"two-sided", "less", "greater"）

    Returns:
        (統計量, p値) のタプル
    """
    statistic, pvalue = stats.mannwhitneyu(group1, group2, alternative=alternative)
    return (statistic, pvalue)


def bonferroni_correction(pvalues: List[float], alpha: float = 0.05) -> List[float]:
    """Bonferroni補正を適用。

    Args:
        pvalues: p値のリスト
        alpha: 有意水準（デフォルト: 0.05）

    Returns:
        補正後のp値のリスト
    """
    n = len(pvalues)
    if n == 0:
        return []

    corrected_pvalues = [min(p * n, 1.0) for p in pvalues]
    return corrected_pvalues

