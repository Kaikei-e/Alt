//! Thin CLI helpers extracted from `main` (healthcheck / warmup / panic hook).

use std::env;
use std::time::Duration;

use tracing::error;

/// Returns `Some(exit_code)` when `args` requests the healthcheck subcommand.
pub(crate) fn try_healthcheck(args: &[String]) -> Option<i32> {
    if args.get(1).map(String::as_str) == Some("healthcheck") {
        Some(run_healthcheck())
    } else {
        None
    }
}

/// Returns `Some(exit_code)` when `args` requests the warmup subcommand.
pub(crate) async fn try_warmup(args: &[String]) -> Option<i32> {
    if args.get(1).map(String::as_str) != Some("warmup") {
        return None;
    }
    // warmup subcommand: populate the rust-bert AllMiniLmL12V2 model cache
    // so the runtime container can boot in a network-isolated stack.
    Some(match recap_worker::warmup_embedding_cache().await {
        Ok(()) => {
            eprintln!("warmup: rust-bert AllMiniLmL12V2 cache populated");
            0
        }
        Err(e) => {
            eprintln!("warmup failed: {e:?}");
            1
        }
    })
}

/// Perform a health check against the local HTTP server.
/// Returns exit code 0 on success, 1 on failure.
fn run_healthcheck() -> i32 {
    let port = env::var("PORT").unwrap_or_else(|_| "9005".to_string());
    let url = format!("http://127.0.0.1:{port}/health/live");

    let client = reqwest::blocking::Client::builder()
        .timeout(Duration::from_secs(5))
        .build();

    let client = match client {
        Ok(c) => c,
        Err(e) => {
            eprintln!("healthcheck failed: failed to create client: {e}");
            return 1;
        }
    };

    match client.get(&url).send() {
        Ok(resp) if resp.status().is_success() => 0,
        Ok(resp) => {
            eprintln!("healthcheck failed: status {}", resp.status());
            1
        }
        Err(e) => {
            eprintln!("healthcheck failed: {e}");
            1
        }
    }
}

async fn run_eval_report(args: &[String]) -> i32 {
    let cli_args = match parse_eval_report_args(args) {
        Ok(a) => a,
        Err(err) => {
            eprintln!("eval report error: {err}");
            eprintln!(
                "usage: recap-worker eval report --window <uuid> [--k 10] [--format json|markdown]"
            );
            return 1;
        }
    };

    let config = match recap_worker::config::Config::from_env() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("failed to load config from environment: {e}");
            return 1;
        }
    };

    let pool = match sqlx::postgres::PgPoolOptions::new()
        .max_connections(5)
        .connect(config.recap_db_dsn())
        .await
    {
        Ok(p) => p,
        Err(e) => {
            eprintln!("failed to connect to database: {e} (check RECAP_DB_DSN / DATABASE_URL)");
            return 1;
        }
    };

    let report =
        match recap_worker::eval::generate_window_report(&pool, cli_args.window_id, cli_args.k)
            .await
        {
            Ok(r) => r,
            Err(e) => {
                eprintln!("eval report failed: {e}");
                return 1;
            }
        };

    if cli_args.format == "json" {
        match report.to_json() {
            Ok(json) => println!("{json}"),
            Err(e) => {
                eprintln!("failed to format json: {e}");
                return 1;
            }
        }
    } else {
        println!("{}", report.to_markdown());
    }

    0
}

