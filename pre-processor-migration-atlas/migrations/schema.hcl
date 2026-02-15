# Pre-Processor DB Schema
# Tables: inoreader_subscriptions, inoreader_articles, sync_state,
#          api_usage_tracking, summarize_job_queue

table "inoreader_subscriptions" {
  schema  = schema.public
  comment = "Stores RSS feed subscriptions synchronized from Inoreader API"
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
    comment = "Internal UUID primary key"
  }
  column "inoreader_id" {
    null    = false
    type    = text
    comment = "Unique identifier from Inoreader API (e.g., feed/http://example.com/rss)"
  }
  column "feed_url" {
    null    = false
    type    = text
    comment = "XML RSS feed URL"
  }
  column "title" {
    null    = true
    type    = text
    comment = "Feed title from Inoreader"
  }
  column "category" {
    null    = true
    type    = text
    comment = "Feed category/folder from Inoreader"
  }
  column "synced_at" {
    null    = true
    type    = timestamptz
    default = sql("now()")
    comment = "Last synchronization timestamp"
  }
  column "created_at" {
    null    = true
    type    = timestamptz
    default = sql("now()")
    comment = "Record creation timestamp"
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_inoreader_subscriptions_inoreader_id" {
    columns = [column.inoreader_id]
  }
  index "idx_inoreader_subscriptions_feed_url" {
    columns = [column.feed_url]
  }
  index "idx_inoreader_subscriptions_synced_at" {
    on {
      desc   = true
      column = column.synced_at
    }
  }
  unique "inoreader_subscriptions_inoreader_id_key" {
    columns = [column.inoreader_id]
  }
}

table "inoreader_articles" {
  schema  = schema.public
  comment = "Stores article metadata fetched from Inoreader stream contents API"
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
    comment = "Internal UUID primary key"
  }
  column "inoreader_id" {
    null    = false
    type    = text
    comment = "Unique article identifier from Inoreader API"
  }
  column "subscription_id" {
    null    = true
    type    = uuid
    comment = "Reference to inoreader_subscriptions table"
  }
  column "article_url" {
    null    = false
    type    = text
    comment = "URL to the original article"
  }
  column "title" {
    null    = true
    type    = text
    comment = "Article title from Inoreader"
  }
  column "author" {
    null    = true
    type    = text
    comment = "Article author"
  }
  column "published_at" {
    null    = true
    type    = timestamptz
    comment = "Original publication timestamp"
  }
  column "fetched_at" {
    null    = true
    type    = timestamptz
    default = sql("now()")
    comment = "When this record was fetched from Inoreader"
  }
  column "processed" {
    null    = true
    type    = boolean
    default = false
    comment = "Whether this article has been processed by other services"
  }
  column "content" {
    null    = true
    type    = text
    comment = "Full article content from Inoreader summary.content field"
  }
  column "content_length" {
    null    = true
    type    = integer
    default = 0
    comment = "Length of content in characters for optimization"
  }
  column "content_type" {
    null    = true
    type    = character_varying(50)
    default = "html"
    comment = "Content type (html, html_rtl, text)"
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "inoreader_articles_subscription_id_fkey" {
    columns     = [column.subscription_id]
    ref_columns = [table.inoreader_subscriptions.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_inoreader_articles_inoreader_id" {
    columns = [column.inoreader_id]
  }
  index "idx_inoreader_articles_subscription_id" {
    columns = [column.subscription_id]
  }
  index "idx_inoreader_articles_article_url" {
    columns = [column.article_url]
  }
  index "idx_inoreader_articles_published_at" {
    on {
      desc   = true
      column = column.published_at
    }
  }
  index "idx_inoreader_articles_fetched_at" {
    on {
      desc   = true
      column = column.fetched_at
    }
  }
  index "idx_inoreader_articles_processed" {
    columns = [column.processed]
    where   = "(processed = false)"
  }
  index "idx_inoreader_articles_has_content" {
    columns = [column.content_length]
    where   = "(content_length > 0)"
  }
  index "idx_inoreader_articles_processed_content" {
    columns = [column.processed, column.content_length]
    where   = "(content_length > 0)"
  }
  index "idx_inoreader_articles_content_type" {
    columns = [column.content_type]
    where   = "((content_type IS NOT NULL) AND ((content_type)::text <> 'html'::text))"
  }
  unique "inoreader_articles_inoreader_id_key" {
    columns = [column.inoreader_id]
  }
}

table "sync_state" {
  schema  = schema.public
  comment = "Stores synchronization state and continuation tokens for Inoreader stream pagination"
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
    comment = "Internal UUID primary key"
  }
  column "stream_id" {
    null    = false
    type    = text
    comment = "Stream identifier (e.g., user/-/state/com.google/reading-list)"
  }
  column "continuation_token" {
    null    = true
    type    = text
    comment = "Continuation token for pagination from Inoreader API"
  }
  column "last_sync" {
    null    = true
    type    = timestamptz
    default = sql("now()")
    comment = "Last successful synchronization timestamp"
  }
  column "created_at" {
    null    = true
    type    = timestamptz
    default = sql("now()")
    comment = "Timestamp when the sync state record was first created"
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_sync_state_stream_id" {
    columns = [column.stream_id]
  }
  index "idx_sync_state_last_sync" {
    on {
      desc   = true
      column = column.last_sync
    }
  }
  index "idx_sync_state_created_at" {
    on {
      desc   = true
      column = column.created_at
    }
  }
  unique "sync_state_stream_id_key" {
    columns = [column.stream_id]
  }
}

