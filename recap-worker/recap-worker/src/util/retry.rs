/// 指数バックオフ+ジッター付き再試行ロジック。
///
/// AWS推奨のFull Jitter戦略を実装します。
use std::time::Duration;

use rand::Rng;

/// 再試行戦略の設定。
#[derive(Debug, Clone, Copy)]
pub(crate) struct RetryConfig {
    /// 最大試行回数（初回を含む）
    pub(crate) max_attempts: usize,
    /// ベースとなる遅延時間（ミリ秒）
    pub(crate) base_delay_ms: u64,
    /// 最大遅延時間（ミリ秒）
    pub(crate) max_delay_ms: u64,
}

impl Default for RetryConfig {
    fn default() -> Self {
        Self {
            max_attempts: 3,
            base_delay_ms: 250,
            max_delay_ms: 10000,
        }
    }
}

impl RetryConfig {
    /// 新しい再試行設定を作成する。
    #[must_use]
    pub(crate) const fn new(max_attempts: usize, base_delay_ms: u64, max_delay_ms: u64) -> Self {
        Self {
            max_attempts,
            base_delay_ms,
            max_delay_ms,
        }
    }

    /// 指定された試行回数に対する遅延時間を計算する（Full Jitter戦略）。
    ///
    /// # Arguments
    /// * `attempt` - 試行回数（0から開始）
    ///
    /// # Returns
    /// 待機すべき期間
    #[must_use]
    pub(crate) fn delay_for_attempt(&self, attempt: usize) -> Duration {
        if attempt == 0 {
            return Duration::from_millis(0);
        }

        // 指数バックオフ: base * 2^(attempt-1)
        let exponential_delay = self
            .base_delay_ms
            .saturating_mul(1_u64.saturating_shl((attempt - 1) as u32));

        // 上限でキャップ
        let capped_delay = exponential_delay.min(self.max_delay_ms);

        // Full Jitter: random(0, capped_delay)
        let jittered_delay = if capped_delay > 0 {
            let mut rng = rand::thread_rng();
            rng.gen_range(0..=capped_delay)
        } else {
            0
        };

        Duration::from_millis(jittered_delay)
    }

    /// この試行回数が再試行可能かどうかを判定する。
    #[must_use]
    pub(crate) const fn can_retry(&self, attempt: usize) -> bool {
        attempt < self.max_attempts
    }
}

/// エラーが再試行可能かどうかを判定する。
///
/// 以下の場合に再試行可能と判断します：
/// - ネットワークエラー
/// - タイムアウト
/// - 5xxサーバーエラー
/// - 429 Too Many Requests
pub(crate) fn is_retryable_error(error: &reqwest::Error) -> bool {
    // ネットワークエラーやタイムアウトは再試行可能
    if error.is_timeout() || error.is_connect() {
        return true;
    }

    // ステータスコードをチェック
    if let Some(status) = error.status() {
        // 5xxエラーまたは429は再試行可能
        if status.is_server_error() || status == reqwest::StatusCode::TOO_MANY_REQUESTS {
            return true;
        }
    }

    false
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn delay_for_attempt_zero_is_zero() {
        let config = RetryConfig::default();
        let delay = config.delay_for_attempt(0);
        assert_eq!(delay, Duration::from_millis(0));
    }

    #[test]
    fn delay_for_attempt_increases_exponentially() {
        let config = RetryConfig::new(5, 100, 10000);

        // 最初の試行は遅延なし
        assert_eq!(config.delay_for_attempt(0), Duration::from_millis(0));

        // 1回目の再試行: 0..=100ms
        let delay1 = config.delay_for_attempt(1);
        assert!(delay1 <= Duration::from_millis(100));

        // 2回目の再試行: 0..=200ms
        let delay2 = config.delay_for_attempt(2);
        assert!(delay2 <= Duration::from_millis(200));

        // 3回目の再試行: 0..=400ms
        let delay3 = config.delay_for_attempt(3);
        assert!(delay3 <= Duration::from_millis(400));
    }

    #[test]
    fn delay_for_attempt_respects_max_delay() {
        let config = RetryConfig::new(10, 100, 500);

        // 10回目の試行でも上限を超えない
        let delay = config.delay_for_attempt(10);
        assert!(delay <= Duration::from_millis(500));
    }

    #[test]
    fn can_retry_respects_max_attempts() {
        let config = RetryConfig::new(3, 100, 1000);

        assert!(config.can_retry(0));
        assert!(config.can_retry(1));
        assert!(config.can_retry(2));
        assert!(!config.can_retry(3));
        assert!(!config.can_retry(4));
    }

    #[test]
    fn full_jitter_provides_variation() {
        let config = RetryConfig::new(5, 100, 10000);

        // 同じ試行回数で複数回呼び出すと異なる値が返されることを確認
        let delays: Vec<Duration> = (0..10).map(|_| config.delay_for_attempt(3)).collect();

        // すべてが同じ値でないことを確認（ジッターが機能している）
        let all_same = delays.windows(2).all(|w| w[0] == w[1]);
        assert!(!all_same, "jitter should produce varying delays");
    }
}
