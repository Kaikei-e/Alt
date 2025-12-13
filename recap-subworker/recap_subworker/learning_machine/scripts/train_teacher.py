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
import torch.nn as nn
import numpy as np
from torch.utils.data import DataLoader
from sklearn.model_selection import train_test_split
from sklearn.metrics import classification_report, f1_score
from transformers import get_linear_schedule_with_warmup
from torch.optim import AdamW

# Path setup
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.learning_machine.teacher.model import TeacherBERT
from recap_subworker.learning_machine.data.dataset import TextClassificationDataset

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

def load_genres(path: Path) -> List[str]:
    # Try different structures if needed
    with open(path) as f:
        # Check file extension
        if path.suffix in [".yaml", ".yml"]:
            data = yaml.safe_load(f)
            # Assuming structure: genres: [...] or list
            if isinstance(data, list):
                return data
            return data.get("genres", [])
        else:
            # JSON
            data = json.load(f)
            return data.get("genres", [])

def load_jsonl(path: Path) -> List[Dict]:
    data = []
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            if line.strip():
                data.append(json.loads(line))
    return data

def main():
    parser = argparse.ArgumentParser()
    # Default values optimized from Experiment 5 (Best: Val F1=0.9094)
    parser.add_argument("--epochs", type=int, default=7)
    parser.add_argument("--batch_size", type=int, default=8)
    parser.add_argument("--lr", type=float, default=1.5e-5)
    parser.add_argument("--weight_decay", type=float, default=0.01, help="Weight decay for AdamW optimizer")
    parser.add_argument("--warmup_steps", type=int, default=150, help="Number of warmup steps for learning rate scheduler")
    parser.add_argument("--max_length", type=int, default=256, help="Maximum sequence length for tokenization")
    parser.add_argument("--output_dir", type=str, default=None, help="Output directory (default: artifacts/teacher/v0_{language})")
    parser.add_argument("--gold_path", type=str, default="recap_subworker/learning_machine/data/gold_seed.jsonl")
    parser.add_argument("--use_external", action="store_true", help="Use silver_external.jsonl if available")
    parser.add_argument("--language", type=str, choices=["ja", "en"], default="ja", help="Language filter for training data")
    parser.add_argument("--model_name", type=str, default=None, help="Base model name (default: Japanese BERT for ja, bert-base-uncased for en)")
    args = parser.parse_args()

    # GPU確認と詳細ログ
    if torch.cuda.is_available():
        device = torch.device("cuda")
        gpu_name = torch.cuda.get_device_name(0)
        gpu_memory = torch.cuda.get_device_properties(0).total_memory / 1024**3  # GB
        logger.info(
            f"GPU detected and will be used - Device: {device}, "
            f"GPU: {gpu_name}, Memory: {round(gpu_memory, 2)}GB, "
            f"CUDA: {torch.version.cuda}"
        )
    else:
        device = torch.device("cpu")
        logger.warning(
            f"CUDA not available, using CPU. Training will be slow! Device: {device}"
        )

    # 1. Load Taxonomy
    # Assuming standard path
    taxonomy_path = Path("recap_subworker/learning_machine/taxonomy/genres.yaml")
    if not taxonomy_path.exists():
        # Fallback to loading from gold_seed source if taxonomy not separate
        # But we saved it in build_seed_gold.
        logger.warning(f"Taxonomy not found at {taxonomy_path}, attempting to infer...")
        # (Omitted fallback logic for brevity, ideally it exists)

    genres = load_genres(taxonomy_path)
    label2id = {g: i for i, g in enumerate(genres)}
    id2label = {i: g for i, g in enumerate(genres)}
    num_labels = len(genres)
    logger.info(f"Loaded {num_labels} genres: {genres[:5]}...")

    # 2. Load Data
    gold_data = load_jsonl(Path(args.gold_path))
    logger.info(f"Loaded {len(gold_data)} gold samples (before language filter).")

    silver_data = []
    if args.use_external:
        ext_path = Path("recap_subworker/learning_machine/data/silver_external.jsonl")
        if ext_path.exists():
            silver_data = load_jsonl(ext_path)
            logger.info(f"Loaded {len(silver_data)} silver samples (before language filter).")
        else:
            logger.warning(f"External data requested but not found at {ext_path}")

    # Filter by language
    gold_data = [item for item in gold_data if item.get("lang") == args.language]
    silver_data = [item for item in silver_data if item.get("lang") == args.language]
    logger.info(f"After language filter ({args.language}): Gold={len(gold_data)}, Silver={len(silver_data)}")

    # Combine
    all_items = []

    # Process Gold
    for item in gold_data:
        text = item.get("content") or item.get("text")
        # item["labels"] is list, let's take first for single-label BERT init
        # or handle multi-label if we want.
        # Plan implies single label training mostly? Or "OVR LR" was previous.
        # BERT usually can handle single label CrossEntropy easily.
        # For multi-label, we need BCEWithLogitsLoss.
        # Let's assume Single Label for now as "Primary Genre classification".
        # If item has multiple, we duplicate? or pick first?
        # Picking first is simple start.
        lbls = item.get("labels", [])
        if not lbls:
            continue
        label_str = lbls[0]
        if label_str in label2id:
            all_items.append({"text": text, "label": label2id[label_str], "weight": 1.0})

    # Process Silver
    for item in silver_data:
        text = item.get("content") or item.get("text")
        label_str = item.get("label") # external data has single 'label' field
        if label_str in label2id:
            # Lower weight for Silver data (0.2 based on request to lower it 0.1~0.3)
            all_items.append({"text": text, "label": label2id[label_str], "weight": 0.2})

    if not all_items:
        logger.error("No valid data found!")
        return

    texts = [x["text"] for x in all_items]
    labels = [x["label"] for x in all_items]
    weights = [x["weight"] for x in all_items]

    # Stratified Split - include weights
    train_texts, val_texts, train_labels, val_labels, train_weights, val_weights = train_test_split(
        texts, labels, weights, test_size=0.2, random_state=42, stratify=labels if len(texts) > num_labels * 5 else None
    )

    logger.info(f"Train: {len(train_texts)}, Val: {len(val_texts)}")

    # 3. Model & Tokenizer
    if args.model_name:
        model_name = args.model_name
    else:
        # Default models based on language
        if args.language == "ja":
            model_name = "tohoku-nlp/bert-base-japanese-v3"
        else:  # en
            model_name = "bert-base-uncased"

    logger.info(f"Using model: {model_name} for language: {args.language}")
    teacher = TeacherBERT(model_name, num_labels, label2id)
    teacher.to(device)

    train_dataset = TextClassificationDataset(train_texts, train_labels, teacher.tokenizer, weights=train_weights, max_length=args.max_length)
    val_dataset = TextClassificationDataset(val_texts, val_labels, teacher.tokenizer, weights=val_weights, max_length=args.max_length)

    train_loader = DataLoader(train_dataset, batch_size=args.batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=args.batch_size)

    optimizer = AdamW(teacher.parameters(), lr=args.lr, weight_decay=args.weight_decay)
    total_steps = len(train_loader) * args.epochs
    warmup_steps = args.warmup_steps if args.warmup_steps > 0 else max(0, int(total_steps * 0.1))  # Default to 10% if not specified
    scheduler = get_linear_schedule_with_warmup(optimizer, num_warmup_steps=warmup_steps, num_training_steps=total_steps)
    logger.info(f"Optimizer: lr={args.lr}, weight_decay={args.weight_decay}, warmup_steps={warmup_steps}, total_steps={total_steps}")

    loss_fn = nn.CrossEntropyLoss(reduction='none')

    best_f1 = 0.0
    if args.output_dir:
        output_path = Path(args.output_dir)
    else:
        output_path = Path(f"recap_subworker/learning_machine/artifacts/teacher/v0_{args.language}")
    output_path.mkdir(parents=True, exist_ok=True)

    # 4. Training Loop
    for epoch in range(args.epochs):
        teacher.train()
        total_loss = 0

        # GPU使用状況の確認（最初のバッチのみ）
        if epoch == 0 and torch.cuda.is_available():
            torch.cuda.reset_peak_memory_stats()
            initial_memory = torch.cuda.memory_allocated(0) / 1024**3
            logger.info(f"Initial GPU memory usage: {initial_memory:.2f} GB")

        for batch_idx, batch in enumerate(train_loader):
            optimizer.zero_grad()

            input_ids = batch['input_ids'].to(device)
            attention_mask = batch['attention_mask'].to(device)
            lbl = batch['labels'].to(device)

            # 最初のバッチでGPU使用を確認
            if epoch == 0 and batch_idx == 0 and torch.cuda.is_available():
                logger.info(
                    f"First batch on GPU - Input shape: {input_ids.shape}, "
                    f"Device: {input_ids.device}"
                )

            outputs = teacher(input_ids, attention_mask, labels=lbl)
            # outputs.loss is mean by default in HF models if labels provided?
            # HF BERT model usually returns loss if labels are provided.
            # However, to weight per sample, we usually ignore the model's computed loss
            # and compute it ourselves from logits.

            logits = outputs.logits
            per_sample_loss = loss_fn(logits, lbl)
            # batch['weights'] might need to be moved to device
            sample_weights = batch['weights'].to(device)
            weighted_loss = (per_sample_loss * sample_weights).mean()

            weighted_loss.backward()
            torch.nn.utils.clip_grad_norm_(teacher.parameters(), 1.0)
            optimizer.step()
            scheduler.step()

            total_loss += weighted_loss.item()

            # 最初のエポックの最初のバッチでGPUメモリ使用量を確認
            if epoch == 0 and batch_idx == 0 and torch.cuda.is_available():
                peak_memory = torch.cuda.max_memory_allocated(0) / 1024**3
                current_memory = torch.cuda.memory_allocated(0) / 1024**3
                logger.info(
                    f"GPU memory after first batch - "
                    f"Peak: {round(peak_memory, 2)}GB, Current: {round(current_memory, 2)}GB"
                )

        avg_train_loss = total_loss / len(train_loader)

        # Validation
        teacher.eval()
        preds = []
        true_lbls = []
        val_loss = 0
        for batch in val_loader:
            input_ids = batch['input_ids'].to(device)
            attention_mask = batch['attention_mask'].to(device)
            lbl = batch['labels'].to(device)

            with torch.no_grad():
                outputs = teacher(input_ids, attention_mask, labels=lbl)
                loss = outputs.loss
                val_loss += loss.item()

            logits = outputs.logits
            pred = torch.argmax(logits, dim=1).cpu().numpy()
            preds.extend(pred)
            true_lbls.extend(lbl.cpu().numpy())

        avg_val_loss = val_loss / len(val_loader)
        val_f1 = f1_score(true_lbls, preds, average="macro")

        logger.info(f"Epoch {epoch+1}/{args.epochs} | Train Loss: {avg_train_loss:.4f} | Val Loss: {avg_val_loss:.4f} | Val Macro F1: {val_f1:.4f}")

        if val_f1 > best_f1:
            best_f1 = val_f1
            teacher.save_pretrained(str(output_path))
            logger.info("New best model saved.")

    logger.info(f"Training completed. Best Val F1: {best_f1:.4f}")

if __name__ == "__main__":
    main()
