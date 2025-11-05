#![allow(dead_code)]

use anyhow::Result;
use serde_json::Value;

pub(crate) fn extract_outer_object(payload: &str) -> Result<Value> {
    let value: Value = serde_json::from_str(payload)?;
    Ok(value)
}
