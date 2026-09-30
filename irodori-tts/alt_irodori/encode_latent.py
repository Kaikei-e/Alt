from __future__ import annotations

import argparse
import os
from pathlib import Path

from irodori_tts.codec import DACVAECodec
import torch
import torchaudio


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Encode audio waveform to latent representation."
    )
    parser.add_argument("in_wav", type=Path, help="Input WAV path")
    parser.add_argument("out_pt", type=Path, help="Output PT path")
    parser.add_argument(
        "--device",
        default="cpu",
        help="Device to load codec on (default: cpu)",
    )
    args = parser.parse_args()

    codec_repo = os.environ["IRODORI_CODEC_REPO"]
    codec = DACVAECodec.load(repo_id=codec_repo, device=args.device)

    try:
        wav, sr = torchaudio.load(str(args.in_wav))
    except RuntimeError:
        import soundfile as sf

        data, sr = sf.read(str(args.in_wav), dtype="float32")
        wav = torch.from_numpy(data)
        if wav.ndim == 1:
            wav = wav.unsqueeze(0)
        else:
            wav = wav.T

    with torch.inference_mode():
        latent = codec.encode_waveform(wav, sample_rate=sr)[0].cpu()

    args.out_pt.parent.mkdir(parents=True, exist_ok=True)
    torch.save(latent, args.out_pt)


if __name__ == "__main__":
    main()
