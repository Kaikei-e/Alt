"""Simple LRU cache for embedding reuse."""

from __future__ import annotations

from collections import OrderedDict
from threading import Lock
from typing import Generic, Hashable, Iterator, MutableMapping, TypeVar


K = TypeVar("K", bound=Hashable)
V = TypeVar("V")


class LRUCache(Generic[K, V]):
    """Thread-safe LRU cache with a bounded capacity."""

    def __init__(self, capacity: int) -> None:
        if capacity < 0:
            raise ValueError("capacity must be non-negative")
        self._capacity = capacity
        self._store: MutableMapping[K, V] = OrderedDict()
        self._lock = Lock()

    def __len__(self) -> int:
        with self._lock:
            return len(self._store)

    def get(self, key: K) -> V | None:
        with self._lock:
            if key not in self._store:
                return None
            value = self._store.pop(key)
            self._store[key] = value
            return value

    def set(self, key: K, value: V) -> None:
        if self._capacity == 0:
            return
        with self._lock:
            if key in self._store:
                self._store.pop(key)
            elif len(self._store) >= self._capacity:
                self._store.popitem(last=False)
            self._store[key] = value

    def items(self) -> Iterator[tuple[K, V]]:
        with self._lock:
            return iter(self._store.items())
