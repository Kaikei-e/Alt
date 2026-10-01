"""Text chunking for TTS synthesis."""

import re


def split_into_chunks(text: str, max_chars: int) -> list[str]:
    """Split text into chunks satisfying max_chars constraints."""
    if not text.strip():
        return []

    raw_sentences: list[str] = []
    for line in text.splitlines():
        line = line.strip()
        if not line:
            continue
        parts = re.split(r"(?<=[。！？!?])(?![。！？!?])", line)
        for part in parts:
            part = part.strip()
            if part:
                raw_sentences.append(part)

    atomic_pieces: list[str] = []
    for sentence in raw_sentences:
        if len(sentence) <= max_chars:
            atomic_pieces.append(sentence)
            continue
        clauses = re.split(r"(?<=、)|(?<=,(?!\d))", sentence)
        for clause in clauses:
            clause = clause.strip()
            if not clause:
                continue
            if len(clause) <= max_chars:
                atomic_pieces.append(clause)
            else:
                for i in range(0, len(clause), max_chars):
                    frag = clause[i : i + max_chars].strip()
                    if frag:
                        atomic_pieces.append(frag)

    atomic_pieces = [p for p in atomic_pieces if any(c.isalnum() for c in p)]

    chunks: list[str] = []
    current_chunk = ""

    for piece in atomic_pieces:
        if not current_chunk:
            current_chunk = piece
            continue
        sep = (
            " "
            if (
                current_chunk[-1].isascii()
                and not current_chunk[-1].isspace()
                and piece[0].isascii()
                and not piece[0].isspace()
            )
            else ""
        )
        if len(current_chunk) + len(sep) + len(piece) <= max_chars:
            current_chunk = f"{current_chunk}{sep}{piece}"
        else:
            chunks.append(current_chunk)
            current_chunk = piece

    if current_chunk:
        chunks.append(current_chunk)

    return chunks
