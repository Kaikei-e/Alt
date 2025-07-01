use std::env;

#[derive(Debug)]
pub struct Settings {
    pub clickhouse_host: String,
    pub clickhouse_port: u16,
    pub clickhouse_user: String,
    pub clickhouse_password: String,
    pub clickhouse_database: String,
}

pub fn get_configuration() -> Result<Settings, Box<dyn std::error::Error>> {
    let clickhouse_host = env::var("APP_CLICKHOUSE_HOST")?;
    let clickhouse_port = env::var("APP_CLICKHOUSE_PORT")?.parse::<u16>()?;
    let clickhouse_user = env::var("APP_CLICKHOUSE_USER")?;
    let clickhouse_password = env::var("APP_CLICKHOUSE_PASSWORD")?;
    let clickhouse_database = env::var("APP_CLICKHOUSE_DATABASE")?;

    Ok(Settings {
        clickhouse_host,
        clickhouse_port,
        clickhouse_user,
        clickhouse_password,
        clickhouse_database,
    })
}