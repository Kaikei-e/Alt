//! Dynamic threshold management for the select stage.

use std::collections::HashMap;

use crate::store::dao::RecapDao;

/// Fetch dynamic thresholds from the database.
pub(crate) async fn get_dynamic_thresholds(
    dao: Option<&std::sync::Arc<dyn RecapDao>>,
) -> (HashMap<String, usize>, HashMap<String, f32>) {
    if let Some(dao) = dao {
        match dao.get_latest_worker_config("genre_distribution").await {
            Ok(Some(payload)) => {
                // payload is {"genre": {"min_docs_threshold": N, "cosine_threshold": F, ...}}
                if let Some(obj) = payload.as_object() {
                    let mut min_docs_map = HashMap::new();
                    let mut cosine_map = HashMap::new();
                    for (genre, stats) in obj {
                        if let Some(threshold) = stats
                            .get("min_docs_threshold")
                            .and_then(serde_json::Value::as_u64)
                        {
                            min_docs_map.insert(genre.clone(), threshold as usize);
                        }
                        if let Some(threshold) = stats
                            .get("cosine_threshold")
                            .and_then(serde_json::Value::as_f64)
                        {
                            cosine_map.insert(genre.clone(), threshold as f32);
                        }
                    }
                    return (min_docs_map, cosine_map);
                }
            }
            Ok(None) => {
                tracing::debug!("no dynamic genre distribution config found");
            }
            Err(e) => {
                tracing::warn!("failed to fetch dynamic genre distribution config: {}", e);
            }
        }
    }
    (HashMap::new(), HashMap::new())
}
