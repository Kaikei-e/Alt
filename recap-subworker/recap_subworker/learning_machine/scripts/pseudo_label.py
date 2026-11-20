import argparse
import json
import logging
import os
import shutil
import subprocess
import sys
from pathlib import Path

import yaml

_MIN_JA_CHAR_RATIO = 0.1
_MIN_EN_CHAR_RATIO = 0.3


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

import numpy as np  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
import torch  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from torch.utils.data import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    DataLoader,
    Dataset,
)
from tqdm import tqdm  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
from transformers import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH setup
    PreTrainedTokenizerBase,
)

# Path setup
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.learning_machine.teacher.model import (  # noqa: E402  # import after CUDA LD_LIBRARY_PATH and sys.path setup
    TeacherBERT,
)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class InferenceDataset(Dataset):
    def __init__(
        self, texts: list[str], tokenizer: PreTrainedTokenizerBase, max_length: int = 256
    ) -> None:
        self.texts = texts
        self.tokenizer = tokenizer
        self.max_length = max_length

    def __len__(self) -> int:
        return len(self.texts)

    def __getitem__(self, idx: int) -> dict[str, torch.Tensor]:
        text = str(self.texts[idx])
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
        }


def load_genres(path: Path) -> list[str]:
    with path.open() as f:
        data = yaml.safe_load(f)
        return data.get("genres", [])


def _is_japanese_char(char: str) -> bool:
    return (
        "\u3040" <= char <= "\u309f" or "\u30a0" <= char <= "\u30ff" or "\u4e00" <= char <= "\u9faf"
    )


# Simple language detection function (same as in learning_machine_classifier)
def detect_language_simple(text: str, min_chars: int = 50) -> str:
    """Simple language detection for Japanese/English."""
    if len(text) < min_chars:
        return "unknown"
    jp_chars = sum(1 for char in text if _is_japanese_char(char))
    en_chars = sum(1 for char in text if char.isascii() and char.isalpha())
    total_chars = sum(1 for c in text if c.isalnum() or _is_japanese_char(c))
    if total_chars == 0:
        return "unknown"
    jp_ratio = jp_chars / total_chars
    en_ratio = en_chars / total_chars
    if jp_chars > 0 and jp_ratio > _MIN_JA_CHAR_RATIO:
        return "ja"
    if en_chars > 0 and en_ratio > _MIN_EN_CHAR_RATIO:
        return "en"
    if jp_chars > 0:
        return "ja"
    return "en" if en_chars > 0 else "unknown"


@dataclass
class _PseudoLabelStats:
    conf_array: np.ndarray
    margin_array: np.ndarray
    percentiles: dict[str, float]
    selection_method: str
    effective_threshold: object
    pseudo_labeled: list[dict]
    candidate_items: list[dict]


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--batch_size", type=int, default=32)
    parser.add_argument(
        "--threshold",
        type=float,
        default=None,
        help="Confidence threshold (alternative to top-percent/margin)",
    )
    parser.add_argument(
        "--model_dir",
        type=str,
        default=None,
        help="Model directory (default: artifacts/teacher/v0_{language})",
    )
    parser.add_argument(
        "--input_path", type=str, default="recap_subworker/learning_machine/data/raw_articles.jsonl"
    )
    parser.add_argument(
        "--output_path",
        type=str,
        default=None,
        help="Output path (default: silver_teacher_v0_{language}.jsonl)",
    )
    parser.add_argument(
        "--language",
        type=str,
        choices=["ja", "en"],
        default="ja",
        help="Language filter for pseudo-labeling",
    )
    parser.add_argument(
        "--max_items",
        type=int,
        default=None,
        help="Limit number of items for distribution estimation (for stats_only)",
    )
    parser.add_argument(
        "--stats_only",
        action="store_true",
        help="Only output statistics, do not generate pseudo-labels",
    )
    parser.add_argument(
        "--accept_top_percent",
        type=float,
        default=None,
        help="Accept top P% of samples by confidence (alternative to threshold)",
    )
    parser.add_argument(
        "--min_margin",
        type=float,
        default=None,
        help="Minimum margin (top1 - top2) to accept (alternative to threshold)",
    )
    parser.add_argument(
        "--temperature",
        type=float,
        default=1.0,
        help="Temperature scaling for softmax (T>1 flattens, T<1 sharpens)",
    )
    parser.add_argument(
        "--per_class_cap",
        type=int,
        default=None,
        help="Maximum pseudo-labels per class (for class balance)",
    )
    args = parser.parse_args()
    if args.model_dir is None:
        args.model_dir = f"recap_subworker/learning_machine/artifacts/teacher/v0_{args.language}"
    if args.output_path is None:
        args.output_path = (
            f"recap_subworker/learning_machine/data/silver_teacher_v0_{args.language}.jsonl"
        )
    return args


