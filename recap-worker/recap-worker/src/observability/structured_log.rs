//! 構造化JSON形式ログ (ADR 98 準拠)。
use serde_json::json;
use std::fmt;
use tracing::{Event, Subscriber};
use tracing_subscriber::fmt::FmtContext;
use tracing_subscriber::fmt::format::{FormatEvent, FormatFields, Writer};
use tracing_subscriber::registry::LookupSpan;

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

/// ADR 98 準拠の JSON イベントフォーマッタ。
///
/// `tracing_subscriber::fmt` レイヤーの `FormatEvent` として差し込む — 素の
/// `fmt::layer().json()` と別レイヤーで両方登録すると全イベントが二重出力
/// されるため、この 1 レイヤーだけが唯一の JSON 出力経路になる。`fmt` の
/// writer 機構を使うので手書きの `eprintln!` も不要。
pub(crate) struct Adr98JsonFormat;

impl<S, N> FormatEvent<S, N> for Adr98JsonFormat
where
    S: Subscriber + for<'a> LookupSpan<'a>,
    N: for<'a> FormatFields<'a> + 'static,
{
    fn format_event(
        &self,
        _ctx: &FmtContext<'_, S, N>,
        mut writer: Writer<'_>,
        event: &Event<'_>,
    ) -> fmt::Result {
        use tracing::field::Visit;

        struct JsonVisitor {
            values: serde_json::Map<String, serde_json::Value>,
            message: Option<String>,
        }

        impl Visit for JsonVisitor {
            fn record_debug(&mut self, field: &tracing::field::Field, value: &dyn fmt::Debug) {
                if field.name() == "message" {
                    self.message = Some(format!("{value:?}"));
                    return;
                }
                let key = convert_to_adr98_key(field.name());
                self.values.insert(key, json!(format!("{value:?}")));
            }

            fn record_str(&mut self, field: &tracing::field::Field, value: &str) {
                if field.name() == "message" {
                    self.message = Some(value.to_string());
                    return;
                }
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
            message: None,
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
            // The visited `message` field is the actual log message; falling
            // back to the event's metadata name (`event src/foo.rs:123`) only
            // when a caller emits an event with no message field at all.
            "message": visitor.message.unwrap_or_else(|| event.metadata().name().to_string()),
            "fields": visitor.values,
        });

        let serialized = serde_json::to_string(&log_entry).map_err(|_| fmt::Error)?;
        writeln!(writer, "{serialized}")
    }
}
