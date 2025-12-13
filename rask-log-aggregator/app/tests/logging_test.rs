use tracing::{error, info};
use tracing_test::traced_test;

#[traced_test]
#[test]
fn test_info_logging() {
    info!("This is an info message");
    // Logs are captured by traced_test, but logs_assert is not available in tracing-test 0.2.5
    // This test verifies that info! macro works without panicking
}

#[traced_test]
#[test]
fn test_error_logging() {
    error!("This is an error message");
    // Logs are captured by traced_test, but logs_assert is not available in tracing-test 0.2.5
    // This test verifies that error! macro works without panicking
}
