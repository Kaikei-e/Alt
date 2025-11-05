use anyhow::{Error, Result};
use once_cell::sync::OnceCell;
use tracing_subscriber::{EnvFilter, fmt};

static TRACING_INIT: OnceCell<()> = OnceCell::new();

/// Tracing サブスクライバを一度だけ初期化する。
///
/// # Errors
/// サブスクライバの初期化に失敗した場合はエラーを返す。
pub fn init() -> Result<()> {
    TRACING_INIT.get_or_try_init(|| {
        let env_filter =
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));

        fmt()
            .with_env_filter(env_filter)
            .with_target(false)
            .try_init()
            .map_err(|error| Error::msg(error.to_string()))?;

        Ok::<(), Error>(())
    })?;
    Ok(())
}
