use std::env;
use std::path::PathBuf;
use std::process;

use anyhow::{Context, Result, anyhow, bail};
use recap_worker::replay::{ReplayConfig, replay_genre_pipeline};

#[tokio::main]
async fn main() -> Result<()> {
    let config = parse_args()?;
    replay_genre_pipeline(config).await
}

fn parse_args() -> Result<ReplayConfig> {
    let mut dataset = None;
    let mut dsn = None;
    let mut graph_window = None;
    let mut graph_ttl_seconds = None;
    let mut require_tags = false;
    let mut dry_run = false;

    let mut args = env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--dataset" => {
                let value = args.next().context("--dataset requires a path argument")?;
                dataset = Some(PathBuf::from(value));
            }
            "--dsn" => {
                let value = args.next().context("--dsn requires a connection string")?;
                dsn = Some(value);
            }
            "--graph-window" => {
                let value = args
                    .next()
                    .context("--graph-window requires a label (e.g. 7d)")?;
                graph_window = Some(value);
            }
            "--graph-ttl" => {
                let value = args.next().context("--graph-ttl requires seconds")?;
                let parsed = value
                    .parse::<u64>()
                    .context("--graph-ttl must be an integer")?;
                graph_ttl_seconds = Some(parsed);
            }
            "--require-tags" => {
                require_tags = true;
            }
            "--dry-run" => {
                dry_run = true;
            }
            "--help" => {
                print_usage();
                process::exit(0);
            }
            _ => {
                bail!("unknown argument: {}", arg);
            }
        }
    }

    let dataset = dataset.ok_or_else(|| anyhow!("--dataset is required"))?;
    let dsn = dsn
        .or_else(|| env::var("RECAP_DB_DSN").ok())
        .ok_or_else(|| anyhow!("RECAP_DB_DSN is required via --dsn or environment"))?;
    let graph_window = graph_window.unwrap_or_else(|| "7d".to_string());
    let graph_ttl_seconds = graph_ttl_seconds.unwrap_or(900);

    Ok(ReplayConfig {
        dataset,
        dsn,
        graph_window,
        graph_ttl_seconds,
        require_tags,
        dry_run,
    })
}

fn print_usage() {
    eprintln!(
        "Usage: replay_genre_pipeline --dataset <path> [--dsn <dsn>] [--graph-window 7d] [--graph-ttl 900] [--require-tags] [--dry-run]"
    );
}
