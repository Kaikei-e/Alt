import argparse
import json
import pickle
import sys
from pathlib import Path

import joblib
import numpy as np
import pandas as pd
import scipy.sparse as sp
import torch
from sentence_transformers import SentenceTransformer
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.model_selection import train_test_split

# Add project root to sys.path to allow imports from recap_subworker
project_root = Path(__file__).parent.parent
sys.path.insert(0, str(project_root))

# Import SudachiTokenizer from the service module where it is defined cleanly
from recap_subworker.services.classifier import (  # noqa: E402 - import after sys.path setup
    SudachiTokenizer,
)

MIN_STRATIFY_SAMPLES = 2


class InputFileNotFoundError(FileNotFoundError):
    """Raised when the input golden dataset file does not exist."""

    def __init__(self, path: Path) -> None:
        super().__init__(f"Input file not found: {path}")


class InvalidGoldenDatasetError(ValueError):
    """Raised when the golden dataset JSON structure is invalid."""

    def __init__(self) -> None:
        super().__init__("Golden dataset has invalid structure")


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Prepare dataset for genre classification")
    parser.add_argument(
        "--input", type=str, required=True, help="Path to golden_classification.json"
    )
    parser.add_argument(
        "--output_dir", type=str, default="data/dataset", help="Output directory for pickle files"
    )
    parser.add_argument(
        "--model_name", type=str, default="BAAI/bge-m3", help="Embedding model name"
    )
    parser.add_argument(
        "--language",
        type=str,
        choices=["ja", "en", "both"],
        default="both",
        help="Language filter: 'ja' for Japanese only, 'en' for English only, 'both' for both (ja priority)",
    )
    return parser.parse_args()


def _extract_item_text(item: dict, language: str) -> str:
    if language == "ja":
        return item["content_ja"].strip() if item.get("content_ja") else ""
    if language == "en":
        return item["content_en"].strip() if item.get("content_en") else ""
    if item.get("content_ja"):
        return item["content_ja"].strip()
    if "content" in item:
        return item["content"].strip()
    if item.get("content_en"):
        return item["content_en"].strip()
    return ""


def _load_records(input_path: Path, language: str) -> pd.DataFrame:
    if not input_path.exists():
        raise InputFileNotFoundError(input_path)

    with input_path.open() as f:
        data = json.load(f)

    if isinstance(data, dict) and "items" in data:
        items = data["items"]
    elif isinstance(data, list):
        items = data
    else:
        raise InvalidGoldenDatasetError

    records = []
    for item in items:
        genres = item.get("expected_genres", [])
        if not genres:
            continue
        text = _extract_item_text(item, language)
        if not text:
            continue
        records.append({"text": text, "label": genres[0]})

    df = pd.DataFrame(records)
    sys.stdout.write(f"Total records: {len(df)}\n")
    vc = df["label"].value_counts()
    valid_labels = vc[vc >= 1].index
    df = df[df["label"].isin(valid_labels)]
    sys.stdout.write(f"Records after filtering: {len(df)}\n")
    return df


def _split_dataset(df: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    try:
        train_df, temp_df = train_test_split(
            df, test_size=0.5, stratify=df["label"], random_state=42
        )
    except ValueError:
        sys.stdout.write("Warning: Stratified split failed. Random split.\n")
        train_df, temp_df = train_test_split(df, test_size=0.5, random_state=42)

    try:
        if len(temp_df) < MIN_STRATIFY_SAMPLES:
            valid_df = temp_df.copy()
            test_df = temp_df.copy()
        else:
            valid_df, test_df = train_test_split(
                temp_df,
                test_size=0.5,
                stratify=temp_df["label"]
                if len(temp_df["label"].unique()) < len(temp_df)
                else None,
                random_state=42,
            )
    except ValueError:
        valid_df, test_df = train_test_split(temp_df, test_size=0.5, random_state=42)

    sys.stdout.write(f"Train: {len(train_df)}, Valid: {len(valid_df)}, Test: {len(test_df)}\n")
    return train_df, valid_df, test_df


def _build_vectorizer(language: str) -> TfidfVectorizer:
    if language == "en":
        return TfidfVectorizer(max_features=5000)
    tokenizer = SudachiTokenizer(mode="C")
    return TfidfVectorizer(tokenizer=tokenizer, max_features=5000)


def _embed_texts(model: SentenceTransformer, texts: list[str]) -> np.ndarray:
    prefixed = [f"passage: {t}" for t in texts]
    return model.encode(prefixed, normalize_embeddings=True, show_progress_bar=True)


def main() -> None:
    args = _parse_args()
    df = _load_records(Path(args.input), args.language)
    train_df, valid_df, test_df = _split_dataset(df)

    sys.stdout.write("Computing TF-IDF features...\n")
    vectorizer = _build_vectorizer(args.language)
    x_train_tfidf = vectorizer.fit_transform(train_df["text"])
    x_valid_tfidf = vectorizer.transform(valid_df["text"])
    x_test_tfidf = vectorizer.transform(test_df["text"])

    device = "cuda" if torch.cuda.is_available() else "cpu"
    sys.stdout.write(f"Loading model: {args.model_name} on {device}\n")
    model = SentenceTransformer(args.model_name, device=device)

    sys.stdout.write("Embedding Train set...\n")
    x_train_emb = _embed_texts(model, train_df["text"].tolist())
    sys.stdout.write("Embedding Valid set...\n")
    x_valid_emb = _embed_texts(model, valid_df["text"].tolist())
    sys.stdout.write("Embedding Test set...\n")
    x_test_emb = _embed_texts(model, test_df["text"].tolist())

    x_train_combined = sp.hstack([x_train_emb, x_train_tfidf], format="csr")
    x_valid_combined = sp.hstack([x_valid_emb, x_valid_tfidf], format="csr")
    x_test_combined = sp.hstack([x_test_emb, x_test_tfidf], format="csr")

    y_train = train_df["label"].values
    y_valid = valid_df["label"].values
    y_test = test_df["label"].values

    sys.stdout.write("Skipping SMOTE (using original data)...\n")
    x_train_res, y_train_res = x_train_combined, y_train

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    with (output_dir / "dataset_train.pkl").open("wb") as f:
        pickle.dump((x_train_res, y_train_res), f)

    with (output_dir / "dataset_valid.pkl").open("wb") as f:
        pickle.dump((x_valid_combined, y_valid), f)

    with (output_dir / "dataset_test.pkl").open("wb") as f:
        pickle.dump((x_test_combined, y_test), f)

    joblib.dump(vectorizer, output_dir / "tfidf_vectorizer.joblib")
    sys.stdout.write(f"Saved datasets and vectorizer to {output_dir}\n")


if __name__ == "__main__":
    main()