async fn run_eval_replay_cli(args: &[String]) -> i32 {
    let cli_args = match parse_eval_replay_args(args) {
        Ok(a) => a,
        Err(err) => {
            eprintln!("eval replay error: {err}");
            eprintln!(
                "usage: recap-worker eval replay --from <RFC3339> --to <RFC3339> [--param key=value ...] [--params-version <label>] [--generate]"
            );
            return 1;
        }
    };

    let config = match recap_worker::config::Config::from_env() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("failed to load config from environment: {e}");
            return 1;
        }
    };

    let pool = match sqlx::postgres::PgPoolOptions::new()
        .max_connections(5)
        .connect(config.recap_db_dsn())
        .await
    {
        Ok(p) => p,
        Err(e) => {
            eprintln!("failed to connect to database: {e} (check RECAP_DB_DSN / DATABASE_URL)");
            return 1;
        }
    };

    let result = match recap_worker::eval::run_eval_replay(
        &pool,
        &config,
        cli_args.from,
        cli_args.to,
        &cli_args.params,
        cli_args.generate,
    )
    .await
    {
        Ok(r) => r,
        Err(e) => {
            eprintln!("eval replay failed: {e:?}");
            return 1;
        }
    };

    let mut val = match serde_json::to_value(&result) {
        Ok(v) => v,
        Err(e) => {
            eprintln!("failed to serialize replay result: {e}");
            return 1;
        }
    };
    if let serde_json::Value::Object(ref mut map) = val {
        map.insert(
            "cards_selected".to_string(),
            serde_json::json!(result.stats.cards_selected),
        );
        map.insert(
            "cards_dropped".to_string(),
            result.stats.cards_dropped.clone(),
        );
        map.insert("llm_ms".to_string(), serde_json::json!(result.stats.llm_ms));
    }

    match serde_json::to_string(&val) {
        Ok(json) => println!("{json}"),
        Err(e) => {
            eprintln!("failed to format json: {e}");
            return 1;
        }
    }

    0
}

/// Returns `Some(exit_code)` when `args` requests the eval subcommand.
pub(crate) async fn try_eval(args: &[String]) -> Option<i32> {
    if args.get(1).map(String::as_str) != Some("eval") {
        return None;
    }

    match args.get(2).map(String::as_str) {
        Some("report") => Some(run_eval_report(&args[3..]).await),
        Some("replay") => Some(run_eval_replay_cli(&args[3..]).await),
        _ => {
            eprintln!("unknown eval command");
            eprintln!(
                "usage: recap-worker eval report --window <uuid> [--k 10] [--format json|markdown]\n       recap-worker eval replay --from <RFC3339> --to <RFC3339> [--param key=value ...] [--params-version <label>] [--generate]"
            );
            Some(1)
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
pub struct EvalReportCliArgs {
    pub window_id: uuid::Uuid,
    pub k: usize,
    pub format: String,
}

pub fn parse_eval_report_args(args: &[String]) -> Result<EvalReportCliArgs, String> {
    let mut window_id = None;
    let mut k = 10;
    let mut format = "markdown".to_string();

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--window" => {
                i += 1;
                let val = args
                    .get(i)
                    .ok_or_else(|| "--window requires a value".to_string())?;
                window_id = Some(
                    uuid::Uuid::parse_str(val)
                        .map_err(|e| format!("invalid --window UUID: {e}"))?,
                );
            }
            s if s.starts_with("--window=") => {
                let val = &s["--window=".len()..];
                window_id = Some(
                    uuid::Uuid::parse_str(val)
                        .map_err(|e| format!("invalid --window UUID: {e}"))?,
                );
            }
            "--k" => {
                i += 1;
                let val = args
                    .get(i)
                    .ok_or_else(|| "--k requires a value".to_string())?;
                k = val
                    .parse::<usize>()
                    .map_err(|e| format!("invalid --k value: {e}"))?;
            }
            s if s.starts_with("--k=") => {
                let val = &s["--k=".len()..];
                k = val
                    .parse::<usize>()
                    .map_err(|e| format!("invalid --k value: {e}"))?;
            }
            "--format" => {
                i += 1;
                let val = args
                    .get(i)
                    .ok_or_else(|| "--format requires a value".to_string())?;
                if val != "json" && val != "markdown" {
                    return Err(format!(
                        "invalid --format: must be 'json' or 'markdown', got '{val}'"
                    ));
                }
                format.clone_from(val);
            }
            s if s.starts_with("--format=") => {
                let val = &s["--format=".len()..];
                if val != "json" && val != "markdown" {
                    return Err(format!(
                        "invalid --format: must be 'json' or 'markdown', got '{val}'"
                    ));
                }
                format = val.to_string();
            }
            other => {
                return Err(format!("unknown argument: '{other}'"));
            }
        }
        i += 1;
    }

    let window_id =
        window_id.ok_or_else(|| "missing required argument: --window <uuid>".to_string())?;

    Ok(EvalReportCliArgs {
        window_id,
        k,
        format,
    })
}

