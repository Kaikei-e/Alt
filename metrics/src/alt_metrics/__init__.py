"""Alt システム健全性アナライザー

ClickHouseに蓄積されたログ・トレースデータを分析し、
システムの健全性レポートを日本語Markdownで生成します。

Usage:
    uv run python -m alt_metrics analyze --hours 24
    uv run python -m alt_metrics analyze --lang ja --verbose
"""

from alt_metrics.models import AnalysisResult, ServiceHealth

__all__ = ["AnalysisResult", "ServiceHealth"]
__version__ = "0.2.0"
