import asyncio
import logging
import sys
from pathlib import Path
from sqlalchemy.ext.asyncio import create_async_engine
from sqlalchemy import text, inspect

# Ensure we can import from recap-subworker
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.infra.config import Settings

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

def get_db_url_with_password(settings: Settings) -> str:
    db_url = settings.db_url
    if "recap-db" in db_url:
        db_url = db_url.replace("recap-db", "localhost").replace("5432", "5435")

    from urllib.parse import urlparse, urlunparse
    secret_path = project_root.parent / "secrets" / "recap_db_password.txt"

    if secret_path.exists():
        try:
            with open(secret_path, "r") as f:
                password = f.read().strip()
            u = urlparse(db_url)
            if '@' in u.netloc:
                user_pass, host_port = u.netloc.rsplit('@', 1)
                if ':' in user_pass:
                    user, _ = user_pass.split(':', 1)
                    new_user_pass = f"{user}:{password}"
                else:
                    new_user_pass = f"{user_pass}:{password}"
                new_netloc = f"{new_user_pass}@{host_port}"
                db_url = urlunparse((u.scheme, new_netloc, u.path, u.params, u.query, u.fragment))
        except Exception as e:
            logger.warning(f"Failed to read password secret: {e}")
    return db_url

async def main():
    settings = Settings()
    db_url = get_db_url_with_password(settings)
    engine = create_async_engine(db_url)

    async with engine.connect() as conn:
        logger.info("Inspecting columns for 'recap_job_articles'...")
        # Note: inspect is synchronous, need run_sync
        def get_columns(connection):
            inspector = inspect(connection)
            return inspector.get_columns("recap_job_articles")

        try:
            columns = await conn.run_sync(get_columns)
            for col in columns:
                print(f"Column: {col['name']} - Type: {col['type']}")
        except Exception as e:
            logger.error(f"Error inspecting table: {e}")

    await engine.dispose()

if __name__ == "__main__":
    asyncio.run(main())