def _resolve_device() -> torch.device:
    # GPU確認と詳細ログ
    if torch.cuda.is_available():
        device = torch.device("cuda")
        gpu_name = torch.cuda.get_device_name(0)
        gpu_memory = torch.cuda.get_device_properties(0).total_memory / 1024**3  # GB
        logger.info(
            "GPU detected and will be used for pseudo-labeling - Device: %s, "
            "GPU: %s, Memory: %.2fGB, CUDA: %s",
            device,
            gpu_name,
            gpu_memory,
            torch.version.cuda,
        )
        return device
    device = torch.device("cpu")
    logger.warning(
        "CUDA not available, using CPU. Pseudo-labeling will be slow! Device: %s",
        device,
    )
    return device


def _load_teacher_model(
    model_dir: str, num_labels: int, device: torch.device
) -> TeacherBERT | None:
    logger.info("Loading model from %s", model_dir)
    try:
        model = TeacherBERT.from_pretrained(model_dir, num_labels=num_labels)
        model.to(device)
        model.eval()
    except Exception:  # CLI script exits cleanly if teacher model fails to load
        logger.exception("Failed to load model")
        return None
    else:
        return model


def _load_and_filter_articles(args: argparse.Namespace) -> list[dict]:
    raw_items = []
    with Path(args.input_path).open(encoding="utf-8") as f:
        for line in f:
            if line.strip():
                item = json.loads(line)
                if "lang" not in item:
                    content = item.get("content", "") or item.get("text", "")
                    item["lang"] = detect_language_simple(content)
                if item.get("lang") == args.language:
                    raw_items.append(item)

    logger.info("Loaded %d raw articles (language=%s).", len(raw_items), args.language)
    if raw_items and args.max_items and args.max_items < len(raw_items):
        raw_items = raw_items[: args.max_items]
        logger.info("Limited to %d items for distribution estimation.", args.max_items)
    return raw_items


def _run_inference(
    model: TeacherBERT,
    raw_items: list[dict],
    id2label: dict[int, str],
    args: argparse.Namespace,
    device: torch.device,
) -> tuple[list[dict], list[float], list[float]]:
    texts = [item.get("content", "") for item in raw_items]
    dataset = InferenceDataset(texts, model.tokenizer)
    loader = DataLoader(dataset, batch_size=args.batch_size, shuffle=False)

    confidence_stats: list[float] = []
    margin_stats: list[float] = []
    candidate_items: list[dict] = []

    logger.info("Running inference...")
    with torch.no_grad():
        for i, batch in enumerate(tqdm(loader)):
            input_ids = batch["input_ids"].to(device)
            attention_mask = batch["attention_mask"].to(device)

            outputs = model(input_ids, attention_mask)
            logits = outputs.logits
            scaled_logits = logits / args.temperature
            probs = torch.softmax(scaled_logits, dim=-1)
            probs_np = probs.cpu().numpy()

            start_idx = i * args.batch_size
            for j, prob_dist in enumerate(probs_np):
                idx = start_idx + j
                if idx >= len(raw_items):
                    break

                max_prob = float(prob_dist.max())
                pred_id = int(prob_dist.argmax())
                pred_label = id2label[pred_id]

                sorted_probs = sorted(prob_dist, reverse=True)
                margin = float(sorted_probs[0] - sorted_probs[1]) if len(sorted_probs) > 1 else 0.0

                confidence_stats.append(max_prob)
                margin_stats.append(margin)
                candidate_items.append(
                    {
                        "item": raw_items[idx].copy(),
                        "max_prob": max_prob,
                        "margin": margin,
                        "pred_label": pred_label,
                        "logits": logits[j].cpu().tolist(),
                    }
                )
    return candidate_items, confidence_stats, margin_stats


