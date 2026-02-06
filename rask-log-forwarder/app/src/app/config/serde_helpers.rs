use serde::{Deserialize, Deserializer, Serializer};
use std::time::Duration;

pub fn serialize<S>(duration: &Duration, serializer: S) -> Result<S::Ok, S::Error>
where
    S: Serializer,
{
    serializer.serialize_u64(duration.as_millis() as u64)
}

pub fn deserialize<'de, D>(deserializer: D) -> Result<Duration, D::Error>
where
    D: Deserializer<'de>,
{
    let millis = u64::deserialize(deserializer)?;
    Ok(Duration::from_millis(millis))
}

/// Helper function to load and parse an environment variable.
/// Returns Ok(()) if the variable doesn't exist (keeps default).
pub fn load_env_var<T>(name: &str, target: &mut T) -> Result<(), super::ConfigError>
where
    T: std::str::FromStr,
    T::Err: std::fmt::Display,
{
    if let Ok(value) = std::env::var(name) {
        *target = value
            .parse()
            .map_err(|e| super::ConfigError::EnvError(format!("Invalid {name}: {e}")))?;
    }
    Ok(())
}

/// Helper function to load an optional string environment variable.
pub fn load_env_string_opt(name: &str, target: &mut Option<String>) {
    if let Ok(value) = std::env::var(name) {
        *target = Some(value);
    }
}

/// Helper function to load a string environment variable.
pub fn load_env_string(name: &str, target: &mut String) {
    if let Ok(value) = std::env::var(name) {
        *target = value;
    }
}

/// Helper function to load a PathBuf environment variable.
pub fn load_env_path(name: &str, target: &mut std::path::PathBuf) {
    if let Ok(value) = std::env::var(name) {
        *target = std::path::PathBuf::from(value);
    }
}

/// Helper function to load an optional PathBuf environment variable.
pub fn load_env_path_opt(name: &str, target: &mut Option<std::path::PathBuf>) {
    if let Ok(value) = std::env::var(name) {
        *target = Some(std::path::PathBuf::from(value));
    }
}
