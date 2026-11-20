import argparse
import json
import logging
import os
import shutil
import subprocess
import sys
from pathlib import Path

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
    except (OSError, subprocess.SubprocessError) as exc:
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
from typing import TypedDict  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup

import torch  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from sklearn.metrics import f1_score  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from sklearn.model_selection import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    train_test_split,
)
from torch import nn  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from torch.optim import AdamW  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from torch.optim.lr_scheduler import LambdaLR  # noqa: E402  # post-CUDA setup
from torch.utils.data import DataLoader  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from transformers import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    get_linear_schedule_with_warmup,
)

# Path setup
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.learning_machine.data.dataset import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH and sys.path setup
    TextClassificationDataset,
)
from recap_subworker.learning_machine.teacher.model import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH and sys.path setup
    TeacherBERT,
)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def load_genres(path: Path) -> list[str]:
    # Try different structures if needed
    with path.open() as f:
        # Check file extension
        if path.suffix in [".yaml", ".yml"]:
            data = yaml.safe_load(f)
            # Assuming structure: genres: [...] or list
            if isinstance(data, list):
                return data
            return data.get("genres", [])
        # JSON
        data = json.load(f)
        return data.get("genres", [])


def load_jsonl(path: Path) -> list[dict]:
    with path.open(encoding="utf-8") as f:
        return [json.loads(line) for line in f if line.strip()]


class _TeacherSample(TypedDict):
    text: str
    label: int
    weight: float


@dataclass
class _TeacherTrainingBundle:
    teacher: TeacherBERT
    train_loader: DataLoader
    val_loader: DataLoader
    optimizer: AdamW
    scheduler: LambdaLR
    loss_fn: nn.CrossEntropyLoss
    output_path: Path


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    # Default values optimized from Experiment 5 (Best: Val F1=0.9094)
    parser.add_argument("--epochs", type=int, default=7)
    parser.add_argument("--batch_size", type=int, default=8)
    parser.add_argument("--lr", type=float, default=1.5e-5)
    parser.add_argument(
        "--weight_decay", type=float, default=0.01, help="Weight decay for AdamW optimizer"
    )
    parser.add_argument(
        "--warmup_steps",
        type=int,
        default=150,
        help="Number of warmup steps for learning rate scheduler",
    )
    parser.add_argument(
        "--max_length", type=int, default=256, help="Maximum sequence length for tokenization"
    )
    parser.add_argument(
        "--output_dir",
        type=str,
        default=None,
        help="Output directory (default: artifacts/teacher/v0_{language})",
    )
    parser.add_argument(
        "--gold_path", type=str, default="recap_subworker/learning_machine/data/gold_seed.jsonl"
    )
    parser.add_argument(
        "--use_external", action="store_true", help="Use silver_external.jsonl if available"
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
        help="Base model name (default: Japanese BERT for ja, bert-base-uncased for en)",
    )
    return parser.parse_args()


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


def _load_teacher_items(args: argparse.Namespace, label2id: dict[str, int]) -> list[_TeacherSample]:
    # 2. Load Data
    gold_data = load_jsonl(Path(args.gold_path))
    logger.info("Loaded %d gold samples (before language filter).", len(gold_data))

    silver_data = []
    if args.use_external:
        ext_path = Path("recap_subworker/learning_machine/data/silver_external.jsonl")
        if ext_path.exists():
            silver_data = load_jsonl(ext_path)
            logger.info("Loaded %d silver samples (before language filter).", len(silver_data))
        else:
            logger.warning("External data requested but not found at %s", ext_path)

    # Filter by language
    gold_data = [item for item in gold_data if item.get("lang") == args.language]
    silver_data = [item for item in silver_data if item.get("lang") == args.language]
    logger.info(
        "After language filter (%s): Gold=%d, Silver=%d",
        args.language,
        len(gold_data),
        len(silver_data),
    )

    # Combine
    all_items: list[_TeacherSample] = []

    # Process Gold
    for item in gold_data:
        text = item.get("content") or item.get("text")
        lbls = item.get("labels", [])
        if not lbls:
            continue
        label_str = lbls[0]
        if label_str in label2id:
            all_items.append({"text": str(text), "label": label2id[label_str], "weight": 1.0})

    # Process Silver
    for item in silver_data:
        text = item.get("content") or item.get("text")
        label_str = item.get("label")  # external data has single 'label' field
        if label_str in label2id:
            # Lower weight for Silver data (0.2 based on request to lower it 0.1~0.3)
            all_items.append({"text": str(text), "label": label2id[label_str], "weight": 0.2})

    return all_items


