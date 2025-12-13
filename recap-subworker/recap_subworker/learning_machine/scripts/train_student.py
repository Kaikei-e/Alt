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
import torch.nn.functional as F
from torch.utils.data import DataLoader, Dataset
from sklearn.model_selection import train_test_split
from sklearn.metrics import classification_report, f1_score
from transformers import get_linear_schedule_with_warmup
from torch.optim import AdamW
import random
import numpy as np

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
                            "logits": logits,
                            "lang": obj.get("lang")  # Preserve language field for filtering
                        })
                except:
                    pass
    return data

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--epochs", type=int, default=None, help="Number of epochs (default: language-specific)")
    parser.add_argument("--batch_size", type=int, default=None, help="Batch size (default: language-specific)")
    parser.add_argument("--lr", type=float, default=None, help="Learning rate (default: language-specific)")
    parser.add_argument("--alpha", type=float, default=None, help="Distillation weight (0.0=Hard only, 1.0=Soft only, default: language-specific)")
    parser.add_argument("--temperature", type=float, default=None, help="Temperature for distillation (default: language-specific)")
    parser.add_argument("--weight_decay", type=float, default=None, help="Weight decay (default: language-specific)")
    parser.add_argument("--warmup_steps", type=int, default=None, help="Warmup steps (default: language-specific)")
    parser.add_argument("--max_length", type=int, default=None, help="Max sequence length (default: language-specific)")
    parser.add_argument("--seed", type=int, default=42, help="Random seed for reproducibility")
    parser.add_argument("--output_dir", type=str, default=None, help="Output directory (default: artifacts/student/v0_{language})")
    parser.add_argument("--language", type=str, choices=["ja", "en"], default="ja", help="Language filter for training data")
    parser.add_argument("--model_name", type=str, default=None, help="Base model name (default: Japanese DistilBERT for ja, distilbert-base-uncased for en)")
    args = parser.parse_args()

    # Language-specific defaults
    if args.language == "ja":
        # Japanese defaults (optimized for better macro F1 - Experiment 2 best: 0.8461)
        default_epochs = 10
        default_batch_size = 16
        default_lr = 2e-5
        default_alpha = 0.3  # Hard label focused (best from experiments)
        default_temperature = 2.0
        default_weight_decay = 0.01
        default_warmup_steps = 100
        default_max_length = 256
    else:  # en
        # English defaults (current working well)
        default_epochs = 5
        default_batch_size = 32
        default_lr = 3e-5
        default_alpha = 0.5
        default_temperature = 2.0
        default_weight_decay = 0.01
        default_warmup_steps = 0
        default_max_length = 256

    # Apply defaults if not specified
    if args.epochs is None:
        args.epochs = default_epochs
    if args.batch_size is None:
        args.batch_size = default_batch_size
    if args.lr is None:
        args.lr = default_lr
    if args.alpha is None:
        args.alpha = default_alpha
    if args.temperature is None:
        args.temperature = default_temperature
    if args.weight_decay is None:
        args.weight_decay = default_weight_decay
    if args.warmup_steps is None:
        args.warmup_steps = default_warmup_steps
    if args.max_length is None:
        args.max_length = default_max_length

    # Set random seed for reproducibility
    random.seed(args.seed)
    np.random.seed(args.seed)
    torch.manual_seed(args.seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(args.seed)

    logger.info(f"Random seed set to: {args.seed}")
    logger.info(f"Language-specific defaults (language={args.language}): "
                f"epochs={args.epochs}, batch_size={args.batch_size}, lr={args.lr}, "
                f"alpha={args.alpha}, temperature={args.temperature}, "
                f"weight_decay={args.weight_decay}, warmup_steps={args.warmup_steps}, "
                f"max_length={args.max_length}")

    # Set default output_dir based on language
    if args.output_dir is None:
        args.output_dir = f"recap_subworker/learning_machine/artifacts/student/v0_{args.language}"

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

    # Load Genres
    taxonomy_path = Path("recap_subworker/learning_machine/taxonomy/genres.yaml")
    genres = load_genres(taxonomy_path)
    label2id = {g: i for i, g in enumerate(genres)}
    num_labels = len(genres)

    # Load Data
    gold_data = load_jsonl(Path("recap_subworker/learning_machine/data/gold_seed.jsonl"), label2id)
    silver_ext = load_jsonl(Path("recap_subworker/learning_machine/data/silver_external.jsonl"), label2id)
    # Load language-specific pseudo labels
    silver_pseudo_path = Path(f"recap_subworker/learning_machine/data/silver_teacher_v0_{args.language}.jsonl")
    silver_pseudo = load_jsonl(silver_pseudo_path, label2id) if silver_pseudo_path.exists() else []

    # Filter by language
    gold_data = [item for item in gold_data if item.get("lang") == args.language]
    silver_ext = [item for item in silver_ext if item.get("lang") == args.language]
    silver_pseudo = [item for item in silver_pseudo if item.get("lang") == args.language]

    logger.info(f"Data (language={args.language}): Gold={len(gold_data)}, SilverExt={len(silver_ext)}, Pseudo={len(silver_pseudo)}")

    # Validation uses ONLY Gold (and maybe a slice of silver if gold is too small, but aim for Gold)
    # Since Gold is small (60), we might need CrossValid, but here Stratified Split of combined?
    # NO. Validation set MUST be trustworthy.
    # Split Gold: 30 validation, 30 train. (Tiny!)
    # Or use ALL gold for valid? But then Student sees no gold in train.
    # Standard: Use Gold for Valid/Test only?
    # Given the goal is "Teacher -> Student", Student learns from Teacher (Pseudo + External).
    # Gold is primarily for EVALUATION.
    # Let's use 50% Gold for Val.

    gold_train, gold_val = train_test_split(gold_data, test_size=0.5, random_state=args.seed)

    train_items = gold_train + silver_ext + silver_pseudo
    val_items = gold_val

    logger.info(f"Train Set: {len(train_items)} (Gold: {len(gold_train)})")
    logger.info(f"Val Set (Gold Only): {len(val_items)}")

    # Model
    if args.model_name:
        model_name = args.model_name
    else:
        # Default models based on language
        if args.language == "ja":
            model_name = "line-corporation/line-distilbert-base-japanese"
        else:  # en
            model_name = "distilbert-base-uncased"

    logger.info(f"Using model: {model_name} for language: {args.language}")
    student = StudentDistilBERT(model_name, num_labels)
    student.to(device)

    train_dataset = DistillationDataset(train_items, student.tokenizer, num_labels, max_length=args.max_length)
    val_dataset = DistillationDataset(val_items, student.tokenizer, num_labels, max_length=args.max_length)

    train_loader = DataLoader(train_dataset, batch_size=args.batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=args.batch_size)

    optimizer = AdamW(student.parameters(), lr=args.lr, weight_decay=args.weight_decay)
    total_steps = len(train_loader) * args.epochs
    scheduler = get_linear_schedule_with_warmup(optimizer, num_warmup_steps=args.warmup_steps, num_training_steps=total_steps)

    ce_loss = nn.CrossEntropyLoss()
    kl_loss = nn.KLDivLoss(reduction="batchmean")

    best_f1 = 0.0
    output_path = Path(args.output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    for epoch in range(args.epochs):
        student.train()
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
            labels = batch['labels'].to(device)
            teacher_logits = batch['teacher_logits'].to(device)
            has_teacher = batch['has_teacher'].to(device)

            # 最初のバッチでGPU使用を確認
            if epoch == 0 and batch_idx == 0 and torch.cuda.is_available():
                logger.info(
                    f"First batch on GPU - Input shape: {input_ids.shape}, "
                    f"Device: {input_ids.device}"
                )

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

            # 最初のエポックの最初のバッチでGPUメモリ使用量を確認
            if epoch == 0 and batch_idx == 0 and torch.cuda.is_available():
                peak_memory = torch.cuda.max_memory_allocated(0) / 1024**3
                current_memory = torch.cuda.memory_allocated(0) / 1024**3
                logger.info(
                    f"GPU memory after first batch - "
                    f"Peak: {round(peak_memory, 2)}GB, Current: {round(current_memory, 2)}GB"
                )

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

        val_f1_macro = f1_score(true_lbls, preds, average="macro")
        val_f1_weighted = f1_score(true_lbls, preds, average="weighted")
        logger.info(f"Epoch {epoch+1} | Loss: {avg_train_loss:.4f} | Val Macro F1: {val_f1_macro:.4f} | Val Weighted F1: {val_f1_weighted:.4f}")

        # Use macro F1 as the primary metric for model selection
        if val_f1_macro >= best_f1:
            best_f1 = val_f1_macro
            student.save_pretrained(str(output_path))
            logger.info(f"New best model saved (Macro F1: {best_f1:.4f})")

    logger.info(f"Student training finished. Best Macro F1: {best_f1:.4f}")

if __name__ == "__main__":
    main()
