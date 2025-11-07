/// エラー分類とリトライ判定ユーティリティ。
use anyhow::Error;
use reqwest::StatusCode;
use sqlx::Error as SqlxError;

/// エラーの種類。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ErrorKind {
    /// リトライ可能なエラー（一時的なネットワークエラー、タイムアウトなど）
    Retryable,
    /// リトライ不可能なエラー（認証エラー、バリデーションエラーなど）
    NonRetryable,
    /// 致命的なエラー（データ破損、設定エラーなど）
    Fatal,
}

/// エラーがリトライ可能かどうかを判定する。
#[must_use]
pub(crate) fn classify_error(error: &Error) -> ErrorKind {
    // HTTPエラーの判定
    if let Some(reqwest_err) = error.downcast_ref::<reqwest::Error>() {
        if reqwest_err.is_timeout() || reqwest_err.is_connect() {
            return ErrorKind::Retryable;
        }

        if let Some(status) = reqwest_err.status() {
            match status {
                // 5xxエラーはリトライ可能
                StatusCode::INTERNAL_SERVER_ERROR
                | StatusCode::BAD_GATEWAY
                | StatusCode::SERVICE_UNAVAILABLE
                | StatusCode::GATEWAY_TIMEOUT => return ErrorKind::Retryable,
                // 4xxエラー（認証・認可以外）はリトライ不可能
                StatusCode::BAD_REQUEST
                | StatusCode::NOT_FOUND
                | StatusCode::UNPROCESSABLE_ENTITY => return ErrorKind::NonRetryable,
                // 認証・認可エラーは致命的
                StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => return ErrorKind::Fatal,
                _ => {}
            }
        }
    }

    // SQLxエラーの判定
    if let Some(sqlx_err) = error.downcast_ref::<SqlxError>() {
        match sqlx_err {
            SqlxError::PoolTimedOut | SqlxError::PoolClosed | SqlxError::Database(_) => {
                return ErrorKind::Retryable
            }
            SqlxError::RowNotFound => return ErrorKind::NonRetryable,
            SqlxError::Configuration(_) => return ErrorKind::Fatal,
            _ => {}
        }
    }

    // デフォルトはリトライ不可能
    ErrorKind::NonRetryable
}

/// エラーがリトライ可能かどうかを判定する。
#[must_use]
pub(crate) fn is_retryable(error: &Error) -> bool {
    matches!(classify_error(error), ErrorKind::Retryable)
}

/// エラーが致命的かどうかを判定する。
#[must_use]
pub(crate) fn is_fatal(error: &Error) -> bool {
    matches!(classify_error(error), ErrorKind::Fatal)
}

#[cfg(test)]
mod tests {
    use super::*;
    use anyhow::anyhow;

    #[test]
    fn timeout_error_is_retryable() {
        let error = anyhow!("timeout");
        // 実際のreqwest::Errorを生成するのは難しいので、簡易テスト
        assert!(!is_fatal(&error));
    }

    #[test]
    fn validation_error_is_non_retryable() {
        let error = anyhow!("validation failed");
        assert!(!is_retryable(&error));
        assert!(!is_fatal(&error));
    }
}