#[derive(Debug, Clone, PartialEq)]
pub struct EvalReplayCliArgs {
    pub from: chrono::DateTime<chrono::Utc>,
    pub to: chrono::DateTime<chrono::Utc>,
    pub params: recap_worker::pipeline::cards::CardsParams,
    pub generate: bool,
}

impl EvalReplayCliArgs {
    #[cfg(test)]
    pub fn params_version(&self) -> &str {
        &self.params.params_version
    }
}

#[allow(clippy::too_many_lines)]
pub fn parse_eval_replay_args(args: &[String]) -> Result<EvalReplayCliArgs, String> {
    let mut from = None;
    let mut to = None;
    let mut params_version = None;
    let mut param_overrides = Vec::new();
    let mut generate = false;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--from" => {
                i += 1;
                let val = args
                    .get(i)
                    .ok_or_else(|| "--from requires a value".to_string())?;
                from = Some(
                    chrono::DateTime::parse_from_rfc3339(val)
                        .map_err(|e| format!("invalid --from RFC3339 timestamp: {e}"))?
                        .with_timezone(&chrono::Utc),
                );
            }
            s if s.starts_with("--from=") => {
                let val = &s["--from=".len()..];
                from = Some(
                    chrono::DateTime::parse_from_rfc3339(val)
                        .map_err(|e| format!("invalid --from RFC3339 timestamp: {e}"))?
                        .with_timezone(&chrono::Utc),
                );
            }
            "--to" => {
                i += 1;
                let val = args
                    .get(i)
                    .ok_or_else(|| "--to requires a value".to_string())?;
                to = Some(
                    chrono::DateTime::parse_from_rfc3339(val)
                        .map_err(|e| format!("invalid --to RFC3339 timestamp: {e}"))?
                        .with_timezone(&chrono::Utc),
                );
            }
            s if s.starts_with("--to=") => {
                let val = &s["--to=".len()..];
                to = Some(
                    chrono::DateTime::parse_from_rfc3339(val)
                        .map_err(|e| format!("invalid --to RFC3339 timestamp: {e}"))?
                        .with_timezone(&chrono::Utc),
                );
            }
            "--params-version" => {
                i += 1;
                let val = args
                    .get(i)
                    .ok_or_else(|| "--params-version requires a value".to_string())?;
                params_version = Some(val.clone());
            }
            s if s.starts_with("--params-version=") => {
                let val = &s["--params-version=".len()..];
                params_version = Some(val.to_string());
            }
            "--param" => {
                i += 1;
                let val = args
                    .get(i)
                    .ok_or_else(|| "--param requires a value (key=value)".to_string())?;
                let (key, value) = val
                    .split_once('=')
                    .ok_or_else(|| format!("invalid --param '{val}': expected key=value"))?;
                if key.trim().is_empty() {
                    return Err(format!("invalid --param '{val}': key cannot be empty"));
                }
                param_overrides.push((key.trim().to_string(), value.to_string()));
            }
            s if s.starts_with("--param=") => {
                let val = &s["--param=".len()..];
                let (key, value) = val
                    .split_once('=')
                    .ok_or_else(|| format!("invalid --param '{val}': expected key=value"))?;
                if key.trim().is_empty() {
                    return Err(format!("invalid --param '{val}': key cannot be empty"));
                }
                param_overrides.push((key.trim().to_string(), value.to_string()));
            }
            "--generate" => {
                generate = true;
            }
            s if s.starts_with("--generate=") => {
                let val = &s["--generate=".len()..];
                generate = val
                    .parse::<bool>()
                    .map_err(|e| format!("invalid --generate boolean '{val}': {e}"))?;
            }
            other => {
                return Err(format!("unknown argument: '{other}'"));
            }
        }
        i += 1;
    }

    let from = from.ok_or_else(|| "missing required argument: --from <RFC3339>".to_string())?;
    let to = to.ok_or_else(|| "missing required argument: --to <RFC3339>".to_string())?;

    let mut params = recap_worker::pipeline::cards::CardsParams::default();
    if let Some(ver) = params_version {
        params.params_version = ver;
    }
    for (key, val) in param_overrides {
        let json_val =
            serde_json::from_str(&val).unwrap_or_else(|_| serde_json::Value::String(val.clone()));
        params = params
            .with_override(&key, &json_val)
            .map_err(|e| format!("invalid parameter override '{key}={val}': {e}"))?;
    }

    Ok(EvalReplayCliArgs {
        from,
        to,
        params,
        generate,
    })
}

