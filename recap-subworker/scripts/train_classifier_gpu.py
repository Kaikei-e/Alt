import argparse
import json
import pickle
import sys
import time
from dataclasses import dataclass
from pathlib import Path

import joblib
import numpy as np
import torch
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report, f1_score, precision_recall_curve
from sklearn.preprocessing import label_binarize
from torch import nn, optim
from torch.utils.data import DataLoader, TensorDataset

BINARY_CLASS_COUNT = 2
LOG_EPOCH_INTERVAL = 10


@dataclass(frozen=True)
class TrainingData:
    """Prepared training and validation tensors."""

    train_loader: DataLoader
    x_valid_tensor: torch.Tensor
    y_valid_tensor: torch.Tensor
    y_valid_idx: np.ndarray
    num_classes: int
    device: torch.device
    pos_weight_tensor: torch.Tensor | None


class FocalLoss(nn.Module):
    """Focal loss for multi-label/OVR classification."""

    def __init__(
        self,
        alpha: torch.Tensor | None = None,
        gamma: float = 2.0,
        reduction: str = "mean",
    ) -> None:
        super().__init__()
        self.alpha = alpha
        self.gamma = gamma
        self.reduction = reduction
        self.bce = nn.BCEWithLogitsLoss(pos_weight=alpha, reduction="none")

    def forward(self, inputs: torch.Tensor, targets: torch.Tensor) -> torch.Tensor:
        bce_loss = self.bce(inputs, targets)
        pt = torch.exp(-bce_loss)
        focal_loss = ((1 - pt) ** self.gamma) * bce_loss
        if self.reduction == "mean":
            return focal_loss.mean()
        return focal_loss.sum()


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Train genre classifier (GPU accelerated)")
    parser.add_argument(
        "--data_dir", type=str, default="data/dataset", help="Directory containing pickle files"
    )
    parser.add_argument(
        "--output_model", type=str, default="data/genre_classifier.joblib", help="Output model path"
    )
    parser.add_argument(
        "--output_thresholds",
        type=str,
        default="data/genre_thresholds.json",
        help="Output thresholds path",
    )
    parser.add_argument("--epochs", type=int, default=100, help="Number of epochs")
    parser.add_argument("--batch_size", type=int, default=1024, help="Batch size")
    parser.add_argument("--lr", type=float, default=0.01, help="Learning rate")
    parser.add_argument("--weight_decay", type=float, default=0.0, help="L2 Regularization")
    parser.add_argument(
        "--loss_type", type=str, default="bce", choices=["bce", "focal"], help="Loss function"
    )
    parser.add_argument(
        "--scheduler", type=str, default="none", choices=["none", "onecycle"], help="LR Scheduler"
    )
    parser.add_argument(
        "--label_smoothing", type=float, default=0.0, help="Label smoothing epsilon"
    )
    parser.add_argument("--no_class_weights", action="store_true", help="Disable class weights")
    return parser.parse_args()


def _to_float_tensor(matrix: np.ndarray) -> torch.Tensor:
    if hasattr(matrix, "toarray"):
        return torch.tensor(matrix.toarray(), dtype=torch.float32)
    return torch.tensor(matrix, dtype=torch.float32)


def _compute_pos_weights(
    y_train_idx: np.ndarray,
    num_classes: int,
    device: torch.device,
) -> torch.Tensor:
    sys.stdout.write("Calculating class weights...\n")
    class_counts = np.bincount(y_train_idx, minlength=num_classes)
    total_samples = len(y_train_idx)
    pos_weights = [(total_samples - count) / count if count > 0 else 1.0 for count in class_counts]
    sys.stdout.write(
        f"Class weights enabled (min/max): {min(pos_weights):.2f} / {max(pos_weights):.2f}\n"
    )
    return torch.tensor(pos_weights, dtype=torch.float32).to(device)


