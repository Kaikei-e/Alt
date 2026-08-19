use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

use prometheus::{CounterVec, Gauge, Opts, Registry};

use super::state::State;

/// Observer receives lifecycle events. Tests record; production logs/metrics.
pub trait Observer: Send + Sync {
    fn on_classified(&self, state: State, remaining_secs: f64);
    fn on_reissued(&self, reason: &str);
    fn on_renewed(&self, success: bool);
    fn on_retry(&self, attempt: u32, err: &str);
}

pub struct NopObserver;

impl Observer for NopObserver {
    fn on_classified(&self, _state: State, _remaining_secs: f64) {}
    fn on_reissued(&self, _reason: &str) {}
    fn on_renewed(&self, _success: bool) {}
    fn on_retry(&self, _attempt: u32, _err: &str) {}
}

/// Publishes the in-process enrollment metric family on the private PKI
/// registry (ops listener, default `127.0.0.1:9110`), never the app `/metrics`.
pub struct PromObserver {
    not_after: Gauge,
    remaining: Gauge,
    last_rotation: Gauge,
    renewal_total: CounterVec,
    reissue_total: CounterVec,
    healthy: Gauge,
    state: Mutex<State>,
}

impl PromObserver {
    pub fn new(subject: &str, registry: &Registry) -> Result<Self, prometheus::Error> {
        let not_after = Gauge::with_opts(
            Opts::new(
                "cert_not_after_seconds",
                "Unix timestamp of the current leaf certificate's not_after.",
            )
            .namespace("pki_enrollment")
            .const_label("subject", subject),
        )?;
        let remaining = Gauge::with_opts(
            Opts::new(
                "cert_remaining_seconds",
                "Seconds until the current leaf expires. Negative if expired.",
            )
            .namespace("pki_enrollment")
            .const_label("subject", subject),
        )?;
        let last_rotation = Gauge::with_opts(
            Opts::new(
                "last_rotation_timestamp_seconds",
                "Unix timestamp of the last successful cert rotation.",
            )
            .namespace("pki_enrollment")
            .const_label("subject", subject),
        )?;
        let renewal_total = CounterVec::new(
            Opts::new(
                "renewal_total",
                "Count of completed rotation attempts grouped by outcome.",
            )
            .namespace("pki_enrollment")
            .const_label("subject", subject),
            &["result"],
        )?;
        let reissue_total = CounterVec::new(
            Opts::new(
                "reissue_total",
                "Count of reissuances by reason (missing / expired / near_expiry / corrupt).",
            )
            .namespace("pki_enrollment")
            .const_label("subject", subject),
            &["reason"],
        )?;
        let healthy = Gauge::with_opts(
            Opts::new(
                "healthy",
                "1 if the cert on disk is currently valid (not expired).",
            )
            .namespace("pki_enrollment")
            .const_label("subject", subject),
        )?;
        registry.register(Box::new(not_after.clone()))?;
        registry.register(Box::new(remaining.clone()))?;
        registry.register(Box::new(last_rotation.clone()))?;
        registry.register(Box::new(renewal_total.clone()))?;
        registry.register(Box::new(reissue_total.clone()))?;
        registry.register(Box::new(healthy.clone()))?;
        Ok(Self {
            not_after,
            remaining,
            last_rotation,
            renewal_total,
            reissue_total,
            healthy,
            state: Mutex::new(State::Missing),
        })
    }
}

impl Observer for PromObserver {
    fn on_classified(&self, state: State, remaining_secs: f64) {
        *self
            .state
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner) = state;
        self.remaining.set(remaining_secs);
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_or(0.0, |d| d.as_secs_f64());
        self.not_after.set(now + remaining_secs);
        match state {
            State::Expired | State::Corrupt | State::Missing => self.healthy.set(0.0),
            State::Fresh | State::NearExpiry => self.healthy.set(1.0),
        }
    }

    fn on_reissued(&self, reason: &str) {
        self.reissue_total.with_label_values(&[reason]).inc();
    }

    fn on_renewed(&self, success: bool) {
        if success {
            let now = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .map_or(0.0, |d| d.as_secs_f64());
            self.last_rotation.set(now);
            self.renewal_total.with_label_values(&["success"]).inc();
        } else {
            self.renewal_total.with_label_values(&["failure"]).inc();
        }
    }

    fn on_retry(&self, _attempt: u32, _err: &str) {}
}

pub(crate) fn render_registry(registry: &Registry) -> String {
    use prometheus::{Encoder, TextEncoder};
    let encoder = TextEncoder::new();
    let families = registry.gather();
    let mut buf = Vec::new();
    let _ = encoder.encode(&families, &mut buf);
    String::from_utf8(buf).unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exports_lifecycle_metrics() {
        let reg = Registry::new();
        let obs = PromObserver::new("recap-worker", &reg).unwrap();
        obs.on_classified(State::Fresh, f64::from(8 * 3600));
        let body = render_registry(&reg);
        assert!(
            body.contains("pki_enrollment_healthy{subject=\"recap-worker\"} 1"),
            "{body}"
        );
        assert!(
            body.contains("pki_enrollment_cert_remaining_seconds{subject=\"recap-worker\"} 28800"),
            "{body}"
        );

        obs.on_renewed(true);
        obs.on_renewed(false);
        obs.on_reissued("expired");
        obs.on_classified(State::Expired, -60.0);
        let body = render_registry(&reg);
        for needle in [
            "pki_enrollment_healthy{subject=\"recap-worker\"} 0",
            "pki_enrollment_renewal_total{result=\"success\",subject=\"recap-worker\"} 1",
            "pki_enrollment_renewal_total{result=\"failure\",subject=\"recap-worker\"} 1",
            "pki_enrollment_reissue_total{reason=\"expired\",subject=\"recap-worker\"} 1",
        ] {
            assert!(body.contains(needle), "missing {needle}\n{body}");
        }
    }
}
