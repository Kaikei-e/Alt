import argparse
import json
import logging
import os
import sys
import yaml
from pathlib import Path
from typing import List, Dict

# CUDAライブラリのパスを動的に検出して設定（CUDA検出を確実にするため）
def _setup_cuda_library_path():
    """動的にCUDAライブラリのパスを検出してLD_LIBRARY_PATHに追加"""
    existing_paths = os.environ.get("LD_LIBRARY_PATH", "").split(":")
    existing_paths = [p for p in existing_paths if p]  # 空文字列を除去

    # 検出するパスの候補
    candidate_paths = []

    # 1. /usr/local/cuda* 配下を検索
    if os.path.exists("/usr/local"):
        for item in os.listdir("/usr/local"):
            cuda_path = os.path.join("/usr/local", item)
            if item.startswith("cuda") and os.path.isdir(cuda_path):
                targets_lib = os.path.join(cuda_path, "targets", "x86_64-linux", "lib")
                if os.path.exists(targets_lib):
                    candidate_paths.append(targets_lib)

    # 2. /usr/local/cuda のシンボリックリンクを確認
    cuda_link = "/usr/local/cuda"
    if os.path.exists(cuda_link):
        targets_lib = os.path.join(cuda_link, "targets", "x86_64-linux", "lib")
        if os.path.exists(targets_lib) and targets_lib not in candidate_paths:
            candidate_paths.append(targets_lib)

    # 3. システム標準パス
    system_paths = [
        "/usr/lib/x86_64-linux-gnu",
        "/usr/lib64",
    ]
    for path in system_paths:
        if os.path.exists(path) and path not in candidate_paths:
            candidate_paths.append(path)

    # 4. ldconfigで検出されたCUDAライブラリのパスを確認
    try:
        import subprocess
        result = subprocess.run(
            ["ldconfig", "-p"],
            capture_output=True,
            text=True,
            timeout=5
        )
        if result.returncode == 0:
            for line in result.stdout.split("\n"):
                if "cuda" in line.lower() and "=>" in line:
                    lib_path = line.split("=>")[-1].strip()
                    if lib_path:
                        dir_path = os.path.dirname(lib_path)
                        if os.path.exists(dir_path) and dir_path not in candidate_paths:
                            candidate_paths.append(dir_path)
    except Exception:
        pass  # ldconfigが使えない場合はスキップ

    # 存在するパスを追加
    new_paths = []
    for path in candidate_paths:
        if os.path.exists(path) and path not in existing_paths:
            new_paths.append(path)

    if new_paths:
        current_ld_path = os.environ.get("LD_LIBRARY_PATH", "")
        updated_ld_path = ":".join(new_paths + ([current_ld_path] if current_ld_path else []))
        os.environ["LD_LIBRARY_PATH"] = updated_ld_path

_setup_cuda_library_path()

import torch
from torch.utils.data import DataLoader
from tqdm import tqdm

# Path setup
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.learning_machine.teacher.model import TeacherBERT
# We can use a simpler dataset class for inference
from torch.utils.data import Dataset

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class InferenceDataset(Dataset):
    def __init__(self, texts: List[str], tokenizer, max_length: int = 256):
        self.texts = texts
        self.tokenizer = tokenizer
        self.max_length = max_length

    def __len__(self):
        return len(self.texts)

    def __getitem__(self, idx):
        text = str(self.texts[idx])
        encoding = self.tokenizer(
            text,
            add_special_tokens=True,
            max_length=self.max_length,
            return_token_type_ids=False,
            padding='max_length',
            truncation=True,
            return_attention_mask=True,
            return_tensors='pt',
        )
        return {
            'input_ids': encoding['input_ids'].flatten(),
            'attention_mask': encoding['attention_mask'].flatten()
        }

