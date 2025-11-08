/// 構造化JSON形式ログ。
use serde_json::json;
use tracing::{Event, Subscriber};
use tracing_subscriber::Layer;
use tracing_subscriber::layer::Context;

/// 重要イベントの構造化ログレイヤー。
#[allow(dead_code)]
pub(crate) struct StructuredLogLayer;

impl<S: Subscriber> Layer<S> for StructuredLogLayer {
    fn on_event(&self, event: &Event<'_>, _ctx: Context<'_, S>) {
        use tracing::field::Visit;

        struct JsonVisitor {
            values: serde_json::Map<String, serde_json::Value>,
        }

        impl Visit for JsonVisitor {
            fn record_debug(&mut self, field: &tracing::field::Field, value: &dyn std::fmt::Debug) {
                self.values
                    .insert(field.name().to_string(), json!(format!("{:?}", value)));
            }

            fn record_str(&mut self, field: &tracing::field::Field, value: &str) {
                self.values.insert(field.name().to_string(), json!(value));
            }

            fn record_i64(&mut self, field: &tracing::field::Field, value: i64) {
                self.values.insert(field.name().to_string(), json!(value));
            }

            fn record_u64(&mut self, field: &tracing::field::Field, value: u64) {
                self.values.insert(field.name().to_string(), json!(value));
            }

            fn record_bool(&mut self, field: &tracing::field::Field, value: bool) {
                self.values.insert(field.name().to_string(), json!(value));
            }
        }

        let mut visitor = JsonVisitor {
            values: serde_json::Map::new(),
        };
        event.record(&mut visitor);

        let log_entry = json!({
            "timestamp": chrono::Utc::now().to_rfc3339(),
            "level": event.metadata().level().as_str(),
            "target": event.metadata().target(),
            "message": event.metadata().name(),
            "fields": visitor.values,
        });

        // 重要イベントのみJSON形式で出力
        if matches!(
            event.metadata().level(),
            &tracing::Level::ERROR | &tracing::Level::WARN | &tracing::Level::INFO
        ) {
            eprintln!("{}", serde_json::to_string(&log_entry).unwrap_or_default());
        }
    }
}
