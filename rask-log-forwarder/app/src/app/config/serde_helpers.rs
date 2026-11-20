use serde::{Deserialize, Deserializer, Serializer, ser::Error as _};
use std::time::Duration;

/// Serializes a `Duration` as a `u64` millisecond count.
///
/// # Errors
///
/// Returns an error if `duration.as_millis()` exceeds `u64::MAX` or the
/// serializer fails.
pub fn serialize<S>(duration: &Duration, serializer: S) -> Result<S::Ok, S::Error>
where
    S: Serializer,
{
    let millis = u64::try_from(duration.as_millis())
        .map_err(|_| S::Error::custom("duration milliseconds exceed u64::MAX"))?;
    serializer.serialize_u64(millis)
}

/// Deserializes a `Duration` from a `u64` millisecond count.
///
/// # Errors
///
/// Returns an error if the deserializer fails to produce a `u64`.
pub fn deserialize<'de, D>(deserializer: D) -> Result<Duration, D::Error>
where
    D: Deserializer<'de>,
{
    let millis = u64::deserialize(deserializer)?;
    Ok(Duration::from_millis(millis))
}

/// Helper function to load and parse an environment variable using a custom lookup.
/// Returns `Ok(())` if the variable doesn't exist (keeps default).
///
/// # Errors
///
/// Returns [`super::ConfigError::EnvError`] if the variable is present but
/// fails to parse into `T`.
pub fn load_env_var_with<T, F>(
    name: &str,
    target: &mut T,
    env: &F,
) -> Result<(), super::ConfigError>
where
    T: std::str::FromStr,
    T::Err: std::fmt::Display,
    F: Fn(&str) -> Option<String>,
{
    if let Some(value) = env(name) {
        *target = value
            .parse()
            .map_err(|e| super::ConfigError::EnvError(format!("Invalid {name}: {e}")))?;
    }
    Ok(())
}

/// Helper function to load and parse an environment variable.
/// Returns `Ok(())` if the variable doesn't exist (keeps default).
///
/// # Errors
///
/// Returns [`super::ConfigError::EnvError`] if the variable is present in the
/// process environment but fails to parse into `T`.
pub fn load_env_var<T>(name: &str, target: &mut T) -> Result<(), super::ConfigError>
where
    T: std::str::FromStr,
    T::Err: std::fmt::Display,
{
    load_env_var_with(name, target, &|k| std::env::var(k).ok())
}

/// Helper function to load an optional string environment variable using a custom lookup.
pub fn load_env_string_opt_with<F>(name: &str, target: &mut Option<String>, env: &F)
where
    F: Fn(&str) -> Option<String>,
{
    if let Some(value) = env(name) {
        *target = Some(value);
    }
}

/// Helper function to load an optional string environment variable.
pub fn load_env_string_opt(name: &str, target: &mut Option<String>) {
    load_env_string_opt_with(name, target, &|k| std::env::var(k).ok());
}

/// Helper function to load a string environment variable using a custom lookup.
pub fn load_env_string_with<F>(name: &str, target: &mut String, env: &F)
where
    F: Fn(&str) -> Option<String>,
{
    if let Some(value) = env(name) {
        *target = value;
    }
}

/// Helper function to load a string environment variable.
pub fn load_env_string(name: &str, target: &mut String) {
    load_env_string_with(name, target, &|k| std::env::var(k).ok());
}

/// Helper function to load a `PathBuf` environment variable using a custom lookup.
pub fn load_env_path_with<F>(name: &str, target: &mut std::path::PathBuf, env: &F)
where
    F: Fn(&str) -> Option<String>,
{
    if let Some(value) = env(name) {
        *target = std::path::PathBuf::from(value);
    }
}

/// Helper function to load a `PathBuf` environment variable.
pub fn load_env_path(name: &str, target: &mut std::path::PathBuf) {
    load_env_path_with(name, target, &|k| std::env::var(k).ok());
}

/// Helper function to load an optional `PathBuf` environment variable using a custom lookup.
pub fn load_env_path_opt_with<F>(name: &str, target: &mut Option<std::path::PathBuf>, env: &F)
where
    F: Fn(&str) -> Option<String>,
{
    if let Some(value) = env(name) {
        *target = Some(std::path::PathBuf::from(value));
    }
}

/// Helper function to load an optional `PathBuf` environment variable.
pub fn load_env_path_opt(name: &str, target: &mut Option<std::path::PathBuf>) {
    load_env_path_opt_with(name, target, &|k| std::env::var(k).ok());
}
