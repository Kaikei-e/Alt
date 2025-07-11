export interface ErrorRecoveryConfig {
  initialBackoff: number;
  maxBackoff: number;
  recoveryThreshold: number;
  errorThreshold: number;
}

export class ErrorRecoveryManager {
  private errorHistory: string[] = [];
  private successHistory: string[] = [];
  private config: ErrorRecoveryConfig;
  private lastErrorTime = 0;
  private currentBackoff = 0;
  private timeProvider: () => number;

  constructor(config: Partial<ErrorRecoveryConfig> = {}, timeProvider: () => number = () => Date.now()) {
    this.config = {
      initialBackoff: 5000, // 5秒
      maxBackoff: 300000, // 5分
      recoveryThreshold: 10, // 10回連続成功で復旧
      errorThreshold: 3, // 3回連続エラーで無効化
      ...config
    };
    this.currentBackoff = this.config.initialBackoff;
    this.timeProvider = timeProvider;
  }

  recordError(errorType: string): void {
    this.errorHistory.push(errorType);
    this.successHistory = []; // 成功履歴をリセット
    this.lastErrorTime = this.timeProvider();

    // 履歴サイズ制限
    if (this.errorHistory.length > 10) {
      this.errorHistory.shift();
    }

    // バックオフ時間を増加（エラー毎に）
    this.currentBackoff = Math.min(
      this.currentBackoff * 2,
      this.config.maxBackoff
    );
  }

  recordSuccess(): void {
    this.successHistory.push('success');
    this.errorHistory = []; // エラー履歴をリセット

    // 履歴サイズ制限
    if (this.successHistory.length > 15) {
      this.successHistory.shift();
    }

    // バックオフ時間をリセット
    if (this.successHistory.length >= this.config.recoveryThreshold) {
      this.currentBackoff = this.config.initialBackoff;
    }
  }

  shouldDisableVirtualization(): boolean {
    // 3回連続エラーで無効化
    if (this.errorHistory.length >= this.config.errorThreshold) {
      return true;
    }

    // バックオフ期間中は無効化（エラーが発生している場合のみ）
    if (this.lastErrorTime > 0 && this.timeProvider() - this.lastErrorTime < this.currentBackoff) {
      return true;
    }

    return false;
  }

  canRetryNow(): boolean {
    if (this.lastErrorTime === 0) return true;
    return this.timeProvider() - this.lastErrorTime >= this.currentBackoff;
  }

  getBackoffTime(): number {
    return this.currentBackoff;
  }

  getErrorCount(): number {
    return this.errorHistory.length;
  }

  getSuccessCount(): number {
    return this.successHistory.length;
  }

  getRecoveryStatus(): {
    errorCount: number;
    successCount: number;
    backoffTime: number;
    canRetry: boolean;
  } {
    return {
      errorCount: this.getErrorCount(),
      successCount: this.getSuccessCount(),
      backoffTime: this.getBackoffTime(),
      canRetry: this.canRetryNow()
    };
  }

  reset(): void {
    this.errorHistory = [];
    this.successHistory = [];
    this.currentBackoff = this.config.initialBackoff;
    this.lastErrorTime = 0;
  }
}