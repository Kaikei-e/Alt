"""WAV audio operations."""

import io
import wave

from tts_speaker.domain.errors import AudioFormatError


def wav_info(wav: bytes) -> tuple[int, float]:
    """Validate 16-bit mono PCM WAV bytes and return (sample_rate, duration_seconds)."""
    try:
        with wave.open(io.BytesIO(wav), "rb") as wf:
            nchannels = wf.getnchannels()
            sampwidth = wf.getsampwidth()
            framerate = wf.getframerate()
            comptype = wf.getcomptype()
            nframes = wf.getnframes()
    except (wave.Error, EOFError) as e:
        raise AudioFormatError(f"Corrupt WAV data: {e}") from e

    if nchannels != 1:
        raise AudioFormatError(f"Expected mono (1 channel), got {nchannels}")
    if sampwidth != 2:
        raise AudioFormatError(f"Expected 16-bit PCM (sampwidth == 2), got {sampwidth}")
    if comptype != "NONE":
        raise AudioFormatError(f"Expected uncompressed PCM, got {comptype}")
    if framerate <= 0:
        raise AudioFormatError("Invalid framerate in WAV")

    duration = nframes / float(framerate)
    return framerate, duration


def append_silence(wav: bytes, ms: int) -> bytes:
    """Append trailing silence to a 16-bit PCM WAV audio segment."""
    try:
        with wave.open(io.BytesIO(wav), "rb") as wf:
            nchannels = wf.getnchannels()
            sampwidth = wf.getsampwidth()
            framerate = wf.getframerate()
            comptype = wf.getcomptype()
            frames = wf.readframes(wf.getnframes())
    except (wave.Error, EOFError) as e:
        raise AudioFormatError(f"Corrupt WAV segment: {e}") from e

    if sampwidth != 2:
        raise AudioFormatError(f"Expected sampwidth == 2, got {sampwidth}")

    if ms <= 0:
        gap_frames = 0
    else:
        gap_frames = int(framerate * (ms / 1000.0))

    gap_bytes = b"\x00" * (gap_frames * nchannels * sampwidth)

    out_buf = io.BytesIO()
    try:
        with wave.open(out_buf, "wb") as out_wf:
            out_wf.setnchannels(nchannels)
            out_wf.setsampwidth(sampwidth)
            out_wf.setframerate(framerate)
            out_wf.setcomptype(comptype, "not compressed")
            out_wf.writeframes(frames)
            if gap_bytes:
                out_wf.writeframes(gap_bytes)
    except (wave.Error, EOFError) as e:
        raise AudioFormatError(f"Failed to write WAV with trailing silence: {e}") from e

    return out_buf.getvalue()
