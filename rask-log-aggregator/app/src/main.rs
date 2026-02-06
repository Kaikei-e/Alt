use rask::error::AggregatorError;

#[tokio::main]
async fn main() -> Result<(), AggregatorError> {
    rask::app::run().await
}