def _select_candidates(
    candidate_items: list[dict],
    conf_array: np.ndarray,
    margin_array: np.ndarray,
    args: argparse.Namespace,
) -> tuple[np.ndarray, str, object]:
    if args.accept_top_percent:
        selection_method = "top_percent"
        sorted_indices = np.argsort(conf_array)[::-1]
        n_accept = int(len(candidate_items) * args.accept_top_percent / 100.0)
        selected_indices = sorted_indices[:n_accept]
        effective_threshold: object = (
            conf_array[sorted_indices[n_accept - 1]] if n_accept > 0 else conf_array.max()
        )
    elif args.min_margin is not None:
        selection_method = "margin"
        effective_threshold = f"margin >= {args.min_margin}"
        selected_indices = np.where(margin_array >= args.min_margin)[0]
    elif args.threshold is not None:
        selection_method = "threshold"
        effective_threshold = args.threshold
        selected_indices = np.where(conf_array >= args.threshold)[0]
    else:
        selection_method = "top_percent (default 5%)"
        sorted_indices = np.argsort(conf_array)[::-1]
        n_accept = max(1, int(len(candidate_items) * 5 / 100.0))
        selected_indices = sorted_indices[:n_accept]
        effective_threshold = (
            conf_array[sorted_indices[n_accept - 1]] if n_accept > 0 else conf_array.max()
        )

    if args.per_class_cap:
        class_counts: dict[str, int] = {}
        filtered_indices = []
        for idx in selected_indices:
            label = candidate_items[idx]["pred_label"]
            if class_counts.get(label, 0) < args.per_class_cap:
                filtered_indices.append(idx)
                class_counts[label] = class_counts.get(label, 0) + 1
        selected_indices = np.array(filtered_indices)
        logger.info(
            "Applied per-class cap (%s): %d items selected",
            args.per_class_cap,
            len(selected_indices),
        )

    return selected_indices, selection_method, effective_threshold


def _build_pseudo_labeled_items(
    candidate_items: list[dict], selected_indices: np.ndarray, language: str
) -> list[dict]:
    pseudo_labeled = []
    for idx in selected_indices:
        candidate = candidate_items[idx]
        item = candidate["item"].copy()
        item["label"] = candidate["pred_label"]
        item["confidence"] = candidate["max_prob"]
        item["margin"] = candidate["margin"]
        item["source"] = f"teacher_v0_{language}_pseudo"
        item["lang"] = language
        item["logits"] = candidate["logits"]
        pseudo_labeled.append(item)
    return pseudo_labeled


