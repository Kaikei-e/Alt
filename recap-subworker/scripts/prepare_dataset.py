import json
import pickle
import argparse
import sys
from pathlib import Path

# Add project root to sys.path to allow imports from recap_subworker
project_root = Path(__file__).parent.parent
sys.path.insert(0, str(project_root))

from sklearn.model_selection import train_test_split
from sentence_transformers import SentenceTransformer
from sklearn.feature_extraction.text import TfidfVectorizer
# from imblearn.over_sampling import SMOTE
# from sudachipy import tokenizer, dictionary # Removed, inside SudachiTokenizer now
import pandas as pd
import numpy as np
import scipy.sparse as sp
import joblib

# Import SudachiTokenizer from the service module where it is defined cleanly
from recap_subworker.services.classifier import SudachiTokenizer

def main():
    parser = argparse.ArgumentParser(description="Prepare dataset for genre classification")
    parser.add_argument("--input", type=str, required=True, help="Path to golden_classification.json")
    parser.add_argument("--output_dir", type=str, default="data/dataset", help="Output directory for pickle files")
    parser.add_argument("--model_name", type=str, default="intfloat/multilingual-e5-large", help="Embedding model name")
    args = parser.parse_args()

    # Load Golden Dataset
    input_path = Path(args.input)
    if not input_path.exists():
        raise FileNotFoundError(f"Input file not found: {input_path}")

    with open(input_path, "r") as f:
        data = json.load(f)

    if isinstance(data, dict) and "items" in data:
        items = data["items"]
    elif isinstance(data, list):
        items = data
    else:
        raise ValueError("Golden dataset has invalid structure")

    # Extract text and labels
    records = []
    for item in items:
        text = None
        # Order: content_ja -> content -> content_en (Plan emphasizes Japanese processing)
        # Actually plan says: "Japanese tokenization... TF-IDF... E5".
        # Let's prioritize JA content for tokenization if we want to capture JA specific nuance.
        if "content_ja" in item and item.get("content_ja"):
            text = item["content_ja"].strip()
        elif "content" in item:
            text = item["content"].strip()
        elif "content_en" in item and item.get("content_en"):
            # Fallback to English if no Japanese
            text = item["content_en"].strip()
        else:
            text = ""

        genres = item.get("expected_genres", [])

        if not text or not genres:
            continue

        primary_label = genres[0]
        # For E5, we add prefix
        # We store raw text for TF-IDF, and "passage: " text for E5 separately if needed.
        # But let's keep it simple: pass raw text to TF-IDF. E5 needs "passage: ".
        records.append({"text": text, "label": primary_label})

    df = pd.DataFrame(records)
    print(f"Total records: {len(df)}")

    # Filter rare labels
    vc = df["label"].value_counts()
    valid_labels = vc[vc >= 1].index
    df = df[df["label"].isin(valid_labels)]
    print(f"Records after filtering: {len(df)}")

    # Split Data
    # Stratified split 50/50 Train/Temp, then Temp -> 50/50 Valid/Test
    try:
        train_df, temp_df = train_test_split(df, test_size=0.5, stratify=df["label"], random_state=42)
    except ValueError:
        print("Warning: Stratified split failed. Random split.")
        train_df, temp_df = train_test_split(df, test_size=0.5, random_state=42)

    try:
        if len(temp_df) < 2:
             valid_df = temp_df.copy()
             test_df = temp_df.copy()
        else:
             valid_df, test_df = train_test_split(
                temp_df, test_size=0.5, stratify=temp_df["label"] if len(temp_df["label"].unique()) < len(temp_df) else None, random_state=42
            )
    except ValueError:
        valid_df, test_df = train_test_split(temp_df, test_size=0.5, random_state=42)

    print(f"Train: {len(train_df)}, Valid: {len(valid_df)}, Test: {len(test_df)}")

    # 1. Sudachi Tokenization + TF-IDF
    print("Computing TF-IDF features...")
    sudachi_tokenizer = SudachiTokenizer(mode="C")
    vectorizer = TfidfVectorizer(tokenizer=sudachi_tokenizer, max_features=5000) # Limit features to avoid explosion

    X_train_tfidf = vectorizer.fit_transform(train_df["text"])
    X_valid_tfidf = vectorizer.transform(valid_df["text"])
    X_test_tfidf = vectorizer.transform(test_df["text"])

    # 2. E5 Embeddings
    import torch
    device = "cuda" if torch.cuda.is_available() else "cpu"
    print(f"Loading model: {args.model_name} on {device}")
    model = SentenceTransformer(args.model_name, device=device)

    def embed(texts):
        # E5 expects "passage: " prefix for documents
        prefixed = [f"passage: {t}" for t in texts]
        return model.encode(prefixed, normalize_embeddings=True, show_progress_bar=True)

    print("Embedding Train set...")
    X_train_emb = embed(train_df["text"].tolist())
    print("Embedding Valid set...")
    X_valid_emb = embed(valid_df["text"].tolist())
    print("Embedding Test set...")
    X_test_emb = embed(test_df["text"].tolist())

    # 3. Concatenate Features
    # X_train_tfidf is sparse, X_train_emb is dense (numpy)
    # Convert embedding to sparse or tfidf to dense?
    # Embeddings are 1024 dim. TF-IDF is 5000.
    # Let's stack them as dense arrays for simplicity, assuming memory fits (small dataset for now).
    # Or keep sparse? SMOTE supports sparse.

    X_train_combined = sp.hstack([X_train_emb, X_train_tfidf], format='csr')
    X_valid_combined = sp.hstack([X_valid_emb, X_valid_tfidf], format='csr')
    X_test_combined = sp.hstack([X_test_emb, X_test_tfidf], format='csr')

    y_train = train_df["label"].values
    y_valid = valid_df["label"].values
    y_test = test_df["label"].values

    # 4. SMOTE Oversampling on Train -> SKIPPED (Dependency Hell)
    print("Skipping SMOTE (using original data)...")
    X_train_res, y_train_res = X_train_combined, y_train

    # print("Applying SMOTE to training data...")
    # # Check min samples for SMOTE k_neighbors (default 5)
    # min_samples = pd.Series(y_train).value_counts().min()
    # k_neighbors = min(min_samples - 1, 5) if min_samples > 1 else 1

    # if min_samples > 1:
    #     smote = SMOTE(k_neighbors=k_neighbors, random_state=42)
    #     try:
    #         X_train_res, y_train_res = smote.fit_resample(X_train_combined, y_train)
    #         print(f"Resampled Train shape: {X_train_res.shape}")
    #     except ValueError as e:
    #         print(f"SMOTE failed: {e}. Using original data.")
    #         X_train_res, y_train_res = X_train_combined, y_train
    # else:
    #      print("Validation too small for SMOTE. Using original data.")
    #      X_train_res, y_train_res = X_train_combined, y_train

    # Save
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    with open(output_dir / "dataset_train.pkl", "wb") as f:
        pickle.dump((X_train_res, y_train_res), f)

    with open(output_dir / "dataset_valid.pkl", "wb") as f:
        pickle.dump((X_valid_combined, y_valid), f)

    with open(output_dir / "dataset_test.pkl", "wb") as f:
        pickle.dump((X_test_combined, y_test), f)

    # Save Vectorizer
    import joblib
    joblib.dump(vectorizer, output_dir / "tfidf_vectorizer.joblib")

    print(f"Saved datasets and vectorizer to {output_dir}")

if __name__ == "__main__":
    main()
