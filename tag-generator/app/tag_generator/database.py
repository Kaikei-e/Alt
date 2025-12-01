"""Database connection management for tag generator service."""

import os
import time
from contextlib import contextmanager
from typing import TYPE_CHECKING

import psycopg2
import psycopg2.extensions
import structlog
from psycopg2.extensions import connection as Connection

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from tag_generator.config import TagGeneratorConfig


class DatabaseConnectionError(Exception):
    """Custom exception for database connection errors."""

    pass


class DatabaseManager:
    """Manages database connections for the tag generator service."""

    def __init__(self, config: "TagGeneratorConfig"):
        """Initialize database manager with configuration."""
        self.config = config
        self._connection_pool = None

    def get_database_dsn(self) -> str:
        """Build database connection string from environment variables."""
        # Check password (env var or file)
        password = os.getenv("DB_TAG_GENERATOR_PASSWORD")
        if not password:
            password_file = os.getenv("DB_TAG_GENERATOR_PASSWORD_FILE")
            if password_file:
                try:
                    with open(password_file) as f:
                        password = f.read().strip()
                except Exception as e:
                    logger.error(f"Failed to read password file: {e}")

        if not password:
            raise ValueError("Missing DB_TAG_GENERATOR_PASSWORD or DB_TAG_GENERATOR_PASSWORD_FILE")

        required_vars = [
            "DB_TAG_GENERATOR_USER",
            "DB_HOST",
            "DB_PORT",
            "DB_NAME",
        ]

        missing_vars = [var for var in required_vars if not os.getenv(var)]
        if missing_vars:
            raise ValueError(f"Missing required environment variables: {missing_vars}")

        dsn = (
            f"postgresql://{os.getenv('DB_TAG_GENERATOR_USER')}:"
            f"{password}@"
            f"{os.getenv('DB_HOST')}:{os.getenv('DB_PORT')}/"
            f"{os.getenv('DB_NAME')}"
        )

        return dsn

    @contextmanager
    def get_connection(self):
        """Get database connection using direct connection as context manager."""
        conn = None
        try:
            conn = self._create_direct_connection()
            yield conn
        finally:
            if conn:
                try:
                    conn.close()
                except Exception as e:
                    logger.warning(f"Error closing direct connection: {e}")

    def _create_direct_connection(self) -> Connection:
        """Create direct database connection with retry logic."""
        dsn = self.get_database_dsn()

        for attempt in range(self.config.max_connection_retries):
            try:
                logger.info(
                    f"Attempting database connection (attempt {attempt + 1}/{self.config.max_connection_retries})"
                )
                conn = psycopg2.connect(dsn)

                # Ensure connection starts in a clean state
                try:
                    # First, check if we're in a transaction and rollback if needed
                    if conn.status != psycopg2.extensions.STATUS_READY:
                        conn.rollback()

                    # Ensure autocommit is enabled
                    if not conn.autocommit:
                        conn.autocommit = True

                    logger.info("Database connected successfully")
                    return conn
                except Exception as setup_error:
                    logger.warning(f"Failed to setup connection state: {setup_error}")
                    # If we can't set up the connection properly, close it and try again
                    try:
                        conn.close()
                    except Exception:
                        pass
                    raise setup_error

            except psycopg2.Error as e:
                logger.error(f"Database connection failed (attempt {attempt + 1}): {e}")

                if attempt < self.config.max_connection_retries - 1:
                    logger.info(f"Retrying in {self.config.connection_retry_delay} seconds...")
                    time.sleep(self.config.connection_retry_delay)
                else:
                    raise DatabaseConnectionError(
                        f"Failed to connect after {self.config.max_connection_retries} attempts"
                    ) from e
        raise DatabaseConnectionError("Failed to establish database connection after multiple retries")
