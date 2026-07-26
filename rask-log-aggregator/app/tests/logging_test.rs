use tracing::{error, info};
use tracing_test::traced_test;

/// `tracing-test` 0.2.6 injects `logs_contain`/`logs_assert` into every
/// `#[traced_test]` function — the crate genuinely supports asserting on
/// captured log content, so exercising `info!`/`error!` without checking
/// what actually landed in the log buffer left these tests unable to catch
/// a dropped message, a swapped level, or a garbled format string.
#[traced_test]
#[test]
fn test_info_logging() {
    info!("This is an info message");

    assert!(
        logs_contain("This is an info message"),
        "info! message must be captured by the tracing subscriber"
    );
    assert!(
        !logs_contain("This is an error message"),
        "a message that was never logged must not be reported as present"
    );
}

#[traced_test]
#[test]
fn test_error_logging() {
    error!("This is an error message");

    assert!(
        logs_contain("This is an error message"),
        "error! message must be captured by the tracing subscriber"
    );
    assert!(
        !logs_contain("This is an info message"),
        "a message that was never logged must not be reported as present"
    );
}
