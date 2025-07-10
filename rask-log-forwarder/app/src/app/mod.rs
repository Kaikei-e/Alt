pub mod application_initializer;
pub mod config;
pub mod docker;
pub mod initialization;
pub mod logging_system;
pub mod service;

pub use application_initializer::{
    ApplicationInitializer, InitializationResult, InitializationStrategy,
};
pub use config::{Config, ConfigError, LogLevel};
pub use initialization::InitializationError;
pub use logging_system::{LoggingSystem, setup_logging_safe};
pub use service::{ServiceError, ServiceManager, ShutdownHandle};

use clap::Parser;
use std::process;
use tracing::{error, info};

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
        // Use the comprehensive ApplicationInitializer for memory-safe initialization
        let initializer = ApplicationInitializer::new();

        // Load config file if specified
        let final_config = if let Some(config_file) = &config.config_file {
            eprintln!("Loading configuration from file: {}", config_file.display());
            Config::from_file(config_file)?
        } else {
            config
        };

        // Initialize application with comprehensive validation
        let init_result = initializer
            .initialize(&final_config)
            .map_err(|e| -> Box<dyn std::error::Error + Send + Sync> { Box::new(e) })?;

        info!("Starting rask-log-forwarder v{}", env!("CARGO_PKG_VERSION"));
        info!(
            "Configuration: target_service={:?}, endpoint={}, batch_size={}",
            final_config.target_service, final_config.endpoint, final_config.batch_size
        );
        info!(
            "Initialization completed in {}ms (strategy: {:?})",
            init_result.initialization_time_ms,
            initializer.determine_initialization_strategy(&final_config)
        );

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

// Note: setup_logging has been replaced with setup_logging_safe in logging_system.rs
// This eliminates all expect() calls and provides memory-safe initialization

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
