use std::time::{Duration, SystemTime};

/// Classifies a leaf against the 2/3-lifetime renewal policy.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum State {
    Missing,
    Fresh,
    NearExpiry,
    Expired,
    Corrupt,
}

impl State {
    #[must_use]
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Missing => "missing",
            Self::Fresh => "fresh",
            Self::NearExpiry => "near_expiry",
            Self::Expired => "expired",
            Self::Corrupt => "corrupt",
        }
    }
}

impl std::fmt::Display for State {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

/// Identical inputs produce identical outputs. `renew_at_fraction` 0.66 means
/// "renew when 66% of the window elapsed".
#[must_use]
pub fn classify_remaining(
    not_before: SystemTime,
    not_after: SystemTime,
    now: SystemTime,
    renew_at_fraction: f64,
) -> State {
    if now >= not_after {
        return State::Expired;
    }
    let Ok(total) = not_after.duration_since(not_before) else {
        return State::Expired;
    };
    if total == Duration::ZERO {
        return State::Expired;
    }
    let elapsed = now.duration_since(not_before).unwrap_or(Duration::ZERO);
    if elapsed.as_secs_f64() / total.as_secs_f64() >= renew_at_fraction {
        return State::NearExpiry;
    }
    State::Fresh
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::UNIX_EPOCH;

    fn ts(secs: u64) -> SystemTime {
        UNIX_EPOCH + Duration::from_secs(secs)
    }

    #[test]
    fn classify_remaining_table() {
        let nb = ts(1_000_000);
        let na = nb + Duration::from_secs(24 * 3600);
        let cases = [
            (
                "fresh at start",
                nb + Duration::from_secs(3600),
                State::Fresh,
            ),
            (
                "fresh just before threshold",
                nb + Duration::from_secs(15 * 3600),
                State::Fresh,
            ),
            (
                "near_expiry at 2/3",
                nb + Duration::from_secs(16 * 3600),
                State::NearExpiry,
            ),
            (
                "near_expiry past threshold",
                nb + Duration::from_secs(20 * 3600),
                State::NearExpiry,
            ),
            ("expired at not_after", na, State::Expired),
            (
                "expired after not_after",
                na + Duration::from_secs(60),
                State::Expired,
            ),
        ];
        for (name, now, want) in cases {
            assert_eq!(classify_remaining(nb, na, now, 0.66), want, "{name}");
        }
    }

    #[test]
    fn classify_remaining_zero_window() {
        let nb = ts(1_000_000);
        assert_eq!(
            classify_remaining(nb, nb, nb - Duration::from_secs(1), 0.66),
            State::Expired
        );
    }
}