def _train_model(
    model: nn.Linear,
    data: TrainingData,
    args: argparse.Namespace,
) -> None:
    if args.loss_type == "focal":
        sys.stdout.write("Using Focal Loss (gamma=2.0)\n")
        criterion: nn.Module = FocalLoss(alpha=data.pos_weight_tensor, gamma=2.0)
    else:
        sys.stdout.write("Using BCE With Logits Loss\n")
        criterion = nn.BCEWithLogitsLoss(pos_weight=data.pos_weight_tensor)

    optimizer = optim.Adam(model.parameters(), lr=args.lr, weight_decay=args.weight_decay)

    if args.scheduler == "onecycle":
        sys.stdout.write("Using OneCycleLR Scheduler\n")
        optim.lr_scheduler.OneCycleLR(
            optimizer, max_lr=args.lr, epochs=args.epochs, steps_per_epoch=len(data.train_loader)
        )

    sys.stdout.write("Starting training...\n")
    start_time = time.time()

    for epoch in range(args.epochs):
        model.train()
        total_loss = 0.0

        for batch_x, batch_y in data.train_loader:
            optimizer.zero_grad()
            one_hot_targets = torch.zeros(batch_x.size(0), data.num_classes, device=data.device)
            one_hot_targets.scatter_(1, batch_y.unsqueeze(1), 1.0)
            outputs = model(batch_x)
            loss = criterion(outputs, one_hot_targets)
            loss.backward()
            optimizer.step()
            total_loss += loss.item()

        avg_loss = total_loss / len(data.train_loader)

        model.eval()
        with torch.no_grad():
            val_outputs = model(data.x_valid_tensor)
            val_one_hot = torch.zeros(
                data.x_valid_tensor.size(0), data.num_classes, device=data.device
            )
            val_one_hot.scatter_(1, data.y_valid_tensor.unsqueeze(1), 1.0)
            val_loss = criterion(val_outputs, val_one_hot).item()
            val_preds = torch.argmax(val_outputs, dim=1)
            val_f1 = f1_score(data.y_valid_idx, val_preds.cpu().numpy(), average="macro")

        if (epoch + 1) % LOG_EPOCH_INTERVAL == 0 or epoch == 0:
            sys.stdout.write(
                f"Epoch {epoch + 1}/{args.epochs} | Loss: {avg_loss:.4f} | Val Loss: {val_loss:.4f} | Val Macro F1: {val_f1:.4f}\n"
            )

    sys.stdout.write(f"Training finished in {time.time() - start_time:.2f}s\n")


def _load_split_datasets(
    data_dir: Path,
) -> tuple[np.ndarray, np.ndarray, np.ndarray, np.ndarray, np.ndarray, np.ndarray]:
    with (data_dir / "dataset_train.pkl").open("rb") as f:
        x_train, y_train = pickle.load(f)  # noqa: S301 - loads local dataset pickle generated by prepare_dataset.py

    with (data_dir / "dataset_valid.pkl").open("rb") as f:
        x_valid, y_valid = pickle.load(f)  # noqa: S301 - loads local dataset pickle generated by prepare_dataset.py

    with (data_dir / "dataset_test.pkl").open("rb") as f:
        x_test, y_test = pickle.load(f)  # noqa: S301 - loads local dataset pickle generated by prepare_dataset.py

    return x_train, y_train, x_valid, y_valid, x_test, y_test


def _export_and_evaluate(
    model: nn.Linear,
    classes: list[str],
    x_test_tensor: torch.Tensor,
    y_test: np.ndarray,
    args: argparse.Namespace,
) -> tuple[LogisticRegression, np.ndarray]:
    sys.stdout.write("Exporting model to Sklearn LogisticRegression format...\n")
    model.eval()
    sklearn_model = LogisticRegression(solver="liblinear")
    sklearn_model.classes_ = np.array(classes)
    sklearn_model.coef_ = model.weight.data.cpu().numpy()
    sklearn_model.intercept_ = model.bias.data.cpu().numpy()
    sklearn_model.n_iter_ = np.array([args.epochs])

    x_test_cpu = x_test_tensor.cpu().numpy()
    sk_preds = sklearn_model.predict(x_test_cpu)
    test_f1 = f1_score(y_test, sk_preds, average="macro")
    sys.stdout.write(f"Exported Sklearn Model Test Macro F1: {test_f1:.4f}\n")

    output_model_path = Path(args.output_model)
    output_model_path.parent.mkdir(parents=True, exist_ok=True)
    joblib.dump(sklearn_model, output_model_path)
    sys.stdout.write(f"Model saved to {output_model_path}\n")
    return sklearn_model, sk_preds


