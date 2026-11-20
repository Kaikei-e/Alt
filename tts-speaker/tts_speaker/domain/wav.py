"""WAV audio operations."""

import io
import wave

from tts_speaker.domain.errors import AudioFormatError

_MONO_CHANNELS = 1
_PCM_16BIT_SAMPWIDTH = 2


def wav_info(wav: bytes) -> tuple[int, float]:
    """Validate 16-bit mono PCM WAV bytes and return (sample_rate, duration_seconds)."""
    try:
        with wave.open(io.BytesIO(wav), "rb") as wf:
            nchannels = wf.getnchannels()
            sampwidth = wf.getsampwidth()
            framerate = wf.getframerate()
            comptype = wf.getcomptype()
            nframes = wf.getnframes()
            frames = wf.readframes(nframes)
    except (wave.Error, EOFError) as e:
        msg = f"Corrupt WAV data: {e}"
        raise AudioFormatError(msg) from e

    if nchannels != _MONO_CHANNELS:
        msg = f"Expected mono (1 channel), got {nchannels}"
        raise AudioFormatError(msg)
    if sampwidth != _PCM_16BIT_SAMPWIDTH:
        msg = f"Expected 16-bit PCM (sampwidth == 2), got {sampwidth}"
        raise AudioFormatError(msg)
    if comptype != "NONE":
        msg = f"Expected uncompressed PCM, got {comptype}"
        raise AudioFormatError(msg)
    if framerate <= 0:
        msg = "Invalid framerate in WAV"
        raise AudioFormatError(msg)
    if nframes <= 0:
        msg = "WAV contains no audio frames"
        raise AudioFormatError(msg)
    if len(frames) != nframes * nchannels * sampwidth:
        msg = "WAV data size does not match frame count"
        raise AudioFormatError(msg)

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
            nframes = wf.getnframes()
            frames = wf.readframes(nframes)
    except (wave.Error, EOFError) as e:
        msg = f"Corrupt WAV segment: {e}"
        raise AudioFormatError(msg) from e

    if sampwidth != _PCM_16BIT_SAMPWIDTH:
        msg = f"Expected sampwidth == 2, got {sampwidth}"
        raise AudioFormatError(msg)
    if nframes <= 0:
        msg = "WAV contains no audio frames"
        raise AudioFormatError(msg)
    if len(frames) != nframes * nchannels * sampwidth:
        msg = "WAV data size does not match frame count"
        raise AudioFormatError(msg)

    gap_frames = 0 if ms <= 0 else int(framerate * (ms / 1000.0))

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
        msg = f"Failed to write WAV with trailing silence: {e}"
        raise AudioFormatError(msg) from e

    return out_buf.getvalue()
