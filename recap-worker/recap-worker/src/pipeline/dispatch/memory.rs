//! Memory monitoring utilities for dispatch stage.

use std::fs;

/// Read current process memory from /proc/self/status.
/// Returns (RSS in KB, Peak in KB) if available.
pub(crate) fn read_process_memory_kb() -> Option<(u64, u64)> {
    let status = fs::read_to_string("/proc/self/status").ok()?;
    let mut rss_kb: Option<u64> = None;
    let mut peak_kb: Option<u64> = None;

    for line in status.lines() {
        if let Some(value) = line.strip_prefix("VmRSS:") {
            rss_kb = value
                .split_whitespace()
                .next()
                .and_then(|raw| raw.parse::<u64>().ok());
        } else if let Some(value) = line.strip_prefix("VmHWM:") {
            peak_kb = value
                .split_whitespace()
                .next()
                .and_then(|raw| raw.parse::<u64>().ok());
        }
    }

    match (rss_kb, peak_kb) {
        (Some(rss), Some(peak)) => Some((rss, peak)),
        (Some(rss), None) => Some((rss, rss)),
        (None, Some(peak)) => Some((peak, peak)),
        _ => None,
    }
}
