use axum::{routing::get, Router};

#[tokio::main]
async fn main() {
    let app: Router = Router::new().route("/", get(|| async { "Hello, World!" }));

    let v1_aggregate_router: Router = Router::new().route("/v1/aggregate", get(|| async { "Hello, World!" }));

    let listener = tokio::net::TcpListener::bind("0.0.0.0:9600").await.unwrap();

    println!("Listening on {}", listener.local_addr().unwrap());

    axum::serve(listener, v1_aggregate_router).await.unwrap();
}
