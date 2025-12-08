import argparse
import json
import logging
import sys
import yaml
from pathlib import Path
from typing import List, Dict

import torch
import torch.nn as nn
import torch.nn.functional as F
from torch.utils.data import DataLoader, Dataset
from sklearn.model_selection import train_test_split
from sklearn.metrics import classification_report, f1_score
from transformers import get_linear_schedule_with_warmup
from torch.optim import AdamW

# Path setup
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.learning_machine.student.model import StudentDistilBERT

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class DistillationDataset(Dataset):
    def __init__(self, items, tokenizer, num_labels: int, max_length: int = 256):
        self.items = items
        self.tokenizer = tokenizer
        self.num_labels = num_labels
        self.max_length = max_length

    def __len__(self):
        return len(self.items)

    def __getitem__(self, idx):
        item = self.items[idx]
        text = str(item["text"])
        label = item["label"]

        # Logits for distillation (if available)
        teacher_logits = item.get("logits")
        has_Teacher = False
        if teacher_logits:
             teacher_logits = torch.tensor(teacher_logits, dtype=torch.float)
             has_Teacher = True
        else:
             # If no logits, use dummy of correct shape
             teacher_logits = torch.zeros(self.num_labels, dtype=torch.float)

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
            'attention_mask': encoding['attention_mask'].flatten(),
            'labels': torch.tensor(label, dtype=torch.long),
            'teacher_logits': teacher_logits,
            'has_teacher': torch.tensor(1 if has_Teacher else 0, dtype=torch.long)
        }

def load_genres(path: Path) -> List[str]:
    with open(path) as f:
        data = yaml.safe_load(f)
        return data.get("genres", [])

