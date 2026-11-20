"""Text chunking for TTS synthesis."""

import re


def _split_into_sentences(text: str) -> list[str]:
    sentences: list[str] = []
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        for raw_part in re.split(r"(?<=[。！？!?])(?![。！？!?])", line):
            part = raw_part.strip()
            if part:
                sentences.append(part)
    return sentences


def _split_into_atomic_pieces(sentences: list[str], max_chars: int) -> list[str]:
    atomic_pieces: list[str] = []
    for sentence in sentences:
        if len(sentence) <= max_chars:
            atomic_pieces.append(sentence)
            continue
        for raw_clause in re.split(r"(?<=、)|(?<=,(?!\d))", sentence):
            clause = raw_clause.strip()
            if not clause:
                continue
            if len(clause) <= max_chars:
                atomic_pieces.append(clause)
            else:
                for i in range(0, len(clause), max_chars):
                    frag = clause[i : i + max_chars].strip()
                    if frag:
                        atomic_pieces.append(frag)
    return [p for p in atomic_pieces if any(c.isalnum() for c in p)]


def _pack_pieces(pieces: list[str], max_chars: int) -> list[str]:
    chunks: list[str] = []
    current_chunk = ""
    for piece in pieces:
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


def split_into_chunks(text: str, max_chars: int) -> list[str]:
    """Split text into chunks satisfying max_chars constraints."""
    if not text.strip():
        return []
    sentences = _split_into_sentences(text)
    atomic_pieces = _split_into_atomic_pieces(sentences, max_chars)
    return _pack_pieces(atomic_pieces, max_chars)
