pub mod config;
pub mod docker;
pub mod service;

pub use config::{Config, ConfigError, LogLevel};
pub use service::{ServiceError, ServiceManager, ShutdownHandle};

use clap::Parser;
use std::process;
use std::sync::Once;
use tracing::{error, info};
use tracing_subscriber::{EnvFilter, fmt, prelude::*};

static INIT: Once = Once::new();

pub struct App {
    service_manager: ServiceManager,
}

impl App {
    pub async fn from_args<I, T>(args: I) -> Result<Self, Box<dyn std::error::Error + Send + Sync>>
    where
        I: IntoIterator<Item = T>,
        T: Into<std::ffi::OsString> + Clone,
    {
        let config = Config::from_args_and_env(args)?;
        Self::from_config(config).await
    }

    pub async fn from_config(
        config: Config,
    ) -> Result<Self, Box<dyn std::error::Error + Send + Sync>> {
        // Setup logging first
        setup_logging(config.log_level)?;

        info!("Starting rask-log-forwarder v{}", env!("CARGO_PKG_VERSION"));
        info!(
            "Configuration: target_service={:?}, endpoint={}, batch_size={}",
            config.target_service, config.endpoint, config.batch_size
        );

        // Load config file if specified
        let final_config = if let Some(config_file) = &config.config_file {
            info!("Loading configuration from file: {}", config_file.display());
            Config::from_file(config_file)?
        } else {
            config
        };

        // Initialize service manager
        let service_manager = ServiceManager::new(final_config).await?;

        Ok(Self { service_manager })
    }

    pub async fn run(mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        // Start the service
        let shutdown_handle = self.service_manager.start().await?;

        info!("rask-log-forwarder is running. Press Ctrl+C to stop.");

        // Wait for shutdown signal
        shutdown_handle.wait_for_shutdown().await;

        info!("rask-log-forwarder stopped.");
        Ok(())
    }

    pub fn get_target_service(&self) -> &str {
        self.service_manager.get_target_service()
    }

    pub async fn health_check(&self) -> crate::reliability::HealthReport {
        self.service_manager.get_health_report().await
    }
}

pub fn setup_logging(log_level: LogLevel) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    INIT.call_once(|| {
        let level: tracing::Level = log_level.into();

        let filter = EnvFilter::builder()
            .with_default_directive(level.into())
            .from_env_lossy()
            .add_directive("hyper=warn".parse().unwrap())
            .add_directive("reqwest=warn".parse().unwrap())
            .add_directive("h2=warn".parse().unwrap());

        let _ = tracing_subscriber::registry()
            .with(
                fmt::layer()
                    .with_target(true)
                    .with_thread_ids(true)
                    .with_level(true)
                    .with_ansi(true),
            )
            .with(filter)
            .try_init();
    });

    Ok(())
}

pub fn get_version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

// Main entry point for the application
pub async fn main() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let args: Vec<String> = std::env::args().collect();

    // Handle version flag specially
    if args.len() > 1 && (args[1] == "--version" || args[1] == "-V") {
        println!("rask-log-forwarder {}", get_version());
        return Ok(());
    }

    // Handle help flag
    if args.len() > 1 && (args[1] == "--help" || args[1] == "-h") {
        Config::parse_from(["rask-log-forwarder", "--help"]);
        return Ok(());
    }

    match App::from_args(args).await {
        Ok(app) => {
            if let Err(e) = app.run().await {
                error!("Application error: {}", e);
                process::exit(1);
            }
        }
        Err(e) => {
            error!("Configuration error: {}", e);
            process::exit(1);
        }
    }

    Ok(())
}
