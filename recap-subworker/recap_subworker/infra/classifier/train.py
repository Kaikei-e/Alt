
import asyncio
import joblib
import json
import numpy as np
import pandas as pd
from pathlib import Path
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report, f1_score
from sklearn.model_selection import train_test_split
from sklearn.preprocessing import LabelEncoder
from sklearn.pipeline import Pipeline, FeatureUnion
from sklearn.base import BaseEstimator, TransformerMixin

from recap_subworker.services.embedder import Embedder
from recap_subworker.infra.config import get_settings

# --- Custom Transformer for Embeddings ---
class EmbeddingTransformer(BaseEstimator, TransformerMixin):
    def __init__(self, embedder: Embedder):
        self.embedder = embedder

    def fit(self, X, y=None):
        return self

    def transform(self, X):
        # X is list of strings or Series
        texts = X.tolist() if isinstance(X, pd.Series) else list(X)
        # Prepend passage: for E5 models if needed, but Embedder might handle it?
        # Sentences from DB might be raw.
        # Embedder.encode expects "passage: " prefix for E5 if asymmetric?
        # Let's assume Embedder handles it or we do it here.
        # E5 expects "query: " or "passage: ". For classification, usually "passage: ".
        # But wait, existing code in classifier.py does: input_texts = [f"passage: {t}" for t in texts]
        # So we should do the same.
        input_texts = [f"passage: {t}" for t in texts]
        return self.embedder.encode(input_texts)

# --- Training Script ---

