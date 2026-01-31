//! Tag profile types for genre refinement.

use serde::{Deserialize, Serialize};

use crate::pipeline::tag_signal::TagSignal;

/// タグプロファイル（Tag Generator出力の要約）。
#[derive(Debug, Clone, PartialEq, Default, Serialize, Deserialize)]
pub(crate) struct TagProfile {
    pub(crate) top_tags: Vec<TagSignal>,
    pub(crate) entropy: f32,
}

impl TagProfile {
    #[must_use]
    pub(crate) fn from_signals(signals: &[TagSignal]) -> Self {
        let entropy = super::scoring::compute_entropy(signals);
        Self {
            top_tags: signals.to_vec(),
            entropy,
        }
    }

    #[must_use]
    pub(crate) fn has_tags(&self) -> bool {
        !self.top_tags.is_empty()
    }
}

/// タグ有無に応じたフォールバックモード。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum TagFallbackMode {
    CoarseOnly,
    AllowRefine,
}

impl TagFallbackMode {
    #[must_use]
    pub(crate) fn require_tags(require: bool, has_tags: bool) -> Self {
        if require && !has_tags {
            Self::CoarseOnly
        } else {
            Self::AllowRefine
        }
    }
}