def _setup_teacher_training(
    args: argparse.Namespace,
    all_items: list[_TeacherSample],
    label2id: dict[str, int],
    device: torch.device,
) -> _TeacherTrainingBundle:
    num_labels = len(label2id)
    texts = [x["text"] for x in all_items]
    labels = [x["label"] for x in all_items]
    weights = [x["weight"] for x in all_items]

    # Stratified Split - include weights
    train_texts, val_texts, train_labels, val_labels, train_weights, val_weights = train_test_split(
        texts,
        labels,
        weights,
        test_size=0.2,
        random_state=42,
        stratify=labels if len(texts) > num_labels * 5 else None,
    )

    logger.info("Train: %d, Val: %d", len(train_texts), len(val_texts))

    # 3. Model & Tokenizer
    if args.model_name:
        model_name = args.model_name
    # Default models based on language
    elif args.language == "ja":
        model_name = "tohoku-nlp/bert-base-japanese-v3"
    else:  # en
        model_name = "bert-base-uncased"

    logger.info("Using model: %s for language: %s", model_name, args.language)
    teacher = TeacherBERT(model_name, num_labels, label2id)
    teacher.to(device)

    train_dataset = TextClassificationDataset(
        train_texts,
        train_labels,
        teacher.tokenizer,
        weights=train_weights,
        max_length=args.max_length,
    )
    val_dataset = TextClassificationDataset(
        val_texts, val_labels, teacher.tokenizer, weights=val_weights, max_length=args.max_length
    )

    train_loader = DataLoader(train_dataset, batch_size=args.batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=args.batch_size)

    optimizer = AdamW(teacher.parameters(), lr=args.lr, weight_decay=args.weight_decay)
    total_steps = len(train_loader) * args.epochs
    warmup_steps = (
        args.warmup_steps if args.warmup_steps > 0 else max(0, int(total_steps * 0.1))
    )  # Default to 10% if not specified
    scheduler = get_linear_schedule_with_warmup(
        optimizer, num_warmup_steps=warmup_steps, num_training_steps=total_steps
    )
    logger.info(
        "Optimizer: lr=%s, weight_decay=%s, warmup_steps=%s, total_steps=%s",
        args.lr,
        args.weight_decay,
        warmup_steps,
        total_steps,
    )

    loss_fn = nn.CrossEntropyLoss(reduction="none")
    if args.output_dir:
        output_path = Path(args.output_dir)
    else:
        output_path = Path(f"recap_subworker/learning_machine/artifacts/teacher/v0_{args.language}")
    output_path.mkdir(parents=True, exist_ok=True)

    return _TeacherTrainingBundle(
        teacher=teacher,
        train_loader=train_loader,
        val_loader=val_loader,
        optimizer=optimizer,
        scheduler=scheduler,
        loss_fn=loss_fn,
        output_path=output_path,
    )


def _train_one_epoch(bundle: _TeacherTrainingBundle, device: torch.device, epoch: int) -> float:
    bundle.teacher.train()
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
        lbl = batch["labels"].to(device)

        # 最初のバッチでGPU使用を確認
        if epoch == 0 and batch_idx == 0 and torch.cuda.is_available():
            logger.info(
                "First batch on GPU - Input shape: %s, Device: %s",
                input_ids.shape,
                input_ids.device,
            )

        outputs = bundle.teacher(input_ids, attention_mask, labels=lbl)
        logits = outputs.logits
        per_sample_loss = bundle.loss_fn(logits, lbl)
        sample_weights = batch["weights"].to(device)
        weighted_loss = (per_sample_loss * sample_weights).mean()

        weighted_loss.backward()
        torch.nn.utils.clip_grad_norm_(bundle.teacher.parameters(), 1.0)
        bundle.optimizer.step()
        bundle.scheduler.step()

        total_loss += weighted_loss.item()

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


def _evaluate_teacher(
    teacher: TeacherBERT, val_loader: DataLoader, device: torch.device
) -> tuple[float, float]:
    teacher.eval()
    preds = []
    true_lbls = []
    val_loss = 0.0
    for batch in val_loader:
        input_ids = batch["input_ids"].to(device)
        attention_mask = batch["attention_mask"].to(device)
        lbl = batch["labels"].to(device)

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
    return avg_val_loss, val_f1


def main() -> None:
    args = _parse_args()
    device = _resolve_device()

    # 1. Load Taxonomy
    taxonomy_path = Path("recap_subworker/learning_machine/taxonomy/genres.yaml")
    if not taxonomy_path.exists():
        logger.warning("Taxonomy not found at %s, attempting to infer...", taxonomy_path)

    genres = load_genres(taxonomy_path)
    label2id = {g: i for i, g in enumerate(genres)}
    num_labels = len(genres)
    logger.info("Loaded %d genres: %s...", num_labels, genres[:5])

    # 2. Load Data
    all_items = _load_teacher_items(args, label2id)
    if not all_items:
        logger.error("No valid data found!")
        return

    # 3. Model & Tokenizer
    bundle = _setup_teacher_training(args, all_items, label2id, device)

    # 4. Training Loop
    best_f1 = 0.0
    for epoch in range(args.epochs):
        avg_train_loss = _train_one_epoch(bundle, device, epoch)
        avg_val_loss, val_f1 = _evaluate_teacher(bundle.teacher, bundle.val_loader, device)

        logger.info(
            "Epoch %d/%d | Train Loss: %.4f | Val Loss: %.4f | Val Macro F1: %.4f",
            epoch + 1,
            args.epochs,
            avg_train_loss,
            avg_val_loss,
            val_f1,
        )

        if val_f1 > best_f1:
            best_f1 = val_f1
            bundle.teacher.save_pretrained(str(bundle.output_path))
            logger.info("New best model saved.")

    logger.info("Training completed. Best Val F1: %.4f", best_f1)


if __name__ == "__main__":
    main()
