import os
import time
from datetime import datetime, UTC

import psycopg2
from psycopg2.extras import DictCursor
from article_fetcher.fetch import fetch_articles
from tag_extractor.extract import extract_tags
from tag_inserter.upsert_tags import upsert_tags

# Processing intervals (similar to pre-processor)
PROCESSING_INTERVAL = 30  # seconds between processing batches
ERROR_RETRY_INTERVAL = 60  # seconds to wait after errors

def main():
    print(f"Hello from tag-generator!")

    db_DSN = f"postgresql://{os.getenv('DB_TAG_GENERATOR_USER')}:{os.getenv('DB_TAG_GENERATOR_PASSWORD')}@{os.getenv('DB_HOST')}:{os.getenv('DB_PORT')}/{os.getenv('DB_NAME')}"

    while True:  # Main service loop
        conn = None
        try:
            conn = psycopg2.connect(db_DSN)

            # 初期カーソル位置：未来から遡るので最大値を設定
            last_created_at = datetime.now(UTC).isoformat()
            last_id = "ffffffff-ffff-ffff-ffff-ffffffffffff"

            processed_count = 0

            # Process articles in batches
            while True:
                rows = fetch_articles(conn, last_created_at, last_id)
                if not rows:
                    if processed_count == 0:
                        print("No new articles found to process")
                    else:
                        print(f"Finished processing batch of {processed_count} articles")
                    break  # Break from batch processing loop, not main loop

                for row in rows:
                    aid, title, content, c_at = row["id"], row["title"], row["content"], row["created_at"]
                    try:
                        tags = extract_tags(title, content)
                        upsert_tags(conn, aid, tags)
                        last_created_at, last_id = c_at, aid
                        processed_count += 1

                        if processed_count % 10 == 0:  # Log progress every 10 articles
                            print(f"Processed {processed_count} articles...")

                    except Exception as e:
                        print(f"Error processing article {aid}: {e}")
                        # Continue with next article instead of failing entirely
                        continue

            print(f"Tag generation batch completed. Sleeping for {PROCESSING_INTERVAL} seconds...")
            time.sleep(PROCESSING_INTERVAL)

        except Exception as e:
            print(f"Database connection or processing error: {e}")
            print(f"Retrying in {ERROR_RETRY_INTERVAL} seconds...")
            time.sleep(ERROR_RETRY_INTERVAL)
        finally:
            if conn:
                conn.close()

if __name__ == "__main__":
    main()