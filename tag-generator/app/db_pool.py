"""
Database connection pool for efficient database management.
"""

import logging
import threading
import time
from contextlib import contextmanager
from dataclasses import dataclass
from typing import Optional, List
import queue

import psycopg2
import psycopg2.extensions
from psycopg2.extensions import connection as Connection

logger = logging.getLogger(__name__)


@dataclass
class PoolConfig:
    """Configuration for database connection pool."""

    min_connections: int = 2
    max_connections: int = 10
    connection_timeout: float = 10.0  # Reduced from 30.0 to detect issues faster
    idle_timeout: float = 300.0  # 5 minutes
    max_retries: int = 3
    retry_delay: float = 1.0


class PooledConnection:
    """Wrapper for pooled database connection."""

    def __init__(self, connection: Connection, pool: "ConnectionPool"):
        self.connection = connection
        self.pool = pool
        self.created_at = time.time()
        self.last_used = time.time()
        self.in_use = False
        self._lock = threading.Lock()

    def __enter__(self) -> Connection:
        """Context manager entry."""
        with self._lock:
            self.in_use = True
            self.last_used = time.time()
            return self.connection

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit - return connection to pool."""
        with self._lock:
            self.in_use = False
            self.last_used = time.time()
        self.pool._return_connection(self)

    def is_expired(self, idle_timeout: float) -> bool:
        """Check if connection has been idle too long."""
        with self._lock:
            if self.in_use:
                return False
            return (time.time() - self.last_used) > idle_timeout

    def is_valid(self) -> bool:
        """Check if connection is still valid."""
        try:
            # Check if connection is closed
            if self.connection.closed != 0:
                return False

            # Ensure we're in a clean state before testing
            if self.connection.status != psycopg2.extensions.STATUS_READY:
                try:
                    self.connection.rollback()
                except Exception:
                    return False

            # Simple query to test connection
            with self.connection.cursor() as cursor:
                cursor.execute("SELECT 1")
                cursor.fetchone()  # Actually fetch the result
                return True
        except Exception as e:
            logger.debug(f"Connection validation failed: {e}")
            return False

    def reset_transaction_state(self) -> bool:
        """Reset connection to a clean transaction state."""
        try:
            # Only rollback if we're actually in a transaction
            if self.connection.status != psycopg2.extensions.STATUS_READY:
                self.connection.rollback()
                logger.debug("Rolled back pending transaction")

            # Only set autocommit if we're not already in autocommit mode
            # and connection is in a ready state
            if (
                not self.connection.autocommit
                and self.connection.status == psycopg2.extensions.STATUS_READY
            ):
                self.connection.autocommit = True
                logger.debug("Enabled autocommit mode")

            logger.debug("Connection transaction state reset")
            return True
        except Exception as e:
            logger.warning(f"Failed to reset transaction state: {e}")
            # If reset fails, mark connection as invalid rather than trying aggressive reset
            return False

    def close(self):
        """Close the underlying connection."""
        try:
            self.connection.close()
        except Exception as e:
            logger.warning(f"Error closing connection: {e}")


class ConnectionPool:
    """Thread-safe database connection pool."""

    def __init__(self, dsn: str, config: Optional[PoolConfig] = None):
        self.dsn = dsn
        self.config = config or PoolConfig()
        self._pool: queue.Queue[PooledConnection] = queue.Queue(
            maxsize=self.config.max_connections
        )
        self._all_connections: List[PooledConnection] = []
        self._lock = threading.Lock()
        self._closed = False

        # Initialize minimum connections
        self._initialize_pool()

        # Start cleanup thread
        self._cleanup_thread = threading.Thread(
            target=self._cleanup_expired, daemon=True
        )
        self._cleanup_thread.start()

        logger.info(
            f"Connection pool initialized with {self.config.min_connections}-{self.config.max_connections} connections"
        )

    def _initialize_pool(self):
        """Create initial pool connections."""
        for _ in range(self.config.min_connections):
            try:
                conn = self._create_new_connection()
                if conn:
                    self._pool.put(conn, block=False)
            except Exception as e:
                logger.error(f"Failed to create initial connection: {e}")

    def _create_new_connection(self) -> Optional[PooledConnection]:
        """Create a new database connection."""
        for attempt in range(self.config.max_retries):
            try:
                raw_conn = psycopg2.connect(self.dsn)

                # Ensure the new connection starts with autocommit enabled
                # This should be safe for new connections
                raw_conn.autocommit = True

                pooled_conn = PooledConnection(raw_conn, self)

                with self._lock:
                    self._all_connections.append(pooled_conn)

                logger.debug(f"Created new pooled connection (attempt {attempt + 1})")
                return pooled_conn

            except Exception as e:
                logger.error(f"Connection creation failed (attempt {attempt + 1}): {e}")
                if attempt < self.config.max_retries - 1:
                    time.sleep(self.config.retry_delay)

        return None

    @contextmanager
    def get_connection(self):
        """Get a connection from the pool."""
        if self._closed:
            raise RuntimeError("Connection pool is closed")

        logger.debug("Starting connection acquisition...")
        conn = None
        try:
            # Try to get existing connection with shorter timeout to prevent hanging
            logger.debug("Attempting to get connection from queue...")
            try:
                # Use shorter timeout to fail faster if pool is hung
                queue_timeout = min(self.config.connection_timeout, 10.0)
                conn = self._pool.get(timeout=queue_timeout)
                logger.debug(f"Got connection from queue: {conn}")

                # Validate connection
                logger.debug("Validating connection...")
                if not conn.is_valid():
                    logger.warning("Got invalid connection from pool, creating new one")
                    self._remove_connection(conn)
                    conn = None
                else:
                    logger.debug("Connection validation passed")

            except queue.Empty:
                logger.debug(
                    "No connections available in pool (queue empty or timeout)"
                )

            # Create new connection if needed
            if conn is None:
                logger.debug("Creating new connection...")
                with self._lock:
                    current_count = len(self._all_connections)
                    logger.debug(
                        f"Current connection count: {current_count}/{self.config.max_connections}"
                    )

                    if current_count < self.config.max_connections:
                        conn = self._create_new_connection()
                        logger.debug(f"Created new connection: {conn}")
                    else:
                        logger.warning(
                            f"Connection pool exhausted! {current_count}/{self.config.max_connections}"
                        )
                        # Log pool statistics for debugging
                        try:
                            stats = self.get_stats()
                            logger.warning(f"Pool stats: {stats}")
                        except Exception as stats_error:
                            logger.warning(f"Failed to get pool stats: {stats_error}")
                        raise RuntimeError("Connection pool exhausted")

            if conn is None:
                raise RuntimeError("Failed to obtain database connection")

            logger.debug("Successfully acquired connection, yielding to caller")
            yield conn.connection

        finally:
            logger.debug("Connection context manager exiting")
            if conn:
                # Connection is returned to pool via PooledConnection.__exit__
                logger.debug("Connection will be returned to pool via __exit__")
            else:
                logger.debug("No connection to return to pool")

    def _return_connection(self, conn: PooledConnection):
        """Return a connection to the pool."""
        if self._closed:
            conn.close()
            return

        if conn.is_valid() and not conn.is_expired(self.config.idle_timeout):
            # Reset transaction state before returning to pool
            if conn.reset_transaction_state():
                try:
                    self._pool.put(conn, block=False)
                except queue.Full:
                    # Pool is full, close this connection
                    self._remove_connection(conn)
            else:
                # Failed to reset transaction state, remove connection
                logger.warning(
                    "Failed to reset connection transaction state, removing from pool"
                )
                self._remove_connection(conn)
        else:
            # Connection is invalid or expired
            self._remove_connection(conn)

    def _remove_connection(self, conn: PooledConnection):
        """Remove and close a connection."""
        with self._lock:
            if conn in self._all_connections:
                self._all_connections.remove(conn)
        conn.close()

    def _cleanup_expired(self):
        """Background thread to clean up expired connections."""
        while not self._closed:
            try:
                time.sleep(60)  # Check every minute

                with self._lock:
                    expired = [
                        conn
                        for conn in self._all_connections
                        if conn.is_expired(self.config.idle_timeout) and not conn.in_use
                    ]

                for conn in expired:
                    logger.debug("Removing expired connection")
                    self._remove_connection(conn)

                # Ensure minimum connections
                with self._lock:
                    current_count = len(self._all_connections)
                    if current_count < self.config.min_connections:
                        needed = self.config.min_connections - current_count
                        for _ in range(needed):
                            new_conn = self._create_new_connection()
                            if new_conn:
                                try:
                                    self._pool.put(new_conn, block=False)
                                except queue.Full:
                                    break

            except Exception as e:
                logger.error(f"Error in connection cleanup: {e}")

    def get_stats(self) -> dict:
        """Get pool statistics."""
        with self._lock:
            total_connections = len(self._all_connections)
            active_connections = sum(1 for conn in self._all_connections if conn.in_use)

        return {
            "total_connections": total_connections,
            "active_connections": active_connections,
            "available_connections": self._pool.qsize(),
            "max_connections": self.config.max_connections,
        }

    def close(self):
        """Close all connections and shutdown pool."""
        self._closed = True

        # Close all connections
        with self._lock:
            for conn in self._all_connections[:]:
                conn.close()
            self._all_connections.clear()

        # Clear the queue
        while not self._pool.empty():
            try:
                self._pool.get_nowait()
            except queue.Empty:
                break

        logger.info("Connection pool closed")


# Global pool instance
_pool_instance: Optional[ConnectionPool] = None
_pool_lock = threading.Lock()


def get_connection_pool(
    dsn: str, config: Optional[PoolConfig] = None
) -> ConnectionPool:
    """Get or create the global connection pool."""
    global _pool_instance

    with _pool_lock:
        if _pool_instance is None:
            _pool_instance = ConnectionPool(dsn, config)
        return _pool_instance


def close_connection_pool():
    """Close the global connection pool."""
    global _pool_instance

    with _pool_lock:
        if _pool_instance:
            _pool_instance.close()
            _pool_instance = None
