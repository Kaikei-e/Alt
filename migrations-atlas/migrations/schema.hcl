table "article_summaries" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "article_id" {
    null = false
    type = uuid
  }
  column "summary" {
    null = false
    type = text
  }
  column "created_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "summary_japanese" {
    null = true
    type = text
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "article_summaries_article_id_fkey" {
    columns     = [column.article_id]
    ref_columns = [table.articles.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_article_summaries_article_id" {
    columns = [column.article_id]
  }
  index "idx_article_summaries_summary_gin_trgm" {
    type = GIN
    on {
      column = column.summary
      ops    = gin_trgm_ops
    }
  }
  index "idx_article_summaries_summary_japanese_gin_trgm" {
    type = GIN
    on {
      column = column.summary_japanese
      ops    = gin_trgm_ops
    }
  }
  index "idx_article_summaries_created_at_id_desc" {
    on {
      desc   = true
      column = column.created_at
    }
    on {
      desc   = true
      column = column.id
    }
  }
}
table "article_tags" {
  schema = schema.public
  column "article_id" {
    null = false
    type = uuid
  }
  column "feed_tag_id" {
    null = false
    type = uuid
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("CURRENT_TIMESTAMP")
  }
  column "tag_type" {
    null    = true
    type    = character_varying(50)
    default = "auto"
  }
  primary_key {
    columns = [column.article_id, column.feed_tag_id]
  }
  foreign_key "article_tags_article_id_fkey" {
    columns     = [column.article_id]
    ref_columns = [table.articles.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  foreign_key "article_tags_feed_tag_id_fkey" {
    columns     = [column.feed_tag_id]
    ref_columns = [table.feed_tags.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_article_tags_created_at" {
    columns = [column.created_at]
  }
  index "idx_article_tags_feed_tag_id" {
    columns = [column.feed_tag_id]
  }
}
table "articles" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "title" {
    null = false
    type = text
  }
  column "content" {
    null = false
    type = text
  }
  column "url" {
    null = false
    type = text
  }
  column "created_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "feed_id" {
    null = true
    type = uuid
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_articles_feed_id" {
    columns     = [column.feed_id]
    ref_columns = [table.feeds.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_articles_created_at_covering" {
    include = [column.id, column.title, column.content, column.url]
    on {
      desc   = true
      column = column.created_at
    }
  }
  index "idx_articles_created_id_desc" {
    on {
      desc   = true
      column = column.created_at
    }
    on {
      desc   = true
      column = column.id
    }
  }
  index "idx_articles_feed_id" {
    columns = [column.feed_id]
  }
  index "idx_articles_id" {
    columns = [column.id]
  }
  index "idx_articles_title" {
    columns = [column.title]
  }
  index "idx_articles_title_created_at" {
    columns = [column.title, column.created_at]
  }
  index "idx_articles_title_gin_trgm_optimized" {
    type  = GIN
    where = "((title IS NOT NULL) AND (length(title) > 0))"
    on {
      column = column.title
      ops    = gin_trgm_ops
    }
  }
  index "idx_articles_url" {
    unique  = true
    columns = [column.url]
  }
  index "idx_articles_url_gin_trgm" {
    type = GIN
    on {
      column = column.url
      ops    = gin_trgm_ops
    }
  }
  unique "articles_url_key" {
    columns = [column.url]
  }
}
table "favorite_feeds" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "user_id" {
    null = false
    type = uuid
  }
  column "feed_id" {
    null = false
    type = uuid
  }
  column "created_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "favorite_feeds_feed_id_fkey" {
    columns     = [column.feed_id]
    ref_columns = [table.feeds.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_favorite_feeds_feed_id" {
    columns = [column.feed_id]
  }
  index "idx_favorite_feeds_user_id" {
    columns = [column.user_id]
  }
  unique "favorite_feeds_user_id_feed_id_key" {
    columns = [column.user_id, column.feed_id]
  }
}
table "feed_links" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "url" {
    null = false
    type = text
  }
  primary_key {
    columns = [column.id]
  }
  unique "feed_links_url_key" {
    columns = [column.url]
  }
  unique "idx_feed_links_id_url" {
    columns = [column.id, column.url]
  }
}
table "feed_tags" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "feed_id" {
    null = false
    type = uuid
  }
  column "tag_name" {
    null = false
    type = text
  }
  column "confidence" {
    null    = false
    type    = double_precision
    default = 0
  }
  column "created_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "tag_type" {
    null    = true
    type    = character_varying(50)
    default = "auto"
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "feed_tags_feed_id_fkey" {
    columns     = [column.feed_id]
    ref_columns = [table.feeds.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_feed_tags_confidence" {
    on {
      desc   = true
      column = column.confidence
    }
  }
  index "idx_feed_tags_feed_id" {
    columns = [column.feed_id]
  }
  index "idx_feed_tags_tag_name" {
    columns = [column.tag_name]
  }
}
table "feeds" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "title" {
    null = false
    type = text
  }
  column "description" {
    null = false
    type = text
  }
  column "link" {
    null = false
    type = text
  }
  column "pub_date" {
    null = false
    type = timestamp
  }
  column "created_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_feeds_created_at" {
    columns = [column.created_at]
  }
  index "idx_feeds_created_at_link" {
    columns = [column.created_at, column.link]
  }
  index "idx_feeds_created_desc_not_mp3_covering" {
    where   = "(link !~ '\\.mp3$'::text)"
    include = [column.id, column.title, column.description, column.link]
    on {
      desc   = true
      column = column.created_at
    }
  }
  index "idx_feeds_id_link" {
    columns = [column.id, column.link]
  }
  index "idx_feeds_link_created_desc" {
    on {
      column = column.link
    }
    on {
      desc   = true
      column = column.created_at
    }
  }
  index "idx_feeds_link_gin_trgm" {
    type = GIN
    on {
      column = column.link
      ops    = gin_trgm_ops
    }
  }
  unique "unique_feeds_link" {
    columns = [column.link]
  }
}
table "read_status" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "feed_id" {
    null = false
    type = uuid
  }
  column "user_id" {
    null    = false
    type    = uuid
    comment = "User ID for read status - uses dummy ID for single-user mode compatibility"
  }
  column "is_read" {
    null    = false
    type    = boolean
    default = false
  }
  column "created_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null    = false
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
  }
  column "read_at" {
    null    = true
    type    = timestamp
    default = sql("CURRENT_TIMESTAMP")
    comment = "Timestamp when the feed was marked as read"
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "read_status_feed_id_fkey" {
    columns     = [column.feed_id]
    ref_columns = [table.feeds.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_read_status_feed_user" {
    columns = [column.feed_id, column.user_id]
  }
  index "idx_read_status_unread" {
    columns = [column.feed_id, column.user_id]
    where   = "(is_read = false)"
  }
  unique "unique_read_status_feed_user" {
    columns = [column.feed_id, column.user_id]
  }
}
table "schema_migrations" {
  schema = schema.public
  column "version" {
    null = false
    type = bigint
  }
  column "dirty" {
    null = false
    type = boolean
  }
  primary_key {
    columns = [column.version]
  }
}
table "scraping_domains" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "domain" {
    null    = false
    type    = text
    comment = "Domain name (e.g., example.com)"
  }
  column "scheme" {
    null    = false
    type    = text
    default = "https"
    comment = "Protocol scheme (http or https)"
  }
  column "allow_fetch_body" {
    null    = false
    type    = boolean
    default = true
    comment = "Whether to allow fetching article bodies"
  }
  column "allow_ml_training" {
    null    = false
    type    = boolean
    default = true
    comment = "Whether to allow using content for ML training/summarization"
  }
  column "allow_cache_days" {
    null    = false
    type    = integer
    default = 7
    comment = "Days to keep article body in cache"
  }
  column "force_respect_robots" {
    null    = false
    type    = boolean
    default = true
    comment = "Whether to strictly respect robots.txt"
  }
  column "robots_txt_url" {
    null = true
    type = text
  }
  column "robots_txt_content" {
    null = true
    type = text
  }
  column "robots_txt_fetched_at" {
    null = true
    type = timestamptz
  }
  column "robots_txt_last_status" {
    null = true
    type = integer
  }
  column "robots_crawl_delay_sec" {
    null = true
    type = integer
  }
  column "robots_disallow_paths" {
    null = true
    type = jsonb
    default = "[]"
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
    columns = [column.id]
  }
  unique "scraping_domains_domain_key" {
    columns = [column.domain]
  }
}
schema "public" {
  comment = "standard public schema"
}
