"""Threshold sweep for the fraud scorer.

The Isolation Forest produces a continuous risk score per transaction; the
decision 'fraud vs not' depends on a threshold. There is no single correct
threshold — it trades precision against recall, and the right point depends on
the relative cost of a missed fraud vs a false hold. This script makes that
tradeoff explicit: it sweeps candidate thresholds and reports precision, recall
and F1 at each, then picks the F1-maximising point as one defensible default.

Metrics are on SYNTHETIC data with realistically overlapping classes; they
illustrate the tradeoff and the method, not real-world performance.
"""

from __future__ import annotations

import argparse
import json
import os

import numpy as np
import pandas as pd
from sklearn.metrics import precision_recall_fscore_support
from sklearn.model_selection import train_test_split
from sklearn.ensemble import IsolationForest

from payflow_ai.generate import FEATURE_COLUMNS


def sweep(csv_path: str, seed: int = 42, steps: int = 19):
    """Train the model, then evaluate precision/recall/F1 across a range of
    score thresholds on the held-out test set. Returns (rows, best_row).
    """
    df = pd.read_csv(csv_path)
    X = df[FEATURE_COLUMNS].to_numpy()
    y = df["is_fraud"].to_numpy()

    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.3, random_state=seed, stratify=y
    )

    model = IsolationForest(n_estimators=200, contamination=0.02, random_state=seed)
    model.fit(X_train)

    # Higher score = more fraud-like (negate decision_function so it points the
    # intuitive way). We threshold THIS score, instead of using the model's
    # built-in -1/+1 cutoff, so we control the operating point.
    scores = -model.decision_function(X_test)

    # Sweep thresholds across the observed score range, skipping the extremes.
    lo, hi = np.quantile(scores, 0.50), np.quantile(scores, 0.999)
    thresholds = np.linspace(lo, hi, steps)

    rows = []
    for thr in thresholds:
        pred = (scores >= thr).astype(int)
        p, r, f1, _ = precision_recall_fscore_support(
            y_test, pred, average="binary", zero_division=0
        )
        flagged = int(pred.sum())
        rows.append({
            "threshold": round(float(thr), 4),
            "precision": round(float(p), 4),
            "recall": round(float(r), 4),
            "f1": round(float(f1), 4),
            "flagged": flagged,
            "flagged_pct": round(100.0 * flagged / len(y_test), 2),
        })

    best = max(rows, key=lambda row: row["f1"])
    return rows, best


def main() -> None:
    parser = argparse.ArgumentParser(description="Sweep fraud-score thresholds.")
    parser.add_argument("--data", default="data/transactions.csv")
    parser.add_argument("--out", default="data/threshold_sweep.json")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    rows, best = sweep(args.data, args.seed)

    print(f"{'threshold':>10} {'precision':>10} {'recall':>8} {'f1':>8} {'flagged%':>9}")
    for row in rows:
        marker = "  <- best F1" if row is best else ""
        print(f"{row['threshold']:>10.4f} {row['precision']:>10.4f} "
              f"{row['recall']:>8.4f} {row['f1']:>8.4f} {row['flagged_pct']:>8.2f}%{marker}")

    print(f"\nChosen operating point (max F1): threshold={best['threshold']}, "
          f"precision={best['precision']}, recall={best['recall']}, f1={best['f1']}")
    print("Note: on synthetic data; the point is the tradeoff, not the absolute numbers.")

    os.makedirs(os.path.dirname(args.out), exist_ok=True)
    with open(args.out, "w") as f:
        json.dump({"sweep": rows, "best_f1": best}, f, indent=2)
    print(f"sweep -> {args.out}")


if __name__ == "__main__":
    main()