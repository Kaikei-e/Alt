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
from psycopg2.extensions import connection as Connection

logger = logging.getLogger(__name__)


@dataclass
class PoolConfig:
    """Configuration for database connection pool."""
    min_connections: int = 2
    max_connections: int = 10
    connection_timeout: float = 30.0
    idle_timeout: float = 300.0  # 5 minutes
    max_retries: int = 3
    retry_delay: float = 1.0


class PooledConnection:
    """Wrapper for pooled database connection."""
    
    def __init__(self, connection: Connection, pool: 'ConnectionPool'):
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
            with self.connection.cursor() as cursor:
                cursor.execute("SELECT 1")
                return True
        except Exception:
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
        self._pool: queue.Queue[PooledConnection] = queue.Queue(maxsize=self.config.max_connections)
        self._all_connections: List[PooledConnection] = []
        self._lock = threading.Lock()
        self._closed = False
        
        # Initialize minimum connections
        self._initialize_pool()
        
        # Start cleanup thread
        self._cleanup_thread = threading.Thread(target=self._cleanup_expired, daemon=True)
        self._cleanup_thread.start()
        
        logger.info(f"Connection pool initialized with {self.config.min_connections}-{self.config.max_connections} connections")
    
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
        
        conn = None
        try:
            # Try to get existing connection
            try:
                conn = self._pool.get(timeout=self.config.connection_timeout)
                
                # Validate connection
                if not conn.is_valid():
                    logger.warning("Got invalid connection from pool, creating new one")
                    self._remove_connection(conn)
                    conn = None
                    
            except queue.Empty:
                logger.debug("No connections available in pool")
            
            # Create new connection if needed
            if conn is None:
                with self._lock:
                    if len(self._all_connections) < self.config.max_connections:
                        conn = self._create_new_connection()
                    else:
                        raise RuntimeError("Connection pool exhausted")
            
            if conn is None:
                raise RuntimeError("Failed to obtain database connection")
            
            yield conn.connection
            
        finally:
            if conn:
                # Connection is returned to pool via PooledConnection.__exit__
                pass
    
    def _return_connection(self, conn: PooledConnection):
        """Return a connection to the pool."""
        if self._closed:
            conn.close()
            return
        
        if conn.is_valid() and not conn.is_expired(self.config.idle_timeout):
            try:
                self._pool.put(conn, block=False)
            except queue.Full:
                # Pool is full, close this connection
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
                    expired = [conn for conn in self._all_connections 
                             if conn.is_expired(self.config.idle_timeout) and not conn.in_use]
                
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
            "max_connections": self.config.max_connections
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


def get_connection_pool(dsn: str, config: Optional[PoolConfig] = None) -> ConnectionPool:
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