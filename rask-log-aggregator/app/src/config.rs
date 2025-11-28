use std::env;
use std::fs;

#[derive(Debug)]
pub struct Settings {
    pub clickhouse_host: String,
    pub clickhouse_port: u16,
    pub clickhouse_user: String,
    pub clickhouse_password: String,
    pub clickhouse_database: String,
}

/// Read a value from environment variable, with support for _FILE suffix (Docker Secrets)
fn get_env_or_file(env_name: &str) -> Result<String, Box<dyn std::error::Error>> {
    // First check for _FILE suffix (Docker Secrets support)
    let file_env = format!("{}_FILE", env_name);
    if let Ok(file_path) = env::var(&file_env) {
        match fs::read_to_string(&file_path) {
            Ok(content) => return Ok(content.trim().to_string()),
            Err(e) => return Err(format!("Failed to read {}: {}", file_env, e).into()),
        }
    }

    // Fallback to standard environment variable
    env::var(env_name).map_err(|_| {
        format!(
            "Missing required environment variable: {} or {}",
            env_name, file_env
        )
        .into()
    })
}

pub fn get_configuration() -> Result<Settings, Box<dyn std::error::Error>> {
    let clickhouse_host = env::var("APP_CLICKHOUSE_HOST")?;
    let clickhouse_port = env::var("APP_CLICKHOUSE_PORT")?.parse::<u16>()?;
    let clickhouse_user = env::var("APP_CLICKHOUSE_USER")?;
    let clickhouse_password = get_env_or_file("APP_CLICKHOUSE_PASSWORD")?;
    let clickhouse_database = env::var("APP_CLICKHOUSE_DATABASE")?;

    Ok(Settings {
        clickhouse_host,
        clickhouse_port,
        clickhouse_user,
        clickhouse_password,
        clickhouse_database,
    })
}
