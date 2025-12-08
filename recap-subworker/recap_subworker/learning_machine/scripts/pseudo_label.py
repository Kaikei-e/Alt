import argparse
import json
import logging
import sys
import yaml
from pathlib import Path
from typing import List, Dict

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
    parser.add_argument("--threshold", type=float, default=0.9, help="Confidence threshold")
    parser.add_argument("--model_dir", type=str, default="recap_subworker/learning_machine/artifacts/teacher/v0")
    parser.add_argument("--input_path", type=str, default="recap_subworker/learning_machine/data/raw_articles.jsonl")
    parser.add_argument("--output_path", type=str, default="recap_subworker/learning_machine/data/silver_teacher_v0.jsonl")
    args = parser.parse_args()

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    logger.info(f"Using device: {device}")

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
    with open(args.input_path, "r", encoding="utf-8") as f:
        for line in f:
            if line.strip():
                raw_items.append(json.loads(line))

    logger.info(f"Loaded {len(raw_items)} raw articles.")
    if not raw_items:
        return

    # 4. Inference
    texts = [item.get("content", "") for item in raw_items]
    dataset = InferenceDataset(texts, model.tokenizer)
    loader = DataLoader(dataset, batch_size=args.batch_size, shuffle=False)

    pseudo_labeled = []
    all_probs = []

    logger.info("Running inference...")
    with torch.no_grad():
        for i, batch in enumerate(tqdm(loader)):
            input_ids = batch['input_ids'].to(device)
            attention_mask = batch['attention_mask'].to(device)

            outputs = model(input_ids, attention_mask)
            logits = outputs.logits
            probs = torch.softmax(logits, dim=-1)

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

                # Check threshold
                if max_prob >= args.threshold:
                    item = raw_items[idx].copy()
                    item["label"] = pred_label
                    item["confidence"] = max_prob
                    item["source"] = "teacher_v0_pseudo"
                    # Ideally store full distribution or logits for distillation?
                    # For simple pseudo-labeling, hard label is often used.
                    # Plan Phase 8 Distillation says "soft loss... KL".
                    # If we want soft labels, we need to store them.
                    # Storing 30 floats per item is fine.
                    item["logits"] = logits[j].cpu().tolist()
                    pseudo_labeled.append(item)

    # 5. Save
    output_path = Path(args.output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    with open(output_path, "w", encoding="utf-8") as f:
        for item in pseudo_labeled:
            f.write(json.dumps(item, ensure_ascii=False) + "\n")

    logger.info(f"Generated {len(pseudo_labeled)} pseudo-labels (Threshold >= {args.threshold}).")
    logger.info(f"Ratio: {len(pseudo_labeled)/len(raw_items):.1%}")

if __name__ == "__main__":
    main()
