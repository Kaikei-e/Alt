import argparse
import json
import logging
import os
import random
import shutil
import subprocess
import sys
from pathlib import Path
from typing import TypedDict

import yaml


def _append_if_exists(candidates: list[str], path: Path) -> None:
    path_str = str(path)
    if path.exists() and path_str not in candidates:
        candidates.append(path_str)


def _discover_ldconfig_cuda_dirs(candidates: list[str]) -> None:
    try:
        ldconfig_bin = shutil.which("ldconfig")
        if ldconfig_bin:
            result = subprocess.run(  # noqa: S603  # fixed ldconfig binary resolved via shutil.which
                [ldconfig_bin, "-p"], capture_output=True, text=True, timeout=5, check=False
            )
            if result.returncode == 0:
                for line in result.stdout.split("\n"):
                    if "cuda" in line.lower() and "=>" in line:
                        lib_path = line.split("=>")[-1].strip()
                        if lib_path:
                            _append_if_exists(candidates, Path(lib_path).parent)
    except (OSError, subprocess.SubprocessError, ValueError) as exc:
        logging.getLogger(__name__).debug("ldconfig lookup skipped: %s", exc)


# CUDAライブラリのパスを動的に検出して設定（CUDA検出を確実にするため）
def _setup_cuda_library_path() -> None:
    """動的にCUDAライブラリのパスを検出してLD_LIBRARY_PATHに追加"""
    existing_paths = [p for p in os.environ.get("LD_LIBRARY_PATH", "").split(":") if p]
    candidate_paths: list[str] = []

    # 1. /usr/local/cuda* 配下を検索
    usr_local = Path("/usr/local")
    if usr_local.exists():
        for cuda_path in usr_local.iterdir():
            if cuda_path.name.startswith("cuda") and cuda_path.is_dir():
                _append_if_exists(candidate_paths, cuda_path / "targets" / "x86_64-linux" / "lib")

    # 2. /usr/local/cuda のシンボリックリンクを確認
    _append_if_exists(candidate_paths, Path("/usr/local/cuda") / "targets" / "x86_64-linux" / "lib")

    # 3. システム標準パス
    for path in (Path("/usr/lib/x86_64-linux-gnu"), Path("/usr/lib64")):
        _append_if_exists(candidate_paths, path)

    # 4. ldconfigで検出されたCUDAライブラリのパスを確認
    _discover_ldconfig_cuda_dirs(candidate_paths)

    # 存在するパスを追加
    new_paths = [
        path for path in candidate_paths if Path(path).exists() and path not in existing_paths
    ]

    if new_paths:
        current_ld_path = os.environ.get("LD_LIBRARY_PATH", "")
        updated_ld_path = ":".join(new_paths + ([current_ld_path] if current_ld_path else []))
        os.environ["LD_LIBRARY_PATH"] = updated_ld_path


_setup_cuda_library_path()

from dataclasses import dataclass  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup

import numpy as np  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
import torch  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
import torch.nn.functional as F  # noqa: E402, N812  # PyTorch functional convention; import after CUDA LD_LIBRARY_PATH setup
from sklearn.metrics import f1_score  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from sklearn.model_selection import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    train_test_split,
)
from torch import nn  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from torch.optim import AdamW  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from torch.optim.lr_scheduler import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    LambdaLR,
)
from torch.utils.data import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    DataLoader,
    Dataset,
)
from transformers import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    PreTrainedTokenizerBase,
    get_linear_schedule_with_warmup,
)

# Path setup
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.learning_machine.student.model import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH and sys.path setup
    StudentDistilBERT,
)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class _DistillationItem(TypedDict):
    text: str
    label: int
    logits: list[float] | None
    lang: str | None


class DistillationDataset(Dataset):
    def __init__(
        self,
        items: list[_DistillationItem],
        tokenizer: PreTrainedTokenizerBase,
        num_labels: int,
        max_length: int = 256,
    ) -> None:
        self.items = items
        self.tokenizer = tokenizer
        self.num_labels = num_labels
        self.max_length = max_length

    def __len__(self) -> int:
        return len(self.items)

    def __getitem__(self, idx: int) -> dict[str, torch.Tensor]:
        item = self.items[idx]
        text = str(item["text"])
        label = item["label"]

        # Logits for distillation (if available)
        teacher_logits = item.get("logits")
        has_teacher_logits = False
        if teacher_logits:
            teacher_logits_tensor = torch.tensor(teacher_logits, dtype=torch.float)
            has_teacher_logits = True
        else:
            # If no logits, use dummy of correct shape
            teacher_logits_tensor = torch.zeros(self.num_labels, dtype=torch.float)

        encoding = self.tokenizer(
            text,
            add_special_tokens=True,
            max_length=self.max_length,
            return_token_type_ids=False,
            padding="max_length",
            truncation=True,
            return_attention_mask=True,
            return_tensors="pt",
        )

        return {
            "input_ids": encoding["input_ids"].flatten(),
            "attention_mask": encoding["attention_mask"].flatten(),
            "labels": torch.tensor(label, dtype=torch.long),
            "teacher_logits": teacher_logits_tensor,
            "has_teacher": torch.tensor(1 if has_teacher_logits else 0, dtype=torch.long),
        }


