export interface SimplePerformanceMetrics {
  renderTime: number;
  itemCount: number;
  memoryUsage: number;
  timestamp: number;
}

export class SimplePerformanceTracker {
  private metrics: SimplePerformanceMetrics[] = [];
  private readonly maxHistory: number;

  constructor(maxHistory: number = 20) {
    this.maxHistory = maxHistory;
  }

  recordMetrics(metrics: Omit<SimplePerformanceMetrics, "timestamp">): void {
    const fullMetrics: SimplePerformanceMetrics = {
      ...metrics,
      timestamp: Date.now(),
    };

    this.metrics.push(fullMetrics);

    if (this.metrics.length > this.maxHistory) {
      this.metrics.shift();
    }

    // 開発環境でのみログ出力
    if (process.env.NODE_ENV === "development") {
      console.log("Performance metrics:", fullMetrics);
    }
  }

  getAverageRenderTime(): number {
    if (this.metrics.length === 0) return 0;

    const total = this.metrics.reduce((sum, m) => sum + m.renderTime, 0);
    return total / this.metrics.length;
  }

  detectPerformanceRegression(): boolean {
    if (this.metrics.length < 10) return false;

    const recent = this.metrics.slice(-5);
    const older = this.metrics.slice(0, 5);

    const recentAvg =
      recent.reduce((sum, m) => sum + m.renderTime, 0) / recent.length;
    const olderAvg =
      older.reduce((sum, m) => sum + m.renderTime, 0) / older.length;

    // 50%以上の性能悪化
    return recentAvg > olderAvg * 1.5;
  }

  clearHistory(): void {
    this.metrics = [];
  }
}
