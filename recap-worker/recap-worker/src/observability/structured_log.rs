/// 構造化JSON形式ログ (ADR 98 準拠)。
use serde_json::json;
use tracing::{Event, Subscriber};
use tracing_subscriber::Layer;
use tracing_subscriber::layer::Context;

/// ADR 98 準拠のフィールド名変換
/// job_id -> alt.job.id, article_id -> alt.article.id, etc.
fn convert_to_adr98_key(key: &str) -> String {
    match key {
        "job_id" => "alt.job.id".to_string(),
        "article_id" => "alt.article.id".to_string(),
        "processing_stage" => "alt.processing.stage".to_string(),
        "ai_pipeline" => "alt.ai.pipeline".to_string(),
        _ => key.to_string(),
    }
}

/// 重要イベントの構造化ログレイヤー (ADR 98 準拠)。
///
/// 全てのトレーシングイベントに対してADR 98で定義された
/// alt.* プレフィックス付きフィールド変換を適用します。
pub(crate) struct StructuredLogLayer;

impl<S: Subscriber> Layer<S> for StructuredLogLayer {
    fn on_event(&self, event: &Event<'_>, _ctx: Context<'_, S>) {
        use tracing::field::Visit;

        struct JsonVisitor {
            values: serde_json::Map<String, serde_json::Value>,
        }

        impl Visit for JsonVisitor {
            fn record_debug(&mut self, field: &tracing::field::Field, value: &dyn std::fmt::Debug) {
                let key = convert_to_adr98_key(field.name());
                self.values.insert(key, json!(format!("{:?}", value)));
            }

            fn record_str(&mut self, field: &tracing::field::Field, value: &str) {
                let key = convert_to_adr98_key(field.name());
                self.values.insert(key, json!(value));
            }

            fn record_i64(&mut self, field: &tracing::field::Field, value: i64) {
                let key = convert_to_adr98_key(field.name());
                self.values.insert(key, json!(value));
            }

            fn record_u64(&mut self, field: &tracing::field::Field, value: u64) {
                let key = convert_to_adr98_key(field.name());
                self.values.insert(key, json!(value));
            }

            fn record_bool(&mut self, field: &tracing::field::Field, value: bool) {
                let key = convert_to_adr98_key(field.name());
                self.values.insert(key, json!(value));
            }
        }

        let mut visitor = JsonVisitor {
            values: serde_json::Map::new(),
        };
        event.record(&mut visitor);

        // ADR 98: Always include ai_pipeline for recap-worker
        visitor
            .values
            .insert("alt.ai.pipeline".to_string(), json!("recap-processing"));

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
