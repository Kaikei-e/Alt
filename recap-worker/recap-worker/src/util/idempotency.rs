#![allow(dead_code)]

use uuid::Uuid;

pub(crate) fn idempotency_key(job_id: Uuid, genre: &str) -> String {
    format!("{job_id}:{genre}")
}
