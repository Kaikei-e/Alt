table "recap_subworker_runs" {
  schema = schema.public
  column "id" {
    null    = false
    type    = bigserial
  }
  column "job_id" {
    null = false
    type = uuid
  }
  column "genre" {
    null = false
    type = text
  }
  column "status" {
    null    = false
    type    = text
  }
  column "cluster_count" {
    null    = false
    type    = integer
    default = 0
  }
  column "started_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  column "finished_at" {
    null = true
    type = timestamptz
  }
  column "request_payload" {
    null    = false
    type    = jsonb
    default = sql("'{}'::jsonb")
  }
  column "response_payload" {
    null = true
    type = jsonb
  }
  column "error_message" {
    null = true
    type = text
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_recap_subworker_runs_job_id" {
    columns = [column.job_id]
  }
  index "idx_recap_subworker_runs_request_payload_gin" {
    type = GIN
    on {
      column = column.request_payload
      ops    = jsonb_path_ops
    }
  }
  index "idx_recap_subworker_runs_response_payload_gin" {
    type = GIN
    on {
      column = column.response_payload
      ops    = jsonb_path_ops
    }
  }
}

table "recap_subworker_clusters" {
  schema = schema.public
  column "id" {
    null    = false
    type    = bigserial
  }
  column "run_id" {
    null = false
    type = bigint
  }
  column "cluster_id" {
    null = false
    type = integer
  }
  column "size" {
    null = false
    type = integer
  }
  column "label" {
    null = true
    type = text
  }
  column "top_terms" {
    null = false
    type = jsonb
  }
  column "stats" {
    null = false
    type = jsonb
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "recap_subworker_clusters_run_id_fkey" {
    columns     = [column.run_id]
    ref_columns = [table.recap_subworker_runs.column.id]
    on_delete   = CASCADE
  }
  unique "recap_subworker_clusters_run_id_cluster_id_key" {
    columns = [column.run_id, column.cluster_id]
  }
  index "idx_recap_subworker_clusters_run_id" {
    columns = [column.run_id]
  }
  index "idx_recap_subworker_clusters_top_terms_gin" {
    type = GIN
    on {
      column = column.top_terms
      ops    = jsonb_path_ops
    }
  }
  index "idx_recap_subworker_clusters_stats_gin" {
    type = GIN
    on {
      column = column.stats
      ops    = jsonb_path_ops
    }
  }
}

table "recap_subworker_sentences" {
  schema = schema.public
  column "id" {
    null    = false
    type    = bigserial
  }
  column "cluster_row_id" {
    null = false
    type = bigint
  }
  column "source_article_id" {
    null = false
    type = text
  }
  column "paragraph_idx" {
    null = true
    type = integer
  }
  column "sentence_id" {
    null    = false
    type    = integer
    default = 0
  }
  column "sentence_text" {
    null = false
    type = text
  }
  column "lang" {
    null    = false
    type    = text
    default = "unknown"
  }
  column "score" {
    null    = false
    type    = real
    default = 0
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "recap_subworker_sentences_cluster_row_id_fkey" {
    columns     = [column.cluster_row_id]
    ref_columns = [table.recap_subworker_clusters.column.id]
    on_delete   = CASCADE
  }
  index "idx_recap_subworker_sentences_cluster_row_id" {
    columns = [column.cluster_row_id]
  }
  unique "unique_cluster_article_sentence" {
    columns = [column.cluster_row_id, column.source_article_id, column.sentence_id]
  }
}

table "recap_cluster_evidence" {
  schema = schema.public
  column "id" {
    null = false
    type = bigserial
  }
  column "cluster_row_id" {
    null = false
    type = bigint
  }
  column "article_id" {
    null = false
    type = text
  }
  column "title" {
    null = true
    type = text
  }
  column "source_url" {
    null = true
    type = text
  }
  column "published_at" {
    null = true
    type = timestamptz
  }
  column "lang" {
    null = true
    type = text
  }
  column "rank" {
    null    = false
    type    = smallint
    default = 0
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "recap_cluster_evidence_cluster_row_id_fkey" {
    columns     = [column.cluster_row_id]
    ref_columns = [table.recap_subworker_clusters.column.id]
    on_delete   = CASCADE
  }
  unique "uniq_recap_cluster_evidence_article" {
    columns = [column.cluster_row_id, column.article_id]
  }
  index "idx_recap_cluster_evidence_cluster_rank" {
    columns = [column.cluster_row_id, column.rank]
  }
  index "idx_recap_cluster_evidence_article" {
    columns = [column.article_id]
  }
}

table "recap_subworker_diagnostics" {
  schema = schema.public
  column "run_id" {
    null = false
    type = bigint
  }
  column "metric" {
    null = false
    type = text
  }
  column "value" {
    null = false
    type = jsonb
  }
  primary_key {
    columns = [column.run_id, column.metric]
  }
  foreign_key "recap_subworker_diagnostics_run_id_fkey" {
    columns     = [column.run_id]
    ref_columns = [table.recap_subworker_runs.column.id]
    on_delete   = CASCADE
  }
  index "idx_recap_subworker_diagnostics_value_gin" {
    type = GIN
    on {
      column = column.value
      ops    = jsonb_path_ops
    }
  }
}

table "recap_run_diagnostics" {
  schema = schema.public
  column "run_id" {
    null = false
    type = bigint
  }
  column "cluster_avg_similarity_mean" {
    null = true
    type = double_precision
  }
  column "cluster_avg_similarity_variance" {
    null = true
    type = double_precision
  }
  column "cluster_avg_similarity_p95" {
    null = true
    type = double_precision
  }
  column "cluster_avg_similarity_max" {
    null = true
    type = double_precision
  }
  column "cluster_count" {
    null    = false
    type    = integer
    default = 0
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.run_id]
  }
  foreign_key "recap_run_diagnostics_run_id_fkey" {
    columns     = [column.run_id]
    ref_columns = [table.recap_subworker_runs.column.id]
    on_delete   = CASCADE
  }
  index "idx_recap_run_diagnostics_run_id" {
    columns = [column.run_id]
  }
}

table "recap_sections" {
  schema = schema.public
  column "job_id" {
    null = false
    type = uuid
  }
  column "genre" {
    null = false
    type = text
  }
  column "response_id" {
    null = true
    type = text
  }
  primary_key {
    columns = [column.job_id, column.genre]
  }
}

table "recap_jobs" {
  schema = schema.public
  column "id" {
    null    = false
    type    = bigserial
  }
  column "job_id" {
    null = false
    type = uuid
  }
  column "kicked_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  column "note" {
    null = true
    type = text
  }
  primary_key {
    columns = [column.id]
  }
  unique "recap_jobs_job_id_key" {
    columns = [column.job_id]
  }
}

table "recap_job_articles" {
  schema = schema.public
  column "id" {
    null    = false
    type    = bigserial
  }
  column "job_id" {
    null = false
    type = uuid
  }
  column "article_id" {
    null = false
    type = text
  }
  column "title" {
    null = true
    type = text
  }
  column "fulltext_html" {
    null = false
    type = text
  }
  column "published_at" {
    null = true
    type = timestamptz
  }
  column "source_url" {
    null = true
    type = text
  }
  column "lang_hint" {
    null = true
    type = text
  }
  column "normalized_hash" {
    null = false
    type = text
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "recap_job_articles_job_id_fkey" {
    columns     = [column.job_id]
    ref_columns = [table.recap_jobs.column.job_id]
    on_delete   = CASCADE
  }
  unique "recap_job_articles_job_id_article_id_key" {
    columns = [column.job_id, column.article_id]
  }
  index "idx_recap_job_articles_job" {
    columns = [column.job_id]
  }
}

table "recap_preprocess_metrics" {
  schema = schema.public
  column "job_id" {
    null = false
    type = uuid
  }
  column "total_articles_fetched" {
    null = false
    type = int
  }
  column "articles_processed" {
    null = false
    type = int
  }
  column "articles_dropped_empty" {
    null = false
    type = int
  }
  column "articles_html_cleaned" {
    null = false
    type = int
  }
  column "total_characters" {
    null = false
    type = bigint
  }
  column "avg_chars_per_article" {
    null = true
    type = double_precision
  }
  column "languages_detected" {
    null    = false
    type    = jsonb
    default = sql("'{}'::jsonb")
  }
  primary_key {
    columns = [column.job_id]
  }
  foreign_key "recap_preprocess_metrics_job_id_fkey" {
    columns     = [column.job_id]
    ref_columns = [table.recap_jobs.column.job_id]
    on_delete   = CASCADE
  }
}

table "recap_final_sections" {
  schema = schema.public
  column "job_id" {
    null = false
    type = uuid
  }
  column "genre" {
    null = false
    type = text
  }
  column "response_id" {
    null = false
    type = text
  }
  column "title_ja" {
    null = false
    type = text
  }
  column "summary_ja" {
    null = false
    type = text
  }
  column "bullets_ja" {
    null = false
    type = jsonb
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.job_id, column.genre]
  }
}

table "recap_outputs" {
  schema = schema.public
  column "job_id" {
    null = false
    type = uuid
  }
  column "genre" {
    null = false
    type = text
  }
  column "response_id" {
    null = false
    type = text
  }
  column "title_ja" {
    null = false
    type = text
  }
  column "summary_ja" {
    null = false
    type = text
  }
  column "bullets_ja" {
    null = false
    type = jsonb
  }
  column "body_json" {
    null = false
    type = jsonb
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  column "updated_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.job_id, column.genre]
  }
  index "idx_recap_outputs_body_json_gin" {
    type = GIN
    on {
      column = column.body_json
      ops    = jsonb_path_ops
    }
  }
  index "idx_recap_outputs_response_id" {
    columns = [column.response_id]
  }
}

schema "public" {
}
