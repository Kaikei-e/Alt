import pickle
import argparse
import joblib
import json
import time
from pathlib import Path
import numpy as np
import pandas as pd
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report, f1_score, precision_recall_curve
from sklearn.preprocessing import label_binarize
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import TensorDataset, DataLoader

def main():
    parser = argparse.ArgumentParser(description="Train genre classifier (GPU accelerated)")
    parser.add_argument("--data_dir", type=str, default="data/dataset", help="Directory containing pickle files")
    parser.add_argument("--output_model", type=str, default="data/genre_classifier.joblib", help="Output model path")
    parser.add_argument("--output_thresholds", type=str, default="data/genre_thresholds.json", help="Output thresholds path")
    parser.add_argument("--epochs", type=int, default=100, help="Number of epochs")
    parser.add_argument("--batch_size", type=int, default=1024, help="Batch size")
    parser.add_argument("--lr", type=float, default=0.01, help="Learning rate")
    parser.add_argument("--weight_decay", type=float, default=0.0, help="L2 Regularization")
    parser.add_argument("--loss_type", type=str, default="bce", choices=["bce", "focal"], help="Loss function")
    parser.add_argument("--scheduler", type=str, default="none", choices=["none", "onecycle"], help="LR Scheduler")
    parser.add_argument("--label_smoothing", type=float, default=0.0, help="Label smoothing epsilon")
    parser.add_argument("--no_class_weights", action="store_true", help="Disable class weights")
    args = parser.parse_args()

    data_dir = Path(args.data_dir)
    print(f"Loading datasets from {data_dir}...")

    # Load data
    with open(data_dir / "dataset_train.pkl", "rb") as f:
        X_train, y_train = pickle.load(f)

    with open(data_dir / "dataset_valid.pkl", "rb") as f:
        X_valid, y_valid = pickle.load(f)

    with open(data_dir / "dataset_test.pkl", "rb") as f:
        X_test, y_test = pickle.load(f)

    print(f"Train shape: {X_train.shape}, Labels: {len(set(y_train))}")

    # Encode labels to integers
    classes = sorted(list(set(y_train)))
    class_to_idx = {cls: i for i, cls in enumerate(classes)}
    idx_to_class = {i: cls for i, cls in enumerate(classes)}

    y_train_idx = np.array([class_to_idx[y] for y in y_train])
    y_valid_idx = np.array([class_to_idx[y] for y in y_valid])
    y_test_idx = np.array([class_to_idx[y] for y in y_test])

    # Convert to Tensors
    # Handle Sparse Matrix if needed
    if hasattr(X_train, "toarray"):
        print("Converting sparse matrix to dense for GPU training (ensure memory is sufficient)...")
        # For very large sparse data, we might need a sparse tensor or batch-wise conversion.
        # Given 60k x 6000 float32 ~ 1.4GB, it should fit in RAM and GPU VRAM.
        X_train_tensor = torch.tensor(X_train.toarray(), dtype=torch.float32)
        X_valid_tensor = torch.tensor(X_valid.toarray(), dtype=torch.float32)
        X_test_tensor = torch.tensor(X_test.toarray(), dtype=torch.float32)
    else:
        X_train_tensor = torch.tensor(X_train, dtype=torch.float32)
        X_valid_tensor = torch.tensor(X_valid, dtype=torch.float32)
        X_test_tensor = torch.tensor(X_test, dtype=torch.float32)

    y_train_tensor = torch.tensor(y_train_idx, dtype=torch.long)
    y_valid_tensor = torch.tensor(y_valid_idx, dtype=torch.long)

    # Move to GPU
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    print(f"Using device: {device}")

    X_train_tensor = X_train_tensor.to(device)
    y_train_tensor = y_train_tensor.to(device)
    X_valid_tensor = X_valid_tensor.to(device)
    y_valid_tensor = y_valid_tensor.to(device)

    # Create DataLoader
    train_dataset = TensorDataset(X_train_tensor, y_train_tensor)
    train_loader = DataLoader(train_dataset, batch_size=args.batch_size, shuffle=True)

    # Define Model (Standard Logistic Regression is linear layer + CrossEntropy/BCE)
    # Sklearn's LogisticRegression(multi_class='ovr') fits N binary classifiers.
    # To mimic this exactly, we can use BCEWithLogitsLoss with one-hot targets,
    # or just use CrossEntropyLoss if we want multinomial (Softmax).
    # BUT, the "refine_plan" emphasized OVR and independent probabilities.
    # Also, "genre_thresholds.json" optimization relies on independent probabilities.
    # So we should likely train using BCEWithLogitsLoss (Multi-label style logic, or OVR).
    # Even if targets are single-class, OVR is fine.

    input_dim = X_train.shape[1]
    num_classes = len(classes)

    # Calculate Class Weights for Imbalance Handling
    # pos_weight for BCEWithLogitsLoss should be (num_neg / num_pos) for each class
    # or just inverse frequency.
    pos_weight_tensor = None
    if not args.no_class_weights:
        print("Calculating class weights...")
        class_counts = np.bincount(y_train_idx, minlength=num_classes)
        total_samples = len(y_train_idx)
        pos_weights = []
        for count in class_counts:
            if count > 0:
                weight = (total_samples - count) / count
            else:
                weight = 1.0
            pos_weights.append(weight)
        pos_weight_tensor = torch.tensor(pos_weights, dtype=torch.float32).to(device)
        print(f"Class weights enabled (min/max): {min(pos_weights):.2f} / {max(pos_weights):.2f}")
    else:
        print("Class weights disabled.")

    # Focal Loss Implementation
    class FocalLoss(nn.Module):
        def __init__(self, alpha=None, gamma=2.0, reduction='mean'):
            super(FocalLoss, self).__init__()
            self.alpha = alpha # Alpha can be pos_weight
            self.gamma = gamma
            self.reduction = reduction
            self.bce = nn.BCEWithLogitsLoss(pos_weight=alpha, reduction='none')

        def forward(self, inputs, targets):
            bce_loss = self.bce(inputs, targets)
            pt = torch.exp(-bce_loss) # prevent numerical instability
            focal_loss = ((1 - pt) ** self.gamma) * bce_loss
            if self.reduction == 'mean':
                return focal_loss.mean()
            return focal_loss.sum()

    # Linear layer: input -> num_classes logits
    model = nn.Linear(input_dim, num_classes)
    model.to(device)

    # Loss Selection
    if args.loss_type == "focal":
        print(f"Using Focal Loss (gamma=2.0)")
        criterion = FocalLoss(alpha=pos_weight_tensor, gamma=2.0)
    else:
        print(f"Using BCE With Logits Loss")
        criterion = nn.BCEWithLogitsLoss(pos_weight=pos_weight_tensor)

    optimizer = optim.Adam(model.parameters(), lr=args.lr, weight_decay=args.weight_decay)

    scheduler = None
    if args.scheduler == "onecycle":
        print("Using OneCycleLR Scheduler")
        scheduler = optim.lr_scheduler.OneCycleLR(
            optimizer,
            max_lr=args.lr,
            epochs=args.epochs,
            steps_per_epoch=len(train_loader)
        )

    print("Starting training...")
    start_time = time.time()

    for epoch in range(args.epochs):
        model.train()
        total_loss = 0

        for batch_X, batch_y in train_loader:
            optimizer.zero_grad()

            # One-hot encode targets for BCE
            # batch_y is (Batch,) indices
            # Need (Batch, NumClasses)
            one_hot_targets = torch.zeros(batch_X.size(0), num_classes, device=device)
            one_hot_targets.scatter_(1, batch_y.unsqueeze(1), 1.0)

            outputs = model(batch_X)
            loss = criterion(outputs, one_hot_targets)

            loss.backward()
            optimizer.step()

            total_loss += loss.item()

        avg_loss = total_loss / len(train_loader)

        # Validation
        model.eval()
        with torch.no_grad():
             # OVR validation
             val_outputs = model(X_valid_tensor)
             # BCE loss
             val_one_hot = torch.zeros(X_valid_tensor.size(0), num_classes, device=device)
             val_one_hot.scatter_(1, y_valid_tensor.unsqueeze(1), 1.0)
             val_loss = criterion(val_outputs, val_one_hot).item()

             # Argmax F1 equivalent check
             val_preds = torch.argmax(val_outputs, dim=1)
             val_f1 = f1_score(y_valid_idx, val_preds.cpu().numpy(), average='macro')

        if (epoch + 1) % 10 == 0 or epoch == 0:
            print(f"Epoch {epoch+1}/{args.epochs} | Loss: {avg_loss:.4f} | Val Loss: {val_loss:.4f} | Val Macro F1: {val_f1:.4f}")

    print(f"Training finished in {time.time() - start_time:.2f}s")

    # Export Weights to Sklearn
    print("Exporting model to Sklearn LogisticRegression format...")
    model.eval()

    # Sklearn expected attributes for LogisticRegression
    # coef_: (n_classes, n_features)
    # intercept_: (n_classes,)
    # classes_: array of class labels

    weights = model.weight.data.cpu().numpy() # (n_classes, n_features)
    bias = model.bias.data.cpu().numpy()      # (n_classes,)

    sklearn_model = LogisticRegression(solver='liblinear')
    # Use standard attributes
    sklearn_model.classes_ = np.array(classes)
    sklearn_model.coef_ = weights
    sklearn_model.intercept_ = bias

    # Hack to allow using predict/predict_proba without calling fit
    sklearn_model.n_iter_ = np.array([args.epochs])

    # Verify export on Test Set
    X_test_cpu = X_test_tensor.cpu().numpy()
    sk_preds = sklearn_model.predict(X_test_cpu)

    # Calculate Test F1
    test_f1 = f1_score(y_test, sk_preds, average='macro')
    print(f"Exported Sklearn Model Test Macro F1: {test_f1:.4f}")

    # Save Model
    output_model_path = Path(args.output_model)
    output_model_path.parent.mkdir(parents=True, exist_ok=True)
    joblib.dump(sklearn_model, output_model_path)
    print(f"Model saved to {output_model_path}")

    # Optimizing Thresholds (Reused logic)
    print("Optimizing Thresholds on Validation Set...")
    best_model = sklearn_model
    X_valid_cpu = X_valid_tensor.cpu().numpy()

    y_valid_proba = best_model.predict_proba(X_valid_cpu)
    y_valid_bin = label_binarize(y_valid, classes=best_model.classes_)

    thresholds_map = {}
    n_classes = len(best_model.classes_)

    for i, class_label in enumerate(best_model.classes_):
        if n_classes == 2:
             y_true_col = y_valid_bin[:, 0] if i == 1 else 1 - y_valid_bin[:, 0]
             y_score_col = y_valid_proba[:, i]
        else:
             y_true_col = y_valid_bin[:, i]
             y_score_col = y_valid_proba[:, i]

        if np.sum(y_true_col) == 0:
            thresholds_map[class_label] = 0.5
            continue

        precisions, recalls, thresholds = precision_recall_curve(y_true_col, y_score_col)
        f1_scores = 2 * (precisions * recalls) / (precisions + recalls + 1e-10)
        best_idx = np.argmax(f1_scores)

        if best_idx < len(thresholds):
            best_thresh = thresholds[best_idx]
        else:
            best_thresh = 0.5

        thresholds_map[class_label] = float(best_thresh)
        print(f"Class: {class_label}, Best Threshold: {best_thresh:.4f}")

    output_thresholds_path = Path(args.output_thresholds)
    with open(output_thresholds_path, 'w') as f:
        json.dump(thresholds_map, f, indent=2)
    print(f"Thresholds saved to {output_thresholds_path}")

    # Final Report
    print("\n--- Standard ArgMax Evaluation on Test ---")
    print(classification_report(y_test, sk_preds))

if __name__ == "__main__":
    main()