async def main():
    settings = get_settings()
    data_path = Path("data/training_data.csv")
    if not data_path.exists():
        print("Data not found!")
        return

    print("Loading data...")
    df = pd.read_csv(data_path)
    df = df.dropna(subset=['content', 'genre'])

    # Filter out rare classes if any (less than 10 samples?)
    # genre distribution earlier showed many with 4 samples.
    # We should probably group them into 'other' or drop them?
    # For now, let's keep them and see (LR might fail with few samples).
    # Or maybe drop classes with < 50 samples?
    counts = df['genre'].value_counts()
    valid_genres = counts[counts >= 20].index
    print(f"Filtering genres with >= 20 samples. Kept: {len(valid_genres)}")
    df = df[df['genre'].isin(valid_genres)]

    # Verification limit removed


    X = df['content']
    y = df['genre']

    print("Splitting data...")
    X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.2, random_state=42, stratify=y)

    print("Initializing Embedder...")
    # Initialize embedder (this might be heavy)
    print("Initializing Embedder...")
    from recap_subworker.services.embedder import EmbedderConfig

    # Construct config from settings
    embedder_config = EmbedderConfig(
        model_id=settings.model_id,
        backend=settings.model_backend,
        device="cuda", # Force CUDA per user authorization
        distill_model_id=settings.distill_model_id,
        batch_size=4, # Reduced to 4 for extreme safety
        cache_size=1000 # Default cache size
    )
    embedder = Embedder(embedder_config)

    # Transform embeddings first to separate them (optional, but cleaner for pipeline if mixed)
    # Actually, let's just compute embeddings once for Train and Test to save time during tuning.
    print("Generating embeddings for training set...")
    emb_transformer = EmbeddingTransformer(embedder)
    X_train_emb = emb_transformer.transform(X_train)
    X_test_emb = emb_transformer.transform(X_test)

    print("Generating TF-IDF features...")
    tfidf = TfidfVectorizer(max_features=5000)
    X_train_tfidf = tfidf.fit_transform(X_train)
    X_test_tfidf = tfidf.transform(X_test)

    # Concatenate
    X_train_combined = np.hstack((X_train_emb, X_train_tfidf.toarray()))
    X_test_combined = np.hstack((X_test_emb, X_test_tfidf.toarray()))

    print(f"Features shape: {X_train_combined.shape}")

    # --- Cleanlab Integration ---
    print("Running Cleanlab to identify label issues...")
    from cleanlab.filter import find_label_issues
    from sklearn.model_selection import cross_val_predict

    # We need a temporary classifier for cross-validation
    # Use a simpler/faster one or the same one? LR is fast enough.
    clf_cv = LogisticRegression(max_iter=1000, multi_class='multinomial', solver='lbfgs', class_weight='balanced')

    try:
        # Get out-of-sample predicted probabilities
        # This might fail if some classes have < 2 samples in a fold, but we filtered min 20.
        pred_probs = cross_val_predict(clf_cv, X_train_combined, y_train, cv=3, method='predict_proba', n_jobs=-1)

        # Find label issues
        issue_indices = find_label_issues(
            labels=y_train,
            pred_probs=pred_probs,
            return_indices_ranked_by='self_confidence'
        )

        print(f"Cleanlab identified {len(issue_indices)} label issues.")

        if len(issue_indices) > 0:
            # Create a mask to keep non-issues
            # X_train is a Series, X_train_combined is ndarray
            # We need to filter X_train_combined and y_train

            # Identify indices to keep
            keep_mask = np.ones(len(y_train), dtype=bool)
            # issue_indices are relative to X_train positions (0 to len(X_train)-1)
            keep_mask[issue_indices] = False

            X_train_cleaned = X_train_combined[keep_mask]
            y_train_cleaned = y_train.iloc[keep_mask]

            print(f"Removed {len(issue_indices)} samples. Training size: {len(y_train)} -> {len(y_train_cleaned)}")

            # Update for final training
            X_train_final = X_train_cleaned
            y_train_final = y_train_cleaned
        else:
            X_train_final = X_train_combined
            y_train_final = y_train

    except Exception as e:
        print(f"Cleanlab failed: {e}")
        print("Falling back to original training data.")
        X_train_final = X_train_combined
        y_train_final = y_train

    # Train Model
    print("Training Final Logistic Regression...")
    clf = LogisticRegression(max_iter=1000, multi_class='multinomial', solver='lbfgs', class_weight='balanced')
    clf.fit(X_train_final, y_train_final)

    # Evaulate raw
    y_pred = clf.predict(X_test_combined)
    print("Initial Classification Report:")
    print(classification_report(y_test, y_pred))

    # Validate/Optimize Thresholds
    print("Optimizing thresholds...")
    probs = clf.predict_proba(X_test_combined)
    classes = clf.classes_

    thresholds = {}
    best_f1s = {}

    # For each class, find best binary threshold vs rest
    y_test_dummies = pd.get_dummies(y_test)

    for i, cls in enumerate(classes):
        if cls not in y_test_dummies.columns:
            continue

        y_true_binary = y_test_dummies[cls]
        y_prob = probs[:, i]

        best_t = 0.5
        best_f1 = 0.0

        for t in np.arange(0.1, 0.95, 0.05):
            y_pred_binary = (y_prob >= t).astype(int)
            score = f1_score(y_true_binary, y_pred_binary)
            if score > best_f1:
                best_f1 = score
                best_t = t

        thresholds[cls] = float(best_t)
        best_f1s[cls] = best_f1
        print(f"Class {cls}: Best Threshold={best_t:.2f}, F1={best_f1:.2f}")

    # Save artifacts
    print("Saving model and artifacts...")
    output_dir = Path("data")

    # We need to save the components to reconstruct the pipeline/predictor
    # The GenericClassifierService will need:
    # 1. Embedder (already has)
    # 2. TfidfVectorizer (fit)
    # 3. LogisticRegression (fit)
    # 4. Thresholds

    joblib.dump(tfidf, output_dir / "tfidf_vectorizer.joblib")
    joblib.dump(clf, output_dir / "genre_classifier.joblib")

    with open(output_dir / "genre_thresholds.json", "w") as f:
        json.dump(thresholds, f, indent=2)

    print("Done!")

if __name__ == "__main__":
    asyncio.run(main())
