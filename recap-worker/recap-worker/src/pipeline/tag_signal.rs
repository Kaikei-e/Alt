use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

/// Tag Generatorが付与したタグ情報。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize, Default)]
#[serde(rename_all = "snake_case")]
pub(crate) struct TagSignal {
    pub(crate) label: String,
    #[serde(default)]
    pub(crate) confidence: f32,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub(crate) source: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub(crate) source_ts: Option<DateTime<Utc>>,
}

impl TagSignal {
    #[must_use]
    pub(crate) fn new(
        label: impl Into<String>,
        confidence: f32,
        source: Option<String>,
        source_ts: Option<DateTime<Utc>>,
    ) -> Self {
        Self {
            label: label.into(),
            confidence: confidence.clamp(0.0, 1.0),
            source,
            source_ts,
        }
    }
}