def _log_pseudo_label_statistics(stats: _PseudoLabelStats) -> None:
    logger.info("=" * 80)
    logger.info("Confidence Distribution Statistics")
    logger.info("=" * 80)
    logger.info("Mean: %.4f", stats.conf_array.mean())
    logger.info("Median: %.4f", np.median(stats.conf_array))
    logger.info("Std: %.4f", stats.conf_array.std())
    logger.info("Min: %.4f, Max: %.4f", stats.conf_array.min(), stats.conf_array.max())
    logger.info(
        "Percentiles - p50: %.4f, p75: %.4f, p90: %.4f, p95: %.4f, p99: %.4f",
        stats.percentiles["p50"],
        stats.percentiles["p75"],
        stats.percentiles["p90"],
        stats.percentiles["p95"],
        stats.percentiles["p99"],
    )
    logger.info("")
    logger.info("Margin Statistics (top1 - top2)")
    logger.info(
        "Mean: %.4f, Median: %.4f, Max: %.4f, Min: %.4f",
        stats.margin_array.mean(),
        np.median(stats.margin_array),
        stats.margin_array.max(),
        stats.margin_array.min(),
    )
    logger.info("")
    logger.info("Selection Method & Estimates")
    logger.info("Method: %s", stats.selection_method)
    logger.info("Effective threshold: %s", stats.effective_threshold)
    logger.info(
        "Selected: %d/%d (%.1f%%)",
        len(stats.pseudo_labeled),
        len(stats.candidate_items),
        len(stats.pseudo_labeled) / len(stats.candidate_items) * 100,
    )

    logger.info("")
    logger.info("Estimated counts for different thresholds:")
    for thresh in [0.3, 0.5, 0.7, 0.85, 0.9]:
        count = np.sum(stats.conf_array >= thresh)
        logger.info(
            "  threshold >= %s: %d items (%.1f%%)",
            thresh,
            count,
            count / len(stats.conf_array) * 100,
        )

    logger.info("")
    logger.info("Estimated counts for different top-percent:")
    for pct in [1, 5, 10, 20]:
        n_est = max(1, int(len(stats.candidate_items) * pct / 100.0))
        thresh_est = np.percentile(stats.conf_array, 100 - pct)
        logger.info("  top %d%%: ~%d items (threshold ~%.4f)", pct, n_est, thresh_est)

    logger.info("")
    logger.info("Estimated counts for different margins:")
    for margin_val in [0.05, 0.1, 0.15, 0.2]:
        count = np.sum(stats.margin_array >= margin_val)
        logger.info(
            "  margin >= %s: %d items (%.1f%%)",
            margin_val,
            count,
            count / len(stats.margin_array) * 100,
        )

    logger.info("=" * 80)

    if stats.pseudo_labeled:
        class_dist: dict[str, int] = {}
        for item in stats.pseudo_labeled:
            label = item["label"]
            class_dist[label] = class_dist.get(label, 0) + 1
        logger.info("Class distribution (top 10):")
        sorted_classes = sorted(class_dist.items(), key=lambda x: x[1], reverse=True)
        for label, count in sorted_classes[:10]:
            logger.info("  %s: %d items", label, count)


def _save_pseudo_labels(output_path: Path, pseudo_labeled: list[dict], raw_count: int) -> None:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", encoding="utf-8") as f:
        for item in pseudo_labeled:
            f.write(json.dumps(item, ensure_ascii=False) + "\n")

    logger.info("Generated %d pseudo-labels", len(pseudo_labeled))
    logger.info("Ratio: %.1f%%", (len(pseudo_labeled) / raw_count) * 100)
    logger.info("Saved to: %s", output_path)


def main() -> None:
    args = _parse_args()
    device = _resolve_device()

    # 1. Load Taxonomy
    taxonomy_path = Path("recap_subworker/learning_machine/taxonomy/genres.yaml")
    genres = load_genres(taxonomy_path)
    id2label = dict(enumerate(genres))
    num_labels = len(genres)

    # 2. Load Model
    model = _load_teacher_model(args.model_dir, num_labels, device)
    if model is None:
        return

    # 3. Load Data
    raw_items = _load_and_filter_articles(args)
    if not raw_items:
        logger.warning("No articles found after language filtering.")
        return

    # 4. Inference
    candidate_items, confidence_stats, margin_stats = _run_inference(
        model, raw_items, id2label, args, device
    )
    if not confidence_stats:
        logger.error("No confidence statistics collected!")
        return

    conf_array = np.array(confidence_stats)
    margin_array = np.array(margin_stats)
    percentiles = {
        "p50": float(np.percentile(conf_array, 50)),
        "p75": float(np.percentile(conf_array, 75)),
        "p90": float(np.percentile(conf_array, 90)),
        "p95": float(np.percentile(conf_array, 95)),
        "p99": float(np.percentile(conf_array, 99)),
    }

    selected_indices, selection_method, effective_threshold = _select_candidates(
        candidate_items, conf_array, margin_array, args
    )
    pseudo_labeled = _build_pseudo_labeled_items(candidate_items, selected_indices, args.language)

    _log_pseudo_label_statistics(
        _PseudoLabelStats(
            conf_array=conf_array,
            margin_array=margin_array,
            percentiles=percentiles,
            selection_method=selection_method,
            effective_threshold=effective_threshold,
            pseudo_labeled=pseudo_labeled,
            candidate_items=candidate_items,
        )
    )

    if args.stats_only:
        logger.info("Stats-only mode: exiting without saving pseudo-labels")
        return

    # 5. Save
    _save_pseudo_labels(Path(args.output_path), pseudo_labeled, len(raw_items))


if __name__ == "__main__":
    main()
