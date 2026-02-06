"""Gateway: PostgreSQL implementation of CursorStorePort.

Delegates to the existing CursorManager for backward compatibility.
"""

from tag_generator.cursor_manager import CursorManager

# The existing CursorManager already satisfies CursorStorePort structurally.
# This module re-exports it under a Clean Architecture-aligned name.
PostgresCursorStore = CursorManager