def load_genres(path: Path) -> List[str]:
    with open(path) as f:
        data = yaml.safe_load(f)
        return data.get("genres", [])

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--batch_size", type=int, default=32)
    parser.add_argument("--threshold", type=float, default=None, help="Confidence threshold (alternative to top-percent/margin)")
    parser.add_argument("--model_dir", type=str, default=None, help="Model directory (default: artifacts/teacher/v0_{language})")
    parser.add_argument("--input_path", type=str, default="recap_subworker/learning_machine/data/raw_articles.jsonl")
    parser.add_argument("--output_path", type=str, default=None, help="Output path (default: silver_teacher_v0_{language}.jsonl)")
    parser.add_argument("--language", type=str, choices=["ja", "en"], default="ja", help="Language filter for pseudo-labeling")
    parser.add_argument("--max_items", type=int, default=None, help="Limit number of items for distribution estimation (for stats_only)")
    parser.add_argument("--stats_only", action="store_true", help="Only output statistics, do not generate pseudo-labels")
    parser.add_argument("--accept_top_percent", type=float, default=None, help="Accept top P% of samples by confidence (alternative to threshold)")
    parser.add_argument("--min_margin", type=float, default=None, help="Minimum margin (top1 - top2) to accept (alternative to threshold)")
    parser.add_argument("--temperature", type=float, default=1.0, help="Temperature scaling for softmax (T>1 flattens, T<1 sharpens)")
    parser.add_argument("--per_class_cap", type=int, default=None, help="Maximum pseudo-labels per class (for class balance)")
    args = parser.parse_args()

    # Set defaults based on language
    if args.model_dir is None:
        args.model_dir = f"recap_subworker/learning_machine/artifacts/teacher/v0_{args.language}"
    if args.output_path is None:
        args.output_path = f"recap_subworker/learning_machine/data/silver_teacher_v0_{args.language}.jsonl"

    # GPU確認と詳細ログ
    if torch.cuda.is_available():
        device = torch.device("cuda")
        gpu_name = torch.cuda.get_device_name(0)
        gpu_memory = torch.cuda.get_device_properties(0).total_memory / 1024**3  # GB
        logger.info(
            f"GPU detected and will be used for pseudo-labeling - Device: {device}, "
            f"GPU: {gpu_name}, Memory: {round(gpu_memory, 2)}GB, "
            f"CUDA: {torch.version.cuda}"
        )
    else:
        device = torch.device("cpu")
        logger.warning(
            f"CUDA not available, using CPU. Pseudo-labeling will be slow! Device: {device}"
        )

    # 1. Load Taxonomy
    taxonomy_path = Path("recap_subworker/learning_machine/taxonomy/genres.yaml")
    genres = load_genres(taxonomy_path)
    # Reconstruct label maps (critical to match training)
    # Assuming the list order is stable.
    id2label = {i: g for i, g in enumerate(genres)}
    label2id = {g: i for i, g in enumerate(genres)}
    num_labels = len(genres)

    # 2. Load Model
    logger.info(f"Loading model from {args.model_dir}")
    # Note: TeacherBERT.from_pretrained expects a directory where config/weights are
    try:
        model = TeacherBERT.from_pretrained(args.model_dir, num_labels=num_labels)
        model.to(device)
        model.eval()
    except Exception as e:
        logger.error(f"Failed to load model: {e}")
        return

    # 3. Load Data
    raw_items = []

    # Simple language detection function (same as in learning_machine_classifier)
    def detect_language_simple(text: str, min_chars: int = 50) -> str:
        """Simple language detection for Japanese/English."""
        if len(text) < min_chars:
            return "unknown"
        has_japanese = any(
            "\u3040" <= char <= "\u309F" or  # Hiragana
            "\u30A0" <= char <= "\u30FF" or  # Katakana
            "\u4E00" <= char <= "\u9FAF"     # CJK Unified Ideographs
            for char in text
        )
        has_english = any(char.isascii() and char.isalpha() for char in text)
        jp_chars = sum(1 for char in text if "\u3040" <= char <= "\u309F" or "\u30A0" <= char <= "\u30FF" or "\u4E00" <= char <= "\u9FAF")
        en_chars = sum(1 for char in text if char.isascii() and char.isalpha())
        total_chars = len([c for c in text if c.isalnum() or ("\u3040" <= c <= "\u309F") or ("\u30A0" <= c <= "\u30FF") or ("\u4E00" <= c <= "\u9FAF")])
        if total_chars == 0:
            return "unknown"
        jp_ratio = jp_chars / total_chars if total_chars > 0 else 0
        en_ratio = en_chars / total_chars if total_chars > 0 else 0
        if has_japanese and jp_ratio > 0.1:
            return "ja"
        elif has_english and en_ratio > 0.3:
            return "en"
        elif has_japanese:
            return "ja"
        elif has_english:
            return "en"
        return "unknown"

    with open(args.input_path, "r", encoding="utf-8") as f:
        for line in f:
            if line.strip():
                item = json.loads(line)
                # Detect or use existing lang field
                if "lang" not in item:
                    content = item.get("content", "") or item.get("text", "")
                    detected_lang = detect_language_simple(content)
                    item["lang"] = detected_lang

                # Filter by language
                if item.get("lang") == args.language:
                    raw_items.append(item)

    logger.info(f"Loaded {len(raw_items)} raw articles (language={args.language}).")
    if not raw_items:
        logger.warning("No articles found after language filtering.")
        return

    # Apply max_items limit if specified (for stats_only mode)
    if args.max_items and args.max_items < len(raw_items):
        raw_items = raw_items[:args.max_items]
        logger.info(f"Limited to {args.max_items} items for distribution estimation.")

    # 4. Inference
    texts = [item.get("content", "") for item in raw_items]
    dataset = InferenceDataset(texts, model.tokenizer)
    loader = DataLoader(dataset, batch_size=args.batch_size, shuffle=False)

    pseudo_labeled = []
    all_probs = []
    confidence_stats = []
    margin_stats = []
    candidate_items = []  # Store all items with metadata for selection

    logger.info("Running inference...")
    with torch.no_grad():
        for i, batch in enumerate(tqdm(loader)):
            input_ids = batch['input_ids'].to(device)
            attention_mask = batch['attention_mask'].to(device)

            outputs = model(input_ids, attention_mask)
            logits = outputs.logits
            # Apply temperature scaling if specified
            scaled_logits = logits / args.temperature
            probs = torch.softmax(scaled_logits, dim=-1)

            # Move to CPU
            probs_np = probs.cpu().numpy()

            start_idx = i * args.batch_size
            for j, prob_dist in enumerate(probs_np):
                idx = start_idx + j
                if idx >= len(raw_items):
                    break

                max_prob = float(prob_dist.max())
                pred_id = int(prob_dist.argmax())
                pred_label = id2label[pred_id]

                # Calculate margin (top1 - top2)
                sorted_probs = sorted(prob_dist, reverse=True)
                margin = float(sorted_probs[0] - sorted_probs[1]) if len(sorted_probs) > 1 else 0.0

                # Collect statistics
                confidence_stats.append(max_prob)
                margin_stats.append(margin)

                # Store candidate item with metadata
                candidate_items.append({
                    "item": raw_items[idx].copy(),
                    "max_prob": max_prob,
                    "margin": margin,
                    "pred_label": pred_label,
                    "logits": logits[j].cpu().tolist(),
                })

    # Calculate statistics and determine selection criteria
    import numpy as np
    if not confidence_stats:
        logger.error("No confidence statistics collected!")
        return

    conf_array = np.array(confidence_stats)
    margin_array = np.array(margin_stats)

    # Calculate percentiles
    percentiles = {
        "p50": np.percentile(conf_array, 50),
        "p75": np.percentile(conf_array, 75),
        "p90": np.percentile(conf_array, 90),
        "p95": np.percentile(conf_array, 95),
        "p99": np.percentile(conf_array, 99),
    }

    # Determine selection method
    selection_method = None
    effective_threshold = None

    if args.accept_top_percent:
        selection_method = "top_percent"
        # Sort by confidence and take top P%
        sorted_indices = np.argsort(conf_array)[::-1]
        n_accept = int(len(candidate_items) * args.accept_top_percent / 100.0)
        selected_indices = sorted_indices[:n_accept]
        effective_threshold = conf_array[sorted_indices[n_accept - 1]] if n_accept > 0 else conf_array.max()
    elif args.min_margin is not None:
        selection_method = "margin"
        effective_threshold = f"margin >= {args.min_margin}"
        selected_indices = np.where(margin_array >= args.min_margin)[0]
    elif args.threshold is not None:
        selection_method = "threshold"
        effective_threshold = args.threshold
        selected_indices = np.where(conf_array >= args.threshold)[0]
    else:
        # Default: use top 5% if no method specified
        selection_method = "top_percent (default 5%)"
        sorted_indices = np.argsort(conf_array)[::-1]
        n_accept = max(1, int(len(candidate_items) * 5 / 100.0))
        selected_indices = sorted_indices[:n_accept]
        effective_threshold = conf_array[sorted_indices[n_accept - 1]] if n_accept > 0 else conf_array.max()

    # Apply per-class cap if specified
    if args.per_class_cap:
        class_counts = {}
        filtered_indices = []
        for idx in selected_indices:
            label = candidate_items[idx]["pred_label"]
            if class_counts.get(label, 0) < args.per_class_cap:
                filtered_indices.append(idx)
                class_counts[label] = class_counts.get(label, 0) + 1
        selected_indices = np.array(filtered_indices)
        logger.info(f"Applied per-class cap ({args.per_class_cap}): {len(selected_indices)} items selected")

    # Build pseudo-labeled items
    for idx in selected_indices:
        candidate = candidate_items[idx]
        item = candidate["item"].copy()
        item["label"] = candidate["pred_label"]
        item["confidence"] = candidate["max_prob"]
        item["margin"] = candidate["margin"]
        item["source"] = f"teacher_v0_{args.language}_pseudo"
        item["lang"] = args.language
        item["logits"] = candidate["logits"]
        pseudo_labeled.append(item)

    # Log comprehensive statistics
    logger.info("=" * 80)
    logger.info("Confidence Distribution Statistics")
    logger.info("=" * 80)
    logger.info(f"Mean: {conf_array.mean():.4f}")
    logger.info(f"Median: {np.median(conf_array):.4f}")
    logger.info(f"Std: {conf_array.std():.4f}")
    logger.info(f"Min: {conf_array.min():.4f}, Max: {conf_array.max():.4f}")
    logger.info(f"Percentiles - p50: {percentiles['p50']:.4f}, p75: {percentiles['p75']:.4f}, "
                f"p90: {percentiles['p90']:.4f}, p95: {percentiles['p95']:.4f}, p99: {percentiles['p99']:.4f}")
    logger.info("")
    logger.info("Margin Statistics (top1 - top2)")
    logger.info(f"Mean: {margin_array.mean():.4f}, Median: {np.median(margin_array):.4f}, "
                f"Max: {margin_array.max():.4f}, Min: {margin_array.min():.4f}")
    logger.info("")
    logger.info("Selection Method & Estimates")
    logger.info(f"Method: {selection_method}")
    logger.info(f"Effective threshold: {effective_threshold}")
    logger.info(f"Selected: {len(pseudo_labeled)}/{len(candidate_items)} ({len(pseudo_labeled)/len(candidate_items)*100:.1f}%)")

    # Estimate for different thresholds
    logger.info("")
    logger.info("Estimated counts for different thresholds:")
    for thresh in [0.3, 0.5, 0.7, 0.85, 0.9]:
        count = np.sum(conf_array >= thresh)
        logger.info(f"  threshold >= {thresh}: {count} items ({count/len(conf_array)*100:.1f}%)")

    # Estimate for different top-percent
    logger.info("")
    logger.info("Estimated counts for different top-percent:")
    for pct in [1, 5, 10, 20]:
        n_est = max(1, int(len(candidate_items) * pct / 100.0))
        thresh_est = np.percentile(conf_array, 100 - pct)
        logger.info(f"  top {pct}%: ~{n_est} items (threshold ~{thresh_est:.4f})")

    # Estimate for different margins
    logger.info("")
    logger.info("Estimated counts for different margins:")
    for margin_val in [0.05, 0.1, 0.15, 0.2]:
        count = np.sum(margin_array >= margin_val)
        logger.info(f"  margin >= {margin_val}: {count} items ({count/len(margin_array)*100:.1f}%)")

    logger.info("=" * 80)

    # Class distribution
    if pseudo_labeled:
        class_dist = {}
        for item in pseudo_labeled:
            label = item["label"]
            class_dist[label] = class_dist.get(label, 0) + 1
        logger.info("Class distribution (top 10):")
        sorted_classes = sorted(class_dist.items(), key=lambda x: x[1], reverse=True)
        for label, count in sorted_classes[:10]:
            logger.info(f"  {label}: {count} items")

    # If stats_only, exit here
    if args.stats_only:
        logger.info("Stats-only mode: exiting without saving pseudo-labels")
        return

    # 5. Save
    output_path = Path(args.output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    with open(output_path, "w", encoding="utf-8") as f:
        for item in pseudo_labeled:
            f.write(json.dumps(item, ensure_ascii=False) + "\n")

    logger.info(f"Generated {len(pseudo_labeled)} pseudo-labels")
    logger.info(f"Ratio: {len(pseudo_labeled)/len(raw_items):.1%}")
    logger.info(f"Saved to: {output_path}")

if __name__ == "__main__":
    main()
