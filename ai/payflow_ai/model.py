"""Isolation Forest fraud scorer for payflow.

Unsupervised anomaly detection: the model learns the shape of normal
transactions and flags outliers as potential fraud. Ground-truth labels are
used ONLY to evaluate the model, never to train it — this mirrors production,
where labels are scarce and fraud evolves.

All metrics produced here are on SYNTHETIC data (see generate.py), which was
designed to be separable. They validate the pipeline; they are not claims about
real-world fraud-detection performance.
"""

from __future__ import annotations

import argparse
import json
import os

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import IsolationForest
from sklearn.metrics import precision_score, recall_score, f1_score, roc_auc_score
from sklearn.model_selection import train_test_split

from payflow_ai.generate import FEATURE_COLUMNS


def train_and_evaluate(csv_path: str, contamination: float = 0.02, seed: int = 42):
    """Train on the 'normal' structure of a train split, evaluate on a held-out
    test split. Returns (model, metrics_dict).
    """
    df = pd.read_csv(csv_path)
    X = df[FEATURE_COLUMNS].to_numpy()
    y = df["is_fraud"].to_numpy()

    # Stratify so the tiny fraud class is represented in both splits.
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.3, random_state=seed, stratify=y
    )

    # Unsupervised: fit only on features, never on labels. contamination is the
    # model's expected fraction of anomalies (we set it to the known base rate).
    model = IsolationForest(
        n_estimators=200,
        contamination=contamination,
        random_state=seed,
    )
    model.fit(X_train)

    # IsolationForest.predict returns -1 for anomalies, +1 for inliers.
    # Map that to our convention: 1 = predicted fraud, 0 = predicted legit.
    pred_test = (model.predict(X_test) == -1).astype(int)

    # decision_function: lower = more anomalous. Negate so higher = more fraud-like,
    # giving a proper risk score for ROC-AUC.
    scores_test = -model.decision_function(X_test)

    metrics = {
        "n_total": int(len(df)),
        "n_test": int(len(y_test)),
        "test_fraud": int(y_test.sum()),
        "precision": round(float(precision_score(y_test, pred_test, zero_division=0)), 4),
        "recall": round(float(recall_score(y_test, pred_test, zero_division=0)), 4),
        "f1": round(float(f1_score(y_test, pred_test, zero_division=0)), 4),
        "roc_auc": round(float(roc_auc_score(y_test, scores_test)), 4),
        "contamination": contamination,
        "seed": seed,
        "note": "metrics on SYNTHETIC data with realistically overlapping classes; validates pipeline, not production performance",
    }
    return model, metrics


def main() -> None:
    parser = argparse.ArgumentParser(description="Train the fraud scorer.")
    parser.add_argument("--data", default="data/transactions.csv")
    parser.add_argument("--model-out", default="data/fraud_model.pkl")
    parser.add_argument("--metrics-out", default="data/fraud_metrics.json")
    parser.add_argument("--contamination", type=float, default=0.02)
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    model, metrics = train_and_evaluate(args.data, args.contamination, args.seed)

    os.makedirs(os.path.dirname(args.model_out), exist_ok=True)
    joblib.dump({"model": model, "features": FEATURE_COLUMNS}, args.model_out)
    with open(args.metrics_out, "w") as f:
        json.dump(metrics, f, indent=2)

    print("Fraud scorer trained (SYNTHETIC data).")
    for k, v in metrics.items():
        print(f"  {k}: {v}")
    print(f"\nmodel  -> {args.model_out}")
    print(f"metrics -> {args.metrics_out}")


if __name__ == "__main__":
    main()