def _optimize_and_save_thresholds(
    sklearn_model: LogisticRegression,
    x_valid_tensor: torch.Tensor,
    y_valid: np.ndarray,
    output_thresholds: str,
) -> None:
    sys.stdout.write("Optimizing Thresholds on Validation Set...\n")
    x_valid_cpu = x_valid_tensor.cpu().numpy()

    y_valid_proba = sklearn_model.predict_proba(x_valid_cpu)
    y_valid_bin = label_binarize(y_valid, classes=sklearn_model.classes_)

    thresholds_map: dict[str, float] = {}
    n_classes = len(sklearn_model.classes_)

    for i, class_label in enumerate(sklearn_model.classes_):
        if n_classes == BINARY_CLASS_COUNT:
            y_true_col = y_valid_bin[:, 0] if i == 1 else 1 - y_valid_bin[:, 0]
        else:
            y_true_col = y_valid_bin[:, i]
        y_score_col = y_valid_proba[:, i]

        if np.sum(y_true_col) == 0:
            thresholds_map[class_label] = 0.5
            continue

        precisions, recalls, thresholds = precision_recall_curve(y_true_col, y_score_col)
        f1_scores = 2 * (precisions * recalls) / (precisions + recalls + 1e-10)
        best_idx = np.argmax(f1_scores)
        best_thresh = thresholds[best_idx] if best_idx < len(thresholds) else 0.5

        thresholds_map[class_label] = float(best_thresh)
        sys.stdout.write(f"Class: {class_label}, Best Threshold: {best_thresh:.4f}\n")

    output_thresholds_path = Path(output_thresholds)
    with output_thresholds_path.open("w") as f:
        json.dump(thresholds_map, f, indent=2)
    sys.stdout.write(f"Thresholds saved to {output_thresholds_path}\n")


def main() -> None:
    args = _parse_args()
    data_dir = Path(args.data_dir)
    sys.stdout.write(f"Loading datasets from {data_dir}...\n")
    x_train, y_train, x_valid, y_valid, x_test, y_test = _load_split_datasets(data_dir)
    sys.stdout.write(f"Train shape: {x_train.shape}, Labels: {len(set(y_train))}\n")

    classes = sorted(set(y_train))
    class_to_idx = {cls: i for i, cls in enumerate(classes)}
    y_train_idx = np.array([class_to_idx[y] for y in y_train])
    y_valid_idx = np.array([class_to_idx[y] for y in y_valid])

    if hasattr(x_train, "toarray"):
        sys.stdout.write(
            "Converting sparse matrix to dense for GPU training (ensure memory is sufficient)...\n"
        )
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    sys.stdout.write(f"Using device: {device}\n")

    x_train_tensor = _to_float_tensor(x_train).to(device)
    y_train_tensor = torch.tensor(y_train_idx, dtype=torch.long).to(device)
    x_valid_tensor = _to_float_tensor(x_valid).to(device)
    y_valid_tensor = torch.tensor(y_valid_idx, dtype=torch.long).to(device)
    x_test_tensor = _to_float_tensor(x_test)

    train_loader = DataLoader(
        TensorDataset(x_train_tensor, y_train_tensor), batch_size=args.batch_size, shuffle=True
    )
    num_classes = len(classes)
    pos_weight_tensor = (
        None if args.no_class_weights else _compute_pos_weights(y_train_idx, num_classes, device)
    )
    if args.no_class_weights:
        sys.stdout.write("Class weights disabled.\n")

    model = nn.Linear(x_train.shape[1], num_classes).to(device)
    training_data = TrainingData(
        train_loader=train_loader,
        x_valid_tensor=x_valid_tensor,
        y_valid_tensor=y_valid_tensor,
        y_valid_idx=y_valid_idx,
        num_classes=num_classes,
        device=device,
        pos_weight_tensor=pos_weight_tensor,
    )
    _train_model(model, training_data, args)

    sklearn_model, sk_preds = _export_and_evaluate(model, classes, x_test_tensor, y_test, args)
    _optimize_and_save_thresholds(sklearn_model, x_valid_tensor, y_valid, args.output_thresholds)

    sys.stdout.write("\n--- Standard ArgMax Evaluation on Test ---\n")
    sys.stdout.write(f"{classification_report(y_test, sk_preds)}\n")


if __name__ == "__main__":
    main()
