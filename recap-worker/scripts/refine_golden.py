
import argparse
import json
import logging
import sys
from collections import defaultdict
from typing import List, Dict, Set, Any, Tuple

try:
    import numpy as np
    from sklearn.metrics import f1_score, precision_recall_fscore_support
    import pandas as pd
except ImportError:
    print("Error: Required libraries not installed. Please install numpy, pandas, scikit-learn.")
    sys.exit(1)

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

def load_data(path: str) -> List[Dict[str, Any]]:
    try:
        with open(path, 'r', encoding='utf-8') as f:
            return json.load(f)
    except Exception as e:
        logger.error(f"Failed to load data from {path}: {e}")
        sys.exit(1)

def extract_labels(data: List[Dict[str, Any]], label_key: str = 'genres') -> Tuple[List[str], List[List[str]]]:
    ids = []
    labels = []
    for item in data:
        ids.append(item.get('id', 'unknown'))
        item_labels = item.get(label_key, [])
        if isinstance(item_labels, str):
            item_labels = [item_labels]
        labels.append([l.lower().strip() for l in item_labels])
    return ids, labels

def align_data(golden: List[Dict[str, Any]], predictions: List[Dict[str, Any]]) -> Tuple[List[List[str]], List[List[str]]]:
    golden_map = {item['id']: item for item in golden}

    y_true = []
    y_pred = []

    for pred in predictions:
        pid = pred.get('id')
        if pid in golden_map:
            # Golden labels
            g_labels = golden_map[pid].get('genres', [])
            if isinstance(g_labels, str): g_labels = [g_labels]
            y_true.append([l.lower().strip() for l in g_labels])

            # Predicted labels
            p_labels = pred.get('genres', [])
            if isinstance(p_labels, str): p_labels = [p_labels]
            # Try 'top_genres' if 'genres' is missing in prediction
            if not p_labels and 'top_genres' in pred:
                p_labels = pred['top_genres']

            y_pred.append([l.lower().strip() for l in p_labels])

    logger.info(f"Aligned {len(y_true)} items from {len(golden)} golden and {len(predictions)} predictions.")
    return y_true, y_pred

def compute_metrics(y_true: List[List[str]], y_pred: List[List[str]]):
    from sklearn.preprocessing import MultiLabelBinarizer

    mlb = MultiLabelBinarizer()
    y_true_bin = mlb.fit_transform(y_true)
    y_pred_bin = mlb.transform(y_pred)

    classes = mlb.classes_
    logger.info(f"Classes found: {classes}")

    # Macro F1
    macro_f1 = f1_score(y_true_bin, y_pred_bin, average='macro', zero_division=0)
    micro_f1 = f1_score(y_true_bin, y_pred_bin, average='micro', zero_division=0)

    logger.info("=" * 40)
    logger.info(f"Macro-F1: {macro_f1:.4f}")
    logger.info(f"Micro-F1: {micro_f1:.4f}")
    logger.info("=" * 40)

    # Per-class metrics
    precision, recall, f1, support = precision_recall_fscore_support(y_true_bin, y_pred_bin, average=None, zero_division=0)

    df = pd.DataFrame({
        'Genre': classes,
        'Precision': precision,
        'Recall': recall,
        'F1': f1,
        'Support': support
    })

    print("\nPer-Class Metrics:")
    print(df.to_string(index=False))

    return macro_f1

def check_bilingual_consistency(golden: List[Dict[str, Any]]):
    # Check if pairs exist
    # Assuming ID format or specific tracking
    # For now, just check if we have ja/en for the same content?
    # Or just summary statistics
    pass

def main():
    parser = argparse.ArgumentParser(description="Evaluate Recap Genre Classification using Macro-F1")
    parser.add_argument("--golden", required=True, help="Path to golden dataset JSON")
    parser.add_argument("--predictions", required=False, help="Path to predictions JSON")

    args = parser.parse_args()

    golden_data = load_data(args.golden)

    if args.predictions:
        pred_data = load_data(args.predictions)
        y_true, y_pred = align_data(golden_data, pred_data)
        compute_metrics(y_true, y_pred)
    else:
        logger.info("No predictions provided. Analyzing golden dataset stats only.")
        # Just stats
        cnt = defaultdict(int)
        for item in golden_data:
            genres = item.get('genres', [])
            if isinstance(genres, str): genres = [genres]
            for g in genres:
                cnt[g.lower().strip()] += 1
        print("Label Distribution in Golden:")
        for k, v in sorted(cnt.items(), key=lambda x: -x[1]):
            print(f"{k}: {v}")

if __name__ == "__main__":
    main()
