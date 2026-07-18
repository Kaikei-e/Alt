"""Boundary-buffering filter for LLM streaming control tokens."""

from __future__ import annotations


class ControlTokenFilter:
    """Filter control tokens that may be split across stream chunks.

    Exact-match filtering alone leaks split tokens (e.g. ``"<|tu"`` + ``"rn>"``).
    This filter buffers while the accumulated suffix is a prefix of any ignored
    token, and only emits text once it cannot complete a control token.
    """

    def __init__(self, ignored_tokens: set[str] | None = None) -> None:
        self._ignored = ignored_tokens or {
            "<start_of_turn>",
            "<end_of_turn>",
            "<|turn>",
            "<turn|>",
            "<|channel>thought",
            "<channel|>",
            "<|system|>",
            "<|user|>",
            "<|assistant|>",
        }
        self._buf = ""
        self._max_len = max((len(t) for t in self._ignored), default=0)

    def push(self, chunk: str) -> str:
        """Ingest a chunk; return emit-safe text (may be empty while buffering)."""
        if not chunk:
            return ""
        self._buf += chunk
        return self._emit_safe_prefix()

    def flush(self) -> str:
        """Release any remaining buffer (end of stream)."""
        out = self._buf
        self._buf = ""
        if out in self._ignored:
            return ""
        return out

    def _emit_safe_prefix(self) -> str:
        """Emit characters that cannot be part of a control-token prefix."""
        emitted: list[str] = []
        while self._buf:
            if self._buf in self._ignored:
                self._buf = ""
                break

            # Longest ignored-token prefix at start of buffer?
            if any(t.startswith(self._buf) for t in self._ignored):
                # Entire buffer is a proper prefix — keep buffering
                break

            # If buffer starts with a prefix of an ignored token, keep that prefix
            # and emit only the non-prefix leading part... but we always append at
            # end, so the ambiguous region is a suffix of _buf.
            # Find longest suffix that is a prefix of some ignored token.
            keep_from = len(self._buf)
            for i in range(len(self._buf)):
                suffix = self._buf[i:]
                if any(t.startswith(suffix) for t in self._ignored):
                    keep_from = i
                    break
            else:
                # No suffix is a token prefix — emit everything
                emitted.append(self._buf)
                self._buf = ""
                break

            if keep_from == 0:
                # Whole buffer is a prefix — wait for more
                break

            emitted.append(self._buf[:keep_from])
            self._buf = self._buf[keep_from:]

            # After emitting, check if remaining buffer is exactly an ignored token
            if self._buf in self._ignored:
                self._buf = ""

        return "".join(emitted)