def load_genres(path: Path) -> list[str]:
    with path.open() as f:
        data = yaml.safe_load(f)
        return data.get("genres", [])


def load_jsonl(path: Path, label2id: dict[str, int]) -> list[_DistillationItem]:
    data: list[_DistillationItem] = []
    skipped = 0
    if not path.exists():
        return []
    with path.open(encoding="utf-8") as f:
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

                    if isinstance(lbl, list) and lbl:
                        lbl = lbl[0]

                    if text and lbl in label2id:
                        logits = obj.get("logits")
                        data.append(
                            {
                                "text": text,
                                "label": label2id[lbl],
                                "logits": logits,
                                "lang": obj.get("lang"),  # Preserve language field for filtering
                            }
                        )
                except (KeyError, ValueError, json.JSONDecodeError) as exc:
                    skipped += 1
                    logger.debug("skipping malformed training record: %s", exc)
    if skipped:
        logger.warning("skipped %d malformed training records", skipped)
    return data


@dataclass
class _StudentTrainingBundle:
    student: StudentDistilBERT
    train_loader: DataLoader
    val_loader: DataLoader
    optimizer: AdamW
    scheduler: LambdaLR
    ce_loss: nn.CrossEntropyLoss
    kl_loss: nn.KLDivLoss
    output_path: Path


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--epochs", type=int, default=None, help="Number of epochs (default: language-specific)"
    )
    parser.add_argument(
        "--batch_size", type=int, default=None, help="Batch size (default: language-specific)"
    )
    parser.add_argument(
        "--lr", type=float, default=None, help="Learning rate (default: language-specific)"
    )
    parser.add_argument(
        "--alpha",
        type=float,
        default=None,
        help="Distillation weight (0.0=Hard only, 1.0=Soft only, default: language-specific)",
    )
    parser.add_argument(
        "--temperature",
        type=float,
        default=None,
        help="Temperature for distillation (default: language-specific)",
    )
    parser.add_argument(
        "--weight_decay", type=float, default=None, help="Weight decay (default: language-specific)"
    )
    parser.add_argument(
        "--warmup_steps", type=int, default=None, help="Warmup steps (default: language-specific)"
    )
    parser.add_argument(
        "--max_length",
        type=int,
        default=None,
        help="Max sequence length (default: language-specific)",
    )
    parser.add_argument("--seed", type=int, default=42, help="Random seed for reproducibility")
    parser.add_argument(
        "--output_dir",
        type=str,
        default=None,
        help="Output directory (default: artifacts/student/v0_{language})",
    )
    parser.add_argument(
        "--language",
        type=str,
        choices=["ja", "en"],
        default="ja",
        help="Language filter for training data",
    )
    parser.add_argument(
        "--model_name",
        type=str,
        default=None,
        help="Base model name (default: Japanese DistilBERT for ja, distilbert-base-uncased for en)",
    )
    return parser.parse_args()


