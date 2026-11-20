import asyncio
import logging
import sys
from pathlib import Path
from urllib.parse import urlparse, urlunparse

from sqlalchemy import Connection, inspect
from sqlalchemy.engine.interfaces import ReflectedColumn
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import create_async_engine

# Ensure we can import from recap-subworker
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.infra.config import Settings  # noqa: E402  # import after sys.path setup

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def get_db_url_with_password(settings: Settings) -> str:
    db_url: str = settings.db_url_str
    if "recap-db" in db_url:
        db_url = db_url.replace("recap-db", "localhost").replace("5432", "5435")

    secret_path = project_root.parent / "secrets" / "recap_db_password.txt"

    if secret_path.exists():
        try:
            with secret_path.open() as f:
                password = f.read().strip()
            u = urlparse(db_url)
            if "@" in u.netloc:
                user_pass, host_port = u.netloc.rsplit("@", 1)
                if ":" in user_pass:
                    user, _ = user_pass.split(":", 1)
                    new_user_pass = f"{user}:{password}"
                else:
                    new_user_pass = f"{user_pass}:{password}"
                new_netloc = f"{new_user_pass}@{host_port}"
                db_url = urlunparse((u.scheme, new_netloc, u.path, u.params, u.query, u.fragment))
        except Exception as e:  # noqa: BLE001  # secret file read/URL parse is best-effort in local helper
            logger.warning("Failed to read password secret: %s", e)
    return db_url


async def main() -> None:
    settings = Settings()
    db_url = get_db_url_with_password(settings)
    engine = create_async_engine(db_url)

    async with engine.connect() as conn:
        logger.info("Inspecting columns for 'recap_job_articles'...")

        # Note: inspect is synchronous, need run_sync
        def get_columns(connection: Connection) -> list[ReflectedColumn]:
            inspector = inspect(connection)
            return inspector.get_columns("recap_job_articles")

        try:
            columns = await conn.run_sync(get_columns)
            for col in columns:
                sys.stdout.write(f"Column: {col['name']} - Type: {col['type']}\n")
        except SQLAlchemyError, OSError:
            logger.exception("Error inspecting table")

    await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
