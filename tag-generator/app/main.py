import os
import time
import gc
from datetime import datetime, UTC

import psycopg2
from psycopg2.extras import DictCursor
from article_fetcher.fetch import fetch_articles
from tag_extractor.extract import extract_tags
from tag_inserter.upsert_tags import upsert_tags

# Processing intervals (similar to pre-processor)
PROCESSING_INTERVAL = 60  # seconds between processing batches (increased for efficiency)
ERROR_RETRY_INTERVAL = 60  # seconds to wait after errors
BATCH_LIMIT = 100  # Process more articles per cycle to be more efficient

def main():
    print(f"Hello from tag-generator!")
    print("Starting tag generation service...")

    db_DSN = f"postgresql://{os.getenv('DB_TAG_GENERATOR_USER')}:{os.getenv('DB_TAG_GENERATOR_PASSWORD')}@{os.getenv('DB_HOST')}:{os.getenv('DB_PORT')}/{os.getenv('DB_NAME')}"
    print("Database connection string prepared")

    while True:  # Main service loop
        conn = None
        try:
            print("Attempting database connection...")
            conn = psycopg2.connect(db_DSN)
            print("Database connected successfully")

            # 初期カーソル位置：未来から遡るので最大値を設定
            last_created_at = datetime.now(UTC).isoformat()
            last_id = "ffffffff-ffff-ffff-ffff-ffffffffffff"

            processed_count = 0
            total_articles_this_cycle = 0
            print(f"Starting article processing from {last_created_at}")

            # Process articles in batches with limit per cycle
            while total_articles_this_cycle < BATCH_LIMIT:
                print("Fetching articles from database...")
                rows = fetch_articles(conn, last_created_at, last_id)
                print(f"Fetched {len(rows)} articles")

                if not rows:
                    if processed_count == 0:
                        print("No new articles found to process")
                    else:
                        print(f"Finished processing batch of {processed_count} articles")
                    break  # Break from batch processing loop, not main loop

                for row in rows:
                    aid, title, content, c_at = row["id"], row["title"], row["content"], row["created_at"]
                    try:
                        print(f"Processing article {aid}...")
                        tags = extract_tags(title, content)
                        print(f"Extracted {len(tags)} tags: {tags}")
                        upsert_tags(conn, aid, tags)
                        last_created_at, last_id = c_at, aid
                        processed_count += 1
                        total_articles_this_cycle += 1

                        if processed_count % 10 == 0:  # Log progress every 10 articles
                            print(f"Processed {processed_count} articles...")
                            # Force garbage collection to prevent memory accumulation
                            gc.collect()

                        # Stop if we've hit our batch limit
                        if total_articles_this_cycle >= BATCH_LIMIT:
                            print(f"Reached batch limit of {BATCH_LIMIT} articles for this cycle")
                            break

                    except Exception as e:
                        print(f"Error processing article {aid}: {e}")
                        # Continue with next article instead of failing entirely
                        continue

                # Break outer loop if we hit batch limit
                if total_articles_this_cycle >= BATCH_LIMIT:
                    break

            print(f"Tag generation batch completed. Processed {total_articles_this_cycle} articles. Sleeping for {PROCESSING_INTERVAL} seconds...")

            # Force garbage collection before sleeping
            gc.collect()
            time.sleep(PROCESSING_INTERVAL)

        except Exception as e:
            print(f"Database connection or processing error: {e}")
            print(f"Retrying in {ERROR_RETRY_INTERVAL} seconds...")
            time.sleep(ERROR_RETRY_INTERVAL)
        finally:
            if conn:
                print("Closing database connection")
                conn.close()
            # Final garbage collection
            gc.collect()

if __name__ == "__main__":
    main()