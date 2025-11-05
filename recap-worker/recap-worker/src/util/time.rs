#![allow(dead_code)]

use chrono::{DateTime, Utc};

pub(crate) fn now() -> DateTime<Utc> {
    Utc::now()
}
