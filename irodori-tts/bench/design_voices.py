"""Generate candidate voice samples using Irodori-TTS VoiceDesign mode."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from bench_vram_rtf import default_http_request, get_wav_duration_seconds


def parse_seeds(seeds_str: str) -> list[int]:
    """Parse a seed specification string into a sorted list of unique integers.

    Supported patterns:
        - Single integer: "5"
        - Comma-separated list: "1,5,9"
        - Range: "1-20"
        - Mixed: "1-3,5,8-10"
    """
    if not seeds_str or not seeds_str.strip():
        raise ValueError("Seeds specification string cannot be empty")

    seeds: set[int] = set()
    chunks = [c.strip() for c in seeds_str.split(",") if c.strip()]
    if not chunks:
        raise ValueError(f"No valid seed entries found in: '{seeds_str}'")

    for chunk in chunks:
        if "-" in chunk:
            parts = chunk.split("-")
            if len(parts) != 2:
                raise ValueError(f"Invalid range specification: '{chunk}'")
            try:
                start = int(parts[0].strip())
                end = int(parts[1].strip())
            except ValueError as e:
                raise ValueError(f"Range boundaries must be integers: '{chunk}'") from e
            if start > end:
                raise ValueError(
                    "Range start cannot be greater than end:"
                    f" '{chunk}' ({start} > {end})"
                )
            seeds.update(range(start, end + 1))
        else:
            try:
                val = int(chunk)
            except ValueError as e:
                raise ValueError(f"Seed must be an integer: '{chunk}'") from e
            seeds.add(val)

    return sorted(seeds)


def generate_voice_design_sample(
    base_url: str,
    api_key: str,
    text: str,
    caption: str,
    seed: int,
    http_fn=None,
    timeout: float = 120.0,
) -> tuple[int, bytes, dict[str, str]]:
    """Request a voice design sample from POST /v1/audio/speech."""
    fn = http_fn or default_http_request
    url = f"{base_url.rstrip('/')}/v1/audio/speech"
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    payload = {
        "model": "irodori-tts",
        "input": text,
        "voice": "none",
        "response_format": "wav",
        "irodori": {
            "caption": caption,
            "seed": seed,
            "chunking_enabled": False,
        },
    }
    data = json.dumps(payload).encode("utf-8")
    status, body, resp_headers = fn(
        url, method="POST", headers=headers, data=data, timeout=timeout
    )
    return status, body, resp_headers


def run_design_voices(
    base_url: str,
    api_key: str,
    caption: str,
    seeds: list[int],
    text: str,
    out_dir: Path,
    http_fn=None,
) -> list[dict]:
    """Generate samples for all seeds and write manifest.json incrementally."""
    out_dir.mkdir(parents=True, exist_ok=True)
    manifest_file = out_dir / "manifest.json"
    manifest: list[dict] = []
    # Initialize / truncate manifest before starting run
    manifest_file.write_text(
        json.dumps(manifest, indent=2, ensure_ascii=False), encoding="utf-8"
    )

    for seed in seeds:
        filename = f"seed_{seed}.wav"
        filepath = out_dir / filename
        status, body, _ = generate_voice_design_sample(
            base_url=base_url,
            api_key=api_key,
            text=text,
            caption=caption,
            seed=seed,
            http_fn=http_fn,
        )
        if status != 200:
            err = body.decode("utf-8", errors="replace")[:200]
            raise RuntimeError(
                f"Server returned status {status} for seed {seed}: {err}"
            )

        filepath.write_bytes(body)
        duration = get_wav_duration_seconds(body)
        manifest.append(
            {
                "seed": seed,
                "caption": caption,
                "text": text,
                "file": filename,
                "audio_seconds": round(duration, 3),
            }
        )
        # Incremental write so mid-run failure preserves completed entries
        manifest_file.write_text(
            json.dumps(manifest, indent=2, ensure_ascii=False), encoding="utf-8"
        )

    return manifest


def parse_args(args=None):
    """Parse CLI arguments for design_voices.py."""
    epilog = (
        "IMPORTANT OPERATIONAL NOTE:\n"
        "  The server must run with IRODORI_ALLOW_NO_REF_VOICE=true only during"
        " this step.\n"
        "  After generation, the chosen WAV is trimmed to 5-10 s and converted"
        " to a latent with:\n"
        "      python -m alt_irodori.encode_latent\n"
        "  inside the irodori-tts container."
    )
    parser = argparse.ArgumentParser(
        description=(
            "Generate VoiceDesign candidate audio samples for style prompting."
        ),
        epilog=epilog,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--base-url",
        default="http://127.0.0.1:8088",
        help="Base URL of Irodori-TTS server",
    )
    parser.add_argument(
        "--api-key-file",
        required=True,
        help="Path to file containing Bearer auth token",
    )
    parser.add_argument(
        "--caption",
        required=True,
        help="Style caption text for voice generation",
    )
    parser.add_argument(
        "--seeds",
        required=True,
        help="Seeds to generate, e.g. '1-20' or '1,5,9' or '1-5,10,12-14'",
    )
    parser.add_argument(
        "--text",
        default="本日は晴天なり。明日の天気予報をお知らせいたします。",
        help="Neutral sentence for voice evaluation",
    )
    parser.add_argument(
        "--out-dir",
        default=str(Path(__file__).resolve().parent / "results" / "voices"),
        help="Output directory for generated WAVs and manifest.json",
    )
    return parser.parse_args(args)


def main(args=None) -> int:
    """Entry point for design_voices."""
    parsed = parse_args(args)

    key_path = Path(parsed.api_key_file)
    if not key_path.is_file():
        sys.stderr.write(f"Error: API key file not found: {parsed.api_key_file}\n")
        return 2

    try:
        api_key = key_path.read_text(encoding="utf-8").strip()
    except OSError as e:
        sys.stderr.write(f"Error: Failed to read API key file: {e}\n")
        return 2

    if not api_key:
        sys.stderr.write("Error: API key file is empty\n")
        return 2

    try:
        seeds = parse_seeds(parsed.seeds)
    except ValueError as e:
        sys.stderr.write(f"Error parsing seeds: {e}\n")
        return 2

    out_dir = Path(parsed.out_dir)
    try:
        manifest = run_design_voices(
            base_url=parsed.base_url,
            api_key=api_key,
            caption=parsed.caption,
            seeds=seeds,
            text=parsed.text,
            out_dir=out_dir,
        )
    except (RuntimeError, OSError) as e:
        sys.stderr.write(f"Error during voice generation: {e}\n")
        return 1

    print(f"Generated {len(manifest)} voice samples in: {out_dir}")
    print(f"Manifest written to: {out_dir / 'manifest.json'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