/// Install a panic hook that routes panics through `tracing`.
///
/// Must be called after tracing has been initialized (e.g. after
/// `ComponentRegistry::build`), otherwise early panics are silently lost.
pub(crate) fn install_panic_hook() {
    std::panic::set_hook(Box::new(|panic_info| {
        let thread = std::thread::current();
        let thread_name = thread.name().unwrap_or("unnamed");
        let message = panic_info
            .payload()
            .downcast_ref::<&str>()
            .copied()
            .or_else(|| {
                panic_info
                    .payload()
                    .downcast_ref::<String>()
                    .map(String::as_str)
            })
            .unwrap_or("unknown panic payload");

        if let Some(location) = panic_info.location() {
            error!(
                thread = thread_name,
                file = location.file(),
                line = location.line(),
                column = location.column(),
                message,
                "panic occurred"
            );
        } else {
            error!(
                thread = thread_name,
                message, "panic occurred without location information"
            );
        }
    }));
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_eval_report_args_success() {
        let args = vec![
            "--window".to_string(),
            "00000000-0000-0000-0000-000000000001".to_string(),
            "--k".to_string(),
            "5".to_string(),
            "--format".to_string(),
            "json".to_string(),
        ];
        let parsed = parse_eval_report_args(&args).expect("valid args");
        assert_eq!(
            parsed.window_id,
            uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap()
        );
        assert_eq!(parsed.k, 5);
        assert_eq!(parsed.format, "json");
    }

    #[test]
    fn test_parse_eval_report_args_equals_syntax() {
        let args = vec![
            "--window=00000000-0000-0000-0000-000000000002".to_string(),
            "--k=20".to_string(),
            "--format=markdown".to_string(),
        ];
        let parsed = parse_eval_report_args(&args).expect("valid args");
        assert_eq!(
            parsed.window_id,
            uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000002").unwrap()
        );
        assert_eq!(parsed.k, 20);
        assert_eq!(parsed.format, "markdown");
    }

    #[test]
    fn test_parse_eval_report_args_missing_window() {
        let args = vec!["--k".to_string(), "5".to_string()];
        let res = parse_eval_report_args(&args);
        assert!(res.is_err());
        assert!(res.unwrap_err().contains("missing required argument"));
    }

    #[test]
    fn test_parse_eval_report_args_invalid_format() {
        let args = vec![
            "--window".to_string(),
            "00000000-0000-0000-0000-000000000001".to_string(),
            "--format".to_string(),
            "xml".to_string(),
        ];
        let res = parse_eval_report_args(&args);
        assert!(res.is_err());
        assert!(res.unwrap_err().contains("must be 'json' or 'markdown'"));
    }

    #[test]
    fn test_parse_eval_replay_args_success() {
        let args = vec![
            "--from".to_string(),
            "2026-09-18T00:00:00Z".to_string(),
            "--to".to_string(),
            "2026-09-21T00:00:00Z".to_string(),
        ];
        let parsed = parse_eval_replay_args(&args).expect("valid args");
        assert_eq!(
            parsed.from,
            chrono::DateTime::parse_from_rfc3339("2026-09-18T00:00:00Z")
                .unwrap()
                .with_timezone(&chrono::Utc)
        );
        assert_eq!(
            parsed.to,
            chrono::DateTime::parse_from_rfc3339("2026-09-21T00:00:00Z")
                .unwrap()
                .with_timezone(&chrono::Utc)
        );
        assert_eq!(parsed.params.params_version, "cards-v0.2");
        assert_eq!(parsed.params_version(), "cards-v0.2");
        assert!(!parsed.generate);
    }

    #[test]
    fn test_parse_eval_replay_args_with_generate() {
        let args = vec![
            "--from".to_string(),
            "2026-09-18T00:00:00Z".to_string(),
            "--to".to_string(),
            "2026-09-21T00:00:00Z".to_string(),
            "--generate".to_string(),
        ];
        let parsed = parse_eval_replay_args(&args).expect("valid args");
        assert!(parsed.generate);
    }

    #[test]
    fn test_parse_eval_replay_args_generate_position_independent() {
        // 1. --generate at the beginning
        let args1 = vec![
            "--generate".to_string(),
            "--from".to_string(),
            "2026-09-18T00:00:00Z".to_string(),
            "--to".to_string(),
            "2026-09-21T00:00:00Z".to_string(),
        ];
        let parsed1 = parse_eval_replay_args(&args1).expect("valid args");
        assert!(parsed1.generate);

        // 2. --generate in the middle
        let args2 = vec![
            "--from".to_string(),
            "2026-09-18T00:00:00Z".to_string(),
            "--generate".to_string(),
            "--to".to_string(),
            "2026-09-21T00:00:00Z".to_string(),
        ];
        let parsed2 = parse_eval_replay_args(&args2).expect("valid args");
        assert!(parsed2.generate);

        // 3. --generate mixed with --param and --params-version
        let args3 = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--param=alpha=0.7".to_string(),
            "--generate".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--params-version=cards-v0.3".to_string(),
        ];
        let parsed3 = parse_eval_replay_args(&args3).expect("valid args");
        assert!(parsed3.generate);
        assert_eq!(parsed3.params.params_version, "cards-v0.3+alpha=0.7");
    }

    #[test]
    fn test_parse_eval_replay_args_generate_equals_syntax() {
        let args_true = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--generate=true".to_string(),
        ];
        let parsed_true = parse_eval_replay_args(&args_true).expect("valid args");
        assert!(parsed_true.generate);

        let args_false = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--generate=false".to_string(),
        ];
        let parsed_false = parse_eval_replay_args(&args_false).expect("valid args");
        assert!(!parsed_false.generate);
    }

    #[test]
    fn test_parse_eval_replay_args_generate_invalid_boolean() {
        let args = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--generate=notabool".to_string(),
        ];
        let res = parse_eval_replay_args(&args);
        assert!(res.is_err());
        assert!(res.unwrap_err().contains("invalid --generate boolean"));
    }

    #[test]
    fn test_parse_eval_replay_args_equals_and_custom_params_version() {
        let args = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--params-version=cards-v0.3".to_string(),
        ];
        let parsed = parse_eval_replay_args(&args).expect("valid args");
        assert_eq!(
            parsed.from,
            chrono::DateTime::parse_from_rfc3339("2026-09-18T00:00:00Z")
                .unwrap()
                .with_timezone(&chrono::Utc)
        );
        assert_eq!(
            parsed.to,
            chrono::DateTime::parse_from_rfc3339("2026-09-21T00:00:00Z")
                .unwrap()
                .with_timezone(&chrono::Utc)
        );
        assert_eq!(parsed.params.params_version, "cards-v0.3");
        assert_eq!(parsed.params_version(), "cards-v0.3");
    }

    #[test]
    fn test_parse_eval_replay_args_with_param_override() {
        let args = vec![
            "--from".to_string(),
            "2026-09-18T00:00:00Z".to_string(),
            "--to".to_string(),
            "2026-09-21T00:00:00Z".to_string(),
            "--param".to_string(),
            "alpha=0.7".to_string(),
        ];
        let parsed = parse_eval_replay_args(&args).expect("valid args");
        assert!((parsed.params.alpha - 0.7).abs() < f32::EPSILON);
        assert_eq!(parsed.params.params_version, "cards-v0.2+alpha=0.7");
        assert_eq!(parsed.params_version(), "cards-v0.2+alpha=0.7");
    }

    #[test]
    fn test_parse_eval_replay_args_with_param_equals_syntax() {
        let args = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--param=theta_novelty=0.85".to_string(),
        ];
        let parsed = parse_eval_replay_args(&args).expect("valid args");
        assert!((parsed.params.theta_novelty - 0.85).abs() < f32::EPSILON);
        assert_eq!(
            parsed.params.params_version,
            "cards-v0.2+theta_novelty=0.85"
        );
    }

    #[test]
    fn test_parse_eval_replay_args_multiple_params_and_custom_version() {
        let args = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--params-version=cards-v0.3".to_string(),
            "--param".to_string(),
            "alpha=0.7".to_string(),
            "--param=theta_novelty=0.85".to_string(),
        ];
        let parsed = parse_eval_replay_args(&args).expect("valid args");
        assert!((parsed.params.alpha - 0.7).abs() < f32::EPSILON);
        assert!((parsed.params.theta_novelty - 0.85).abs() < f32::EPSILON);
        assert_eq!(
            parsed.params.params_version,
            "cards-v0.3+alpha=0.7,theta_novelty=0.85"
        );
    }

    #[test]
    fn test_parse_eval_replay_args_invalid_param_no_equal() {
        let args = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--param".to_string(),
            "no_equal_here".to_string(),
        ];
        let res = parse_eval_replay_args(&args);
        assert!(res.is_err());
        assert!(res.unwrap_err().contains("expected key=value"));
    }

    #[test]
    fn test_parse_eval_replay_args_invalid_param_unknown_key() {
        let args = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--param".to_string(),
            "unknown_key=123".to_string(),
        ];
        let res = parse_eval_replay_args(&args);
        assert!(res.is_err());
        assert!(
            res.unwrap_err()
                .contains("unknown CardsParams override key")
        );
    }

    #[test]
    fn test_parse_eval_replay_args_invalid_param_bad_value() {
        let args = vec![
            "--from=2026-09-18T00:00:00Z".to_string(),
            "--to=2026-09-21T00:00:00Z".to_string(),
            "--param".to_string(),
            "alpha=not_a_float".to_string(),
        ];
        let res = parse_eval_replay_args(&args);
        assert!(res.is_err());
        assert!(res.unwrap_err().contains("invalid float string for alpha"));
    }

    #[test]
    fn test_parse_eval_replay_args_missing_from() {
        let args = vec!["--to".to_string(), "2026-09-21T00:00:00Z".to_string()];
        let res = parse_eval_replay_args(&args);
        assert!(res.is_err());
        assert!(
            res.unwrap_err()
                .contains("missing required argument: --from")
        );
    }

    #[test]
    fn test_parse_eval_replay_args_missing_to() {
        let args = vec!["--from".to_string(), "2026-09-18T00:00:00Z".to_string()];
        let res = parse_eval_replay_args(&args);
        assert!(res.is_err());
        assert!(res.unwrap_err().contains("missing required argument: --to"));
    }

    #[test]
    fn test_parse_eval_replay_args_invalid_rfc3339() {
        let args = vec![
            "--from".to_string(),
            "not-a-date".to_string(),
            "--to".to_string(),
            "2026-09-21T00:00:00Z".to_string(),
        ];
        let res = parse_eval_replay_args(&args);
        assert!(res.is_err());
        assert!(
            res.unwrap_err()
                .contains("invalid --from RFC3339 timestamp")
        );
    }

    static ENV_MUTEX: std::sync::LazyLock<std::sync::Mutex<()>> =
        std::sync::LazyLock::new(|| std::sync::Mutex::new(()));

    #[test]
    fn test_eval_report_config_resolves_password_file() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        let temp_dir = tempfile::tempdir().expect("temp dir");
        let password_file = temp_dir.path().join("db_password.txt");
        std::fs::write(&password_file, "secret_from_file_456\n").expect("write password file");

        let vars = vec![
            ("RECAP_DB_DSN", None),
            ("DATABASE_URL", None),
            ("RECAP_DB_HOST", Some("127.0.0.1")),
            ("RECAP_DB_PORT", Some("5432")),
            ("RECAP_DB_USER", Some("recap_user")),
            ("RECAP_DB_NAME", Some("recap_test")),
            ("RECAP_DB_PASSWORD", None),
            (
                "RECAP_DB_PASSWORD_FILE",
                Some(password_file.to_str().unwrap()),
            ),
            ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
            ("SUBWORKER_BASE_URL", Some("http://localhost:8002/")),
            ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
            ("RECAP_KNOWLEDGE_EMIT", Some("false")),
            ("RECAP_ADMIN_AUTH", Some("disabled")),
            ("RECAP_CARDS_JOB", Some("disabled")),
            ("RECAP_EVAL_LISTENER", Some("disabled")),
        ];

        temp_env::with_vars(vars, || {
            let config = recap_worker::config::Config::from_env().expect("config should load");
            assert_eq!(
                config.recap_db_dsn(),
                "postgres://recap_user:secret_from_file_456@127.0.0.1:5432/recap_test"
            );
        });
    }
}