def _apply_language_defaults_and_seed(args: argparse.Namespace) -> None:
    # Language-specific defaults
    if args.language == "ja":
        # Japanese defaults (optimized for better macro F1 - Experiment 2 best: 0.8461)
        defaults = {
            "epochs": 10,
            "batch_size": 16,
            "lr": 2e-5,
            "alpha": 0.3,
            "temperature": 2.0,
            "weight_decay": 0.01,
            "warmup_steps": 100,
            "max_length": 256,
        }
    else:  # en
        # English defaults (current working well)
        defaults = {
            "epochs": 5,
            "batch_size": 32,
            "lr": 3e-5,
            "alpha": 0.5,
            "temperature": 2.0,
            "weight_decay": 0.01,
            "warmup_steps": 0,
            "max_length": 256,
        }

    for key, value in defaults.items():
        if getattr(args, key) is None:
            setattr(args, key, value)

    # Set random seed for reproducibility
    random.seed(args.seed)
    np.random.seed(args.seed)
    torch.manual_seed(args.seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(args.seed)

    logger.info("Random seed set to: %s", args.seed)
    logger.info(
        "Language-specific defaults (language=%s): "
        "epochs=%s, batch_size=%s, lr=%s, alpha=%s, temperature=%s, "
        "weight_decay=%s, warmup_steps=%s, max_length=%s",
        args.language,
        args.epochs,
        args.batch_size,
        args.lr,
        args.alpha,
        args.temperature,
        args.weight_decay,
        args.warmup_steps,
        args.max_length,
    )


def _resolve_device() -> torch.device:
    # GPU確認と詳細ログ
    if torch.cuda.is_available():
        device = torch.device("cuda")
        gpu_name = torch.cuda.get_device_name(0)
        gpu_memory = torch.cuda.get_device_properties(0).total_memory / 1024**3  # GB
        logger.info(
            "GPU detected and will be used - Device: %s, GPU: %s, Memory: %.2fGB, CUDA: %s",
            device,
            gpu_name,
            gpu_memory,
            torch.version.cuda,
        )
        return device
    device = torch.device("cpu")
    logger.warning("CUDA not available, using CPU. Training will be slow! Device: %s", device)
    return device


def _load_student_splits(
    args: argparse.Namespace, label2id: dict[str, int]
) -> tuple[list[_DistillationItem], list[_DistillationItem]]:
    gold_data = load_jsonl(Path("recap_subworker/learning_machine/data/gold_seed.jsonl"), label2id)
    silver_ext = load_jsonl(
        Path("recap_subworker/learning_machine/data/silver_external.jsonl"), label2id
    )
    silver_pseudo_path = Path(
        f"recap_subworker/learning_machine/data/silver_teacher_v0_{args.language}.jsonl"
    )
    silver_pseudo = load_jsonl(silver_pseudo_path, label2id) if silver_pseudo_path.exists() else []

    # Filter by language
    gold_data = [item for item in gold_data if item.get("lang") == args.language]
    silver_ext = [item for item in silver_ext if item.get("lang") == args.language]
    silver_pseudo = [item for item in silver_pseudo if item.get("lang") == args.language]

    logger.info(
        "Data (language=%s): Gold=%d, SilverExt=%d, Pseudo=%d",
        args.language,
        len(gold_data),
        len(silver_ext),
        len(silver_pseudo),
    )

    gold_train, gold_val = train_test_split(gold_data, test_size=0.5, random_state=args.seed)

    train_items = gold_train + silver_ext + silver_pseudo
    val_items = gold_val

    logger.info("Train Set: %d (Gold: %d)", len(train_items), len(gold_train))
    logger.info("Val Set (Gold Only): %d", len(val_items))
    return train_items, val_items


def _setup_student_training(
    args: argparse.Namespace,
    train_items: list[_DistillationItem],
    val_items: list[_DistillationItem],
    num_labels: int,
    device: torch.device,
) -> _StudentTrainingBundle:
    if args.output_dir is None:
        args.output_dir = f"recap_subworker/learning_machine/artifacts/student/v0_{args.language}"

    if args.model_name:
        model_name = args.model_name
    elif args.language == "ja":
        model_name = "line-corporation/line-distilbert-base-japanese"
    else:  # en
        model_name = "distilbert-base-uncased"

    logger.info("Using model: %s for language: %s", model_name, args.language)
    student = StudentDistilBERT(model_name, num_labels)
    student.to(device)

    train_dataset = DistillationDataset(
        train_items, student.tokenizer, num_labels, max_length=args.max_length
    )
    val_dataset = DistillationDataset(
        val_items, student.tokenizer, num_labels, max_length=args.max_length
    )

    train_loader = DataLoader(train_dataset, batch_size=args.batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=args.batch_size)

    optimizer = AdamW(student.parameters(), lr=args.lr, weight_decay=args.weight_decay)
    total_steps = len(train_loader) * args.epochs
    scheduler = get_linear_schedule_with_warmup(
        optimizer, num_warmup_steps=args.warmup_steps, num_training_steps=total_steps
    )

    ce_loss = nn.CrossEntropyLoss()
    kl_loss = nn.KLDivLoss(reduction="batchmean")

    output_path = Path(args.output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    return _StudentTrainingBundle(
        student=student,
        train_loader=train_loader,
        val_loader=val_loader,
        optimizer=optimizer,
        scheduler=scheduler,
        ce_loss=ce_loss,
        kl_loss=kl_loss,
        output_path=output_path,
    )


def _train_one_epoch(
    bundle: _StudentTrainingBundle,
    args: argparse.Namespace,
    device: torch.device,
    epoch: int,
) -> float:
    bundle.student.train()
    total_loss = 0.0

    # GPU使用状況の確認（最初のバッチのみ）
    if epoch == 0 and torch.cuda.is_available():
        torch.cuda.reset_peak_memory_stats()
        initial_memory = torch.cuda.memory_allocated(0) / 1024**3
        logger.info("Initial GPU memory usage: %.2f GB", initial_memory)

    for batch_idx, batch in enumerate(bundle.train_loader):
        bundle.optimizer.zero_grad()

        input_ids = batch["input_ids"].to(device)
        attention_mask = batch["attention_mask"].to(device)
        labels = batch["labels"].to(device)
        teacher_logits = batch["teacher_logits"].to(device)
        has_teacher = batch["has_teacher"].to(device)

        # 最初のバッチでGPU使用を確認
        if epoch == 0 and batch_idx == 0 and torch.cuda.is_available():
            logger.info(
                "First batch on GPU - Input shape: %s, Device: %s",
                input_ids.shape,
                input_ids.device,
            )

        outputs = bundle.student(input_ids, attention_mask)
        student_logits = outputs.logits

        # 1. Hard Loss
        loss_ce = bundle.ce_loss(student_logits, labels)

        # 2. Soft Loss (Distillation)
        loss_distill = torch.tensor(0.0, device=device)
        mask = has_teacher.bool()
        if mask.any():
            s_log = F.log_softmax(student_logits[mask] / args.temperature, dim=-1)
            t_prob = F.softmax(teacher_logits[mask] / args.temperature, dim=-1)
            loss_distill = bundle.kl_loss(s_log, t_prob) * (args.temperature**2)

        loss = args.alpha * loss_distill + (1.0 - args.alpha) * loss_ce if mask.any() else loss_ce

        loss.backward()
        bundle.optimizer.step()
        bundle.scheduler.step()
        total_loss += loss.item()

        # 最初のエポックの最初のバッチでGPUメモリ使用量を確認
        if epoch == 0 and batch_idx == 0 and torch.cuda.is_available():
            peak_memory = torch.cuda.max_memory_allocated(0) / 1024**3
            current_memory = torch.cuda.memory_allocated(0) / 1024**3
            logger.info(
                "GPU memory after first batch - Peak: %.2fGB, Current: %.2fGB",
                peak_memory,
                current_memory,
            )

    return total_loss / len(bundle.train_loader)


def _evaluate_student(
    student: StudentDistilBERT, val_loader: DataLoader, device: torch.device
) -> tuple[float, float]:
    student.eval()
    preds = []
    true_lbls = []

    for batch in val_loader:
        input_ids = batch["input_ids"].to(device)
        attention_mask = batch["attention_mask"].to(device)
        lbl = batch["labels"].to(device)

        with torch.no_grad():
            outputs = student(input_ids, attention_mask)

        pred = torch.argmax(outputs.logits, dim=1).cpu().numpy()
        preds.extend(pred)
        true_lbls.extend(lbl.cpu().numpy())

    val_f1_macro = f1_score(true_lbls, preds, average="macro")
    val_f1_weighted = f1_score(true_lbls, preds, average="weighted")
    return val_f1_macro, val_f1_weighted


def main() -> None:
    args = _parse_args()
    _apply_language_defaults_and_seed(args)
    device = _resolve_device()

    # Load Genres
    taxonomy_path = Path("recap_subworker/learning_machine/taxonomy/genres.yaml")
    genres = load_genres(taxonomy_path)
    label2id = {g: i for i, g in enumerate(genres)}
    num_labels = len(genres)

    # Load Data
    train_items, val_items = _load_student_splits(args, label2id)
    bundle = _setup_student_training(args, train_items, val_items, num_labels, device)

    best_f1 = 0.0
    for epoch in range(args.epochs):
        avg_train_loss = _train_one_epoch(bundle, args, device, epoch)
        val_f1_macro, val_f1_weighted = _evaluate_student(bundle.student, bundle.val_loader, device)
        logger.info(
            "Epoch %d | Loss: %.4f | Val Macro F1: %.4f | Val Weighted F1: %.4f",
            epoch + 1,
            avg_train_loss,
            val_f1_macro,
            val_f1_weighted,
        )

        # Use macro F1 as the primary metric for model selection
        if val_f1_macro >= best_f1:
            best_f1 = val_f1_macro
            bundle.student.save_pretrained(str(bundle.output_path))
            logger.info("New best model saved (Macro F1: %.4f)", best_f1)

    logger.info("Student training finished. Best Macro F1: %.4f", best_f1)


if __name__ == "__main__":
    main()