def load_jsonl(path: Path, label2id: Dict) -> List[Dict]:
    data = []
    if not path.exists():
        return []
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            if line.strip():
                try:
                    obj = json.loads(line)
                    text = obj.get("content") or obj.get("text")

                    lbl = obj.get("label")
                    if lbl is None:
                        lbls = obj.get("labels")
                        if lbls and isinstance(lbls, list):
                            lbl = lbls[0]

                    if isinstance(lbl, list) and lbl: lbl = lbl[0]

                    if text and lbl in label2id:
                        logits = obj.get("logits")
                        data.append({
                            "text": text,
                            "label": label2id[lbl],
                            "logits": logits
                        })
                except:
                    pass
    return data

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--epochs", type=int, default=5)
    parser.add_argument("--batch_size", type=int, default=32)
    parser.add_argument("--lr", type=float, default=3e-5)
    parser.add_argument("--alpha", type=float, default=0.5, help="Distillation weight (0.0=Hard only, 1.0=Soft only)")
    parser.add_argument("--temperature", type=float, default=2.0)
    parser.add_argument("--output_dir", type=str, default="recap_subworker/learning_machine/artifacts/student/v0")
    args = parser.parse_args()

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    logger.info(f"Using device: {device}")

    # Load Genres
    taxonomy_path = Path("recap_subworker/learning_machine/taxonomy/genres.yaml")
    genres = load_genres(taxonomy_path)
    label2id = {g: i for i, g in enumerate(genres)}
    num_labels = len(genres)

    # Load Data
    gold_data = load_jsonl(Path("recap_subworker/learning_machine/data/gold_seed.jsonl"), label2id)
    silver_ext = load_jsonl(Path("recap_subworker/learning_machine/data/silver_external.jsonl"), label2id)
    silver_pseudo = load_jsonl(Path("recap_subworker/learning_machine/data/silver_teacher_v0.jsonl"), label2id)

    logger.info(f"Data: Gold={len(gold_data)}, SilverExt={len(silver_ext)}, Pseudo={len(silver_pseudo)}")

    # Validation uses ONLY Gold (and maybe a slice of silver if gold is too small, but aim for Gold)
    # Since Gold is small (60), we might need CrossValid, but here Stratified Split of combined?
    # NO. Validation set MUST be trustworthy.
    # Split Gold: 30 validation, 30 train. (Tiny!)
    # Or use ALL gold for valid? But then Student sees no gold in train.
    # Standard: Use Gold for Valid/Test only?
    # Given the goal is "Teacher -> Student", Student learns from Teacher (Pseudo + External).
    # Gold is primarily for EVALUATION.
    # Let's use 50% Gold for Val.

    gold_train, gold_val = train_test_split(gold_data, test_size=0.5, random_state=42)

    train_items = gold_train + silver_ext + silver_pseudo
    val_items = gold_val

    logger.info(f"Train Set: {len(train_items)} (Gold: {len(gold_train)})")
    logger.info(f"Val Set (Gold Only): {len(val_items)}")

    # Model
    model_name = "line-corporation/line-distilbert-base-japanese"
    student = StudentDistilBERT(model_name, num_labels)
    student.to(device)

    train_dataset = DistillationDataset(train_items, student.tokenizer, num_labels)
    val_dataset = DistillationDataset(val_items, student.tokenizer, num_labels)

    train_loader = DataLoader(train_dataset, batch_size=args.batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=args.batch_size)

    optimizer = AdamW(student.parameters(), lr=args.lr)
    total_steps = len(train_loader) * args.epochs
    scheduler = get_linear_schedule_with_warmup(optimizer, num_warmup_steps=0, num_training_steps=total_steps)

    ce_loss = nn.CrossEntropyLoss()
    kl_loss = nn.KLDivLoss(reduction="batchmean")

    best_f1 = 0.0
    output_path = Path(args.output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    for epoch in range(args.epochs):
        student.train()
        total_loss = 0
        for batch in train_loader:
            optimizer.zero_grad()

            input_ids = batch['input_ids'].to(device)
            attention_mask = batch['attention_mask'].to(device)
            labels = batch['labels'].to(device)
            teacher_logits = batch['teacher_logits'].to(device)
            has_teacher = batch['has_teacher'].to(device)

            outputs = student(input_ids, attention_mask)
            student_logits = outputs.logits

            # 1. Hard Loss
            loss_ce = ce_loss(student_logits, labels)

            # 2. Soft Loss (Distillation)
            # Only apply where we have teacher logits
            loss_distill = torch.tensor(0.0, device=device)
            # Find indices where has_teacher=1
            # But DistillationDataset fills dummy zeros if no teacher.
            # We assume pseudo-labeled data has logits. External has NOT. Gold has NOT.
            # So mixing is tricky in batch efficiently?
            # Masked select?

            # Global soft loss if we ignore mask? No, comparing to zeros is bad.
            # Only compute KL if has_teacher mask is active?
            # Mask out non-teacher items?
            mask = has_teacher.bool()
            if mask.any():
                # Filter
                s_log = F.log_softmax(student_logits[mask] / args.temperature, dim=-1)
                t_prob = F.softmax(teacher_logits[mask] / args.temperature, dim=-1)
                loss_distill = kl_loss(s_log, t_prob) * (args.temperature ** 2)

            if mask.any():
                # Weighted Sum
                loss = args.alpha * loss_distill + (1.0 - args.alpha) * loss_ce
            else:
                loss = loss_ce # Only hard labels

            loss.backward()
            optimizer.step()
            scheduler.step()
            total_loss += loss.item()

        avg_train_loss = total_loss / len(train_loader)

        # Validation on Gold
        student.eval()
        preds = []
        true_lbls = []

        for batch in val_loader:
            input_ids = batch['input_ids'].to(device)
            attention_mask = batch['attention_mask'].to(device)
            lbl = batch['labels'].to(device)

            with torch.no_grad():
                outputs = student(input_ids, attention_mask)

            pred = torch.argmax(outputs.logits, dim=1).cpu().numpy()
            preds.extend(pred)
            true_lbls.extend(lbl.cpu().numpy())

        val_f1 = f1_score(true_lbls, preds, average="macro")
        logger.info(f"Epoch {epoch+1} | Loss: {avg_train_loss:.4f} | Val F1 (Gold): {val_f1:.4f}")

        if val_f1 >= best_f1:
            best_f1 = val_f1
            student.save_pretrained(str(output_path))

    logger.info(f"Student training finished. Best F1: {best_f1:.4f}")

if __name__ == "__main__":
    main()
