#!/usr/bin/env python3
"""Convert SentenceTransformer model to ONNX format for faster inference."""

import os
import sys
from pathlib import Path
import logging

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def convert_to_onnx(
    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2",
    output_dir: str = "/models/onnx",
    max_length: int = 256,
) -> Path:
    """
    Convert SentenceTransformer model to ONNX format.

    Args:
        model_name: HuggingFace model identifier
        output_dir: Directory to save ONNX model
        max_length: Maximum sequence length for tokenization

    Returns:
        Path to the converted ONNX model file
    """
    try:
        from optimum.onnxruntime import ORTModelForFeatureExtraction
        from optimum.onnxruntime.configuration import AutoOptimizationConfig
        from optimum.onnxruntime import ORTOptimizer
        from transformers import AutoTokenizer
    except ImportError:
        logger.error(
            "optimum[onnxruntime] is required for ONNX conversion. "
            "Install with: pip install optimum[onnxruntime]"
        )
        sys.exit(1)

    # Normalize model name: add sentence-transformers/ prefix if not present
    if not model_name.startswith("sentence-transformers/") and "/" not in model_name:
        model_name = f"sentence-transformers/{model_name}"
        logger.info(f"Normalized model name to: {model_name}")

    # Get Hugging Face token from environment
    hf_token = os.getenv("HF_TOKEN") or os.getenv("HUGGINGFACE_HUB_TOKEN")
    if hf_token and hf_token != "placeholder":
        logger.info("Using Hugging Face authentication token from environment")
    else:
        logger.warning(
            "No valid Hugging Face token found. "
            "Some models may require authentication. "
            "Set HF_TOKEN or HUGGINGFACE_HUB_TOKEN environment variable if needed."
        )
        hf_token = None

    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    logger.info(f"Converting {model_name} to ONNX format...")
    logger.info(f"Output directory: {output_path}")

    try:
        # Load the model and convert to ONNX
        logger.info("Loading model and tokenizer...")
        model = ORTModelForFeatureExtraction.from_pretrained(
            model_name,
            export=True,  # Convert to ONNX
            token=hf_token,
        )
        tokenizer = AutoTokenizer.from_pretrained(
            model_name,
            token=hf_token,
        )

        # Save ONNX model
        onnx_model_path = output_path / "model.onnx"
        logger.info(f"Saving ONNX model to {onnx_model_path}...")

        # Export the model
        model.save_pretrained(str(output_path))
        tokenizer.save_pretrained(str(output_path))

        # The actual ONNX file is typically saved as model.onnx
        # Check if it exists, otherwise look for the standard ONNX file
        if not (output_path / "model.onnx").exists():
            # Check for the model file in the standard location
            onnx_files = list(output_path.glob("*.onnx"))
            if onnx_files:
                onnx_model_path = onnx_files[0]
                logger.info(f"Found ONNX model at: {onnx_model_path}")
            else:
                # Try to find it in a subdirectory
                for subdir in output_path.iterdir():
                    if subdir.is_dir():
                        onnx_files = list(subdir.glob("*.onnx"))
                        if onnx_files:
                            onnx_model_path = onnx_files[0]
                            logger.info(f"Found ONNX model at: {onnx_model_path}")
                            break

        if onnx_model_path.exists():
            logger.info(f"✅ ONNX model saved successfully: {onnx_model_path}")
            logger.info(f"   Model size: {onnx_model_path.stat().st_size / (1024 * 1024):.2f} MB")
            return onnx_model_path
        else:
            logger.warning("ONNX model file not found in expected location")
            logger.info("Model files saved to:", output_path)
            for file in output_path.rglob("*"):
                if file.is_file():
                    logger.info(f"  - {file}")
            return output_path / "model.onnx"  # Return expected path even if not found

    except Exception as e:
        logger.error(f"Failed to convert model to ONNX: {e}", exc_info=True)
        sys.exit(1)


def main():
    """Main function for ONNX conversion."""
    import argparse

    parser = argparse.ArgumentParser(description="Convert SentenceTransformer to ONNX")
    parser.add_argument(
        "--model-name",
        default="sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2",
        help="HuggingFace model identifier (will auto-add sentence-transformers/ prefix if needed)",
    )
    parser.add_argument(
        "--output-dir",
        default=os.getenv("ONNX_OUTPUT_DIR", "/models/onnx"),
        help="Output directory for ONNX model",
    )
    parser.add_argument(
        "--max-length",
        type=int,
        default=256,
        help="Maximum sequence length",
    )

    args = parser.parse_args()

    logger.info("Starting ONNX conversion...")
    logger.info(f"Model: {args.model_name}")
    logger.info(f"Output: {args.output_dir}")

    onnx_path = convert_to_onnx(
        model_name=args.model_name,
        output_dir=args.output_dir,
        max_length=args.max_length,
    )

    logger.info(f"✅ Conversion complete! ONNX model available at: {onnx_path}")
    logger.info(f"   Set TAG_ONNX_MODEL_PATH={onnx_path} to use it")


if __name__ == "__main__":
    main()

