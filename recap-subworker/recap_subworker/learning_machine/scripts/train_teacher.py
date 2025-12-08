import argparse
import json
import logging
import sys
import yaml
from pathlib import Path
from typing import List, Dict

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
    parser.add_argument("--epochs", type=int, default=5)
    parser.add_argument("--batch_size", type=int, default=16)
    parser.add_argument("--lr", type=float, default=2e-5)
    parser.add_argument("--output_dir", type=str, default="recap_subworker/learning_machine/artifacts/teacher/v0")
    parser.add_argument("--gold_path", type=str, default="recap_subworker/learning_machine/data/gold_seed.jsonl")
    parser.add_argument("--use_external", action="store_true", help="Use silver_external.jsonl if available")
    args = parser.parse_args()

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    logger.info(f"Using device: {device}")

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
    logger.info(f"Loaded {len(gold_data)} gold samples.")

    silver_data = []
    if args.use_external:
        ext_path = Path("recap_subworker/learning_machine/data/silver_external.jsonl")
        if ext_path.exists():
            silver_data = load_jsonl(ext_path)
            logger.info(f"Loaded {len(silver_data)} silver samples.")
        else:
            logger.warning(f"External data requested but not found at {ext_path}")

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
        text = item["content"]
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
    model_name = "tohoku-nlp/bert-base-japanese-v3"
    teacher = TeacherBERT(model_name, num_labels, label2id)
    teacher.to(device)

    train_dataset = TextClassificationDataset(train_texts, train_labels, teacher.tokenizer, weights=train_weights)
    val_dataset = TextClassificationDataset(val_texts, val_labels, teacher.tokenizer, weights=val_weights)

    train_loader = DataLoader(train_dataset, batch_size=args.batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=args.batch_size)

    optimizer = AdamW(teacher.parameters(), lr=args.lr)
    total_steps = len(train_loader) * args.epochs
    scheduler = get_linear_schedule_with_warmup(optimizer, num_warmup_steps=0, num_training_steps=total_steps)

    loss_fn = nn.CrossEntropyLoss(reduction='none')

    best_f1 = 0.0
    output_path = Path(args.output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    # 4. Training Loop
    for epoch in range(args.epochs):
        teacher.train()
        total_loss = 0
        for batch in train_loader:
            optimizer.zero_grad()

            input_ids = batch['input_ids'].to(device)
            attention_mask = batch['attention_mask'].to(device)
            lbl = batch['labels'].to(device)

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
