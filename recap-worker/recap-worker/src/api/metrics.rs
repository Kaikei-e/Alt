use axum::{extract::State, http::StatusCode, response::IntoResponse};

use crate::app::AppState;

pub(crate) async fn exporter(State(state): State<AppState>) -> impl IntoResponse {
    (StatusCode::OK, state.telemetry().render_prometheus()).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::observability::Telemetry;

    #[tokio::test]
    async fn metrics_endpoint_renders_surface_health_gauges_for_all_surfaces() {
        let telemetry = Telemetry::new().expect("telemetry");
        let resp = (StatusCode::OK, telemetry.render_prometheus()).into_response();
        assert_eq!(resp.status(), StatusCode::OK);
        let bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
            .await
            .expect("body bytes");
        let text = String::from_utf8(bytes.to_vec()).expect("utf8");

        assert!(text.contains("# TYPE recap_surface_last_success_timestamp_seconds gauge"));
        assert!(text.contains("# TYPE recap_surface_consecutive_failures gauge"));
        for surface in [
            "three_day_recap",
            "topic_cards",
            "evening_pulse",
            "morning_letter",
        ] {
            assert!(
                text.contains(&format!(
                    "recap_surface_last_success_timestamp_seconds{{surface=\"{surface}\"}}"
                )),
                "missing last_success series for {surface}"
            );
            assert!(
                text.contains(&format!(
                    "recap_surface_consecutive_failures{{surface=\"{surface}\"}}"
                )),
                "missing consecutive_failures series for {surface}"
            );
        }
    }
}
