use chrono::{DateTime, FixedOffset, LocalResult, NaiveDate, NaiveTime, TimeZone, Utc};

#[derive(Debug, Clone)]
pub(crate) struct DailyCadence {
    tz: FixedOffset,
    target: NaiveTime,
}

impl DailyCadence {
    pub(crate) fn new(tz: FixedOffset, hour: u32, minute: u32) -> Self {
        let target = NaiveTime::from_hms_opt(hour, minute, 0)
            .unwrap_or_else(|| panic!("invalid time: {hour:02}:{minute:02}"));
        Self { tz, target }
    }

    pub(crate) fn next_run_from(&self, now: DateTime<Utc>) -> DateTime<Utc> {
        let localized_now = now.with_timezone(&self.tz);
        let mut date = localized_now.date_naive();
        if localized_now.time() > self.target {
            date = advance_day(date);
        }

        let local_target = date.and_time(self.target);

        match self.tz.from_local_datetime(&local_target) {
            LocalResult::Single(dt) => dt.with_timezone(&Utc),
            LocalResult::Ambiguous(first, _) => first.with_timezone(&Utc),
            LocalResult::None => unreachable!("fixed offset should not produce nonexistent times"),
        }
    }
}

fn advance_day(date: NaiveDate) -> NaiveDate {
    date.succ_opt()
        .expect("date should remain representable when advancing")
}

#[cfg(test)]
mod tests {
    use super::DailyCadence;
    use chrono::{DateTime, FixedOffset, Utc};

    fn parse_utc(ts: &str) -> DateTime<Utc> {
        DateTime::parse_from_rfc3339(ts)
            .expect("valid datetime")
            .with_timezone(&Utc)
    }

    fn jst() -> FixedOffset {
        FixedOffset::east_opt(9 * 3600).expect("jst offset")
    }

    #[test]
    fn next_run_same_day_when_before_trigger() {
        let cadence = DailyCadence::new(jst(), 4, 0);
        let now = parse_utc("2025-11-08T18:30:00Z"); // 03:30 JST (same calendar day)
        let expected = parse_utc("2025-11-08T19:00:00Z"); // 04:00 JST
        let next = cadence.next_run_from(now);
        assert_eq!(next, expected);
    }

    #[test]
    fn next_run_next_day_when_past_trigger() {
        let cadence = DailyCadence::new(jst(), 4, 0);
        let now = parse_utc("2025-11-08T10:00:00Z"); // 19:00 JST (already past 04:00)
        let expected = parse_utc("2025-11-08T19:00:00Z"); // Next day's 04:00 JST
        let next = cadence.next_run_from(now);
        assert_eq!(next, expected);
    }

    #[test]
    fn next_run_immediate_when_exact_trigger() {
        let cadence = DailyCadence::new(jst(), 4, 0);
        let now = parse_utc("2025-11-08T19:00:00Z"); // Exactly 04:00 JST
        let next = cadence.next_run_from(now);
        assert_eq!(next, now);
    }
}
