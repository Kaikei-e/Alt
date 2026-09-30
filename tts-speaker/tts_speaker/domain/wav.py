"""WAV audio operations."""

import io
import wave

from tts_speaker.domain.errors import AudioFormatError


def concat_wav(parts: list[bytes], gap_ms: int) -> bytes:
    """Concatenate multiple WAV audio byte segments with a silence gap."""
    if not parts:
        raise AudioFormatError("Empty parts list cannot be concatenated")

    try:
        with wave.open(io.BytesIO(parts[0]), "rb") as wf:
            base_params = (
                wf.getnchannels(),
                wf.getsampwidth(),
                wf.getframerate(),
                wf.getcomptype(),
            )
            first_frames = wf.readframes(wf.getnframes())
    except (wave.Error, EOFError) as e:
        raise AudioFormatError(f"Corrupt WAV segment: {e}") from e

    if base_params[1] != 2:
        raise AudioFormatError(f"Expected sampwidth == 2, got {base_params[1]}")

    audio_frames: list[bytes] = [first_frames]

    for i, part in enumerate(parts[1:], start=1):
        try:
            with wave.open(io.BytesIO(part), "rb") as wf:
                params = (
                    wf.getnchannels(),
                    wf.getsampwidth(),
                    wf.getframerate(),
                    wf.getcomptype(),
                )
                frames = wf.readframes(wf.getnframes())
        except (wave.Error, EOFError) as e:
            raise AudioFormatError(f"Corrupt WAV segment: {e}") from e

        if params != base_params:
            raise AudioFormatError(f"WAV parameter mismatch: part 0 had {base_params}, part {i} had {params}")

        audio_frames.append(frames)

    nchannels, sampwidth, framerate, comptype = base_params

    gap_frames = int(framerate * (gap_ms / 1000.0))
    gap_bytes = b"\x00" * (gap_frames * nchannels * sampwidth)

    out_buf = io.BytesIO()
    try:
        with wave.open(out_buf, "wb") as out_wf:
            out_wf.setnchannels(nchannels)
            out_wf.setsampwidth(sampwidth)
            out_wf.setframerate(framerate)
            out_wf.setcomptype(comptype, "not compressed")

            for i, frames in enumerate(audio_frames):
                if i > 0 and gap_bytes:
                    out_wf.writeframes(gap_bytes)
                out_wf.writeframes(frames)
    except (wave.Error, EOFError) as e:
        raise AudioFormatError(f"Failed to write concatenated WAV: {e}") from e

    return out_buf.getvalue()


def wav_duration_seconds(data: bytes) -> float:
    """Compute the duration of a WAV byte stream in seconds."""
    try:
        with wave.open(io.BytesIO(data), "rb") as wf:
            framerate = wf.getframerate()
            nframes = wf.getnframes()
    except (wave.Error, EOFError) as e:
        raise AudioFormatError(f"Corrupt WAV data: {e}") from e

    if framerate <= 0:
        raise AudioFormatError("Invalid framerate in WAV")
    return nframes / float(framerate)