table "api_usage_tracking" {
  schema  = schema.public
  comment = "Tracks daily API usage for Inoreader rate limit monitoring (Zone 1: 100/day, Zone 2: 100/day)"
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
    comment = "Internal UUID primary key"
  }
  column "date" {
    null    = true
    type    = date
    default = sql("CURRENT_DATE")
    comment = "Date for this usage tracking record (YYYY-MM-DD)"
  }
  column "zone1_requests" {
    null    = true
    type    = integer
    default = 0
    comment = "Number of Zone 1 API requests made (read operations)"
  }
  column "zone2_requests" {
    null    = true
    type    = integer
    default = 0
    comment = "Number of Zone 2 API requests made (write operations)"
  }
  column "last_reset" {
    null    = true
    type    = timestamptz
    default = sql("now()")
    comment = "Last time the counters were reset or updated"
  }
  column "rate_limit_headers" {
    null    = true
    type    = jsonb
    default = "'{}'::jsonb"
    comment = "JSON object storing rate limit headers from Inoreader API responses"
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_api_usage_tracking_date" {
    on {
      desc   = true
      column = column.date
    }
  }
  index "idx_api_usage_tracking_last_reset" {
    on {
      desc   = true
      column = column.last_reset
    }
  }
  unique "uq_api_usage_tracking_date" {
    columns = [column.date]
  }
}

table "summarize_job_queue" {
  schema  = schema.public
  comment = "Queue table for asynchronous article summarization jobs"
  column "id" {
    null = false
    type = serial
    comment = "Internal serial primary key"
  }
  column "job_id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
    comment = "Unique UUID identifier for the job (returned to client)"
  }
  column "article_id" {
    null    = false
    type    = text
    comment = "Article ID (TEXT) to be summarized"
  }
  column "status" {
    null    = false
    type    = character_varying(20)
    default = "pending"
    comment = "Job status: pending, running, completed, failed"
  }
  column "summary" {
    null    = true
    type    = text
    comment = "Generated summary (populated when status is completed)"
  }
  column "error_message" {
    null    = true
    type    = text
    comment = "Error message (populated when status is failed)"
  }
  column "retry_count" {
    null    = false
    type    = integer
    default = 0
    comment = "Number of retry attempts"
  }
  column "max_retries" {
    null    = false
    type    = integer
    default = 3
    comment = "Maximum number of retry attempts allowed"
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
    comment = "Timestamp when job was created"
  }
  column "started_at" {
    null    = true
    type    = timestamptz
    comment = "Timestamp when job processing started"
  }
  column "completed_at" {
    null    = true
    type    = timestamptz
    comment = "Timestamp when job processing completed"
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_summarize_job_queue_status" {
    columns = [column.status]
    where   = "((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('running'::character varying)::text]))"
  }
  index "idx_summarize_job_queue_job_id" {
    columns = [column.job_id]
  }
  index "idx_summarize_job_queue_article_id" {
    columns = [column.article_id]
  }
  unique "summarize_job_queue_job_id_key" {
    columns = [column.job_id]
  }
  check "summarize_job_queue_status_check" {
    expr = "((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('running'::character varying)::text, ('completed'::character varying)::text, ('failed'::character varying)::text]))"
  }
}

schema "public" {
  comment = "standard public schema"
}
