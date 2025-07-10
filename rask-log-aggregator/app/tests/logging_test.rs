use tracing::{error, info};
use tracing_test::{logs_assert, traced_test};

#[traced_test]
#[test]
fn test_info_logging() {
    info!("This is an info message");
    logs_assert::contains("This is an info message");
}

#[traced_test]
#[test]
fn test_error_logging() {
    error!("This is an error message");
    logs_assert::contains("This is an error message");
}
