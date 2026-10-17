"""Synthetic transaction generator for payflow's fraud scorer.

IMPORTANT: this data is entirely synthetic. It exists to exercise and validate
the fraud-scoring pipeline end to end (generate -> train -> evaluate -> serve).
Any metric computed on it demonstrates the method, not real-world performance.

The generator is seeded, so the dataset is identical on every run and all
downstream metrics are reproducible.

Design: legitimate transactions cluster around ordinary behaviour (moderate
amounts, daytime hours, low velocity). Fraudulent ones are drawn to be
distinguishable along a few axes an unsupervised model can pick up:
  - amount:    unusually high (or tiny "card-testing" probes)
  - hour:      skewed toward late-night
  - velocity:  many transactions from the same account in a short window
These are deliberately simple, documented assumptions — not claims about how
real fraud looks.
"""

from __future__ import annotations

import argparse
import numpy as np
import pandas as pd

# Column order is fixed so the model and the scorer agree on feature layout.
FEATURE_COLUMNS = ["amount", "hour", "velocity", "amount_to_account_avg"]


def generate(n: int = 10_000, fraud_rate: float = 0.02, seed: int = 42) -> pd.DataFrame:
    """Return a DataFrame of n synthetic transactions.

    Columns: the FEATURE_COLUMNS above plus 'is_fraud' (1/0) ground-truth label,
    used only for evaluation — the unsupervised model never sees it in training.
    """
    rng = np.random.default_rng(seed)
    n_fraud = int(round(n * fraud_rate))
    n_legit = n - n_fraud

    # --- Legitimate transactions ------------------------------------------
    # Wider spread, so "normal" already contains big spenders and night owls.
    legit_amount = rng.gamma(shape=2.0, scale=60.0, size=n_legit)
    legit_hour = rng.normal(loc=14.0, scale=6.0, size=n_legit) % 24
    legit_velocity = rng.poisson(lam=2.0, size=n_legit).astype(float)
    legit_ratio = rng.normal(loc=1.0, scale=0.6, size=n_legit).clip(0.1)

    legit = pd.DataFrame({
        "amount": legit_amount,
        "hour": legit_hour,
        "velocity": legit_velocity,
        "amount_to_account_avg": legit_ratio,
        "is_fraud": 0,
    })

    # --- Fraudulent transactions ------------------------------------------
    # Only SHIFTED from normal, not separated. Distributions overlap heavily, so
    # most fraud looks plausibly normal on any single feature — the model must
    # combine weak signals. This is what makes the task realistic (and the
    # metrics believable) rather than trivially perfect.
    fraud_amount = rng.gamma(shape=2.0, scale=95.0, size=n_fraud)            # a bit higher, overlaps
    fraud_hour = rng.normal(loc=9.0, scale=7.0, size=n_fraud) % 24           # mildly skewed, broad
    fraud_velocity = rng.poisson(lam=3.5, size=n_fraud).astype(float)        # somewhat bursty
    fraud_ratio = rng.normal(loc=2.0, scale=1.2, size=n_fraud).clip(0.1)     # elevated, overlaps

    fraud = pd.DataFrame({
        "amount": fraud_amount,
        "hour": fraud_hour,
        "velocity": fraud_velocity,
        "amount_to_account_avg": fraud_ratio,
        "is_fraud": 1,
    })

    df = pd.concat([legit, fraud], ignore_index=True)
    df = df.sample(frac=1.0, random_state=seed).reset_index(drop=True)
    return df


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate synthetic transactions.")
    parser.add_argument("--n", type=int, default=10_000, help="number of transactions")
    parser.add_argument("--fraud-rate", type=float, default=0.02, help="fraction fraudulent")
    parser.add_argument("--seed", type=int, default=42, help="random seed")
    parser.add_argument("--out", default="data/transactions.csv", help="output CSV path")
    args = parser.parse_args()

    df = generate(n=args.n, fraud_rate=args.fraud_rate, seed=args.seed)

    import os
    os.makedirs(os.path.dirname(args.out), exist_ok=True)
    df.to_csv(args.out, index=False)

    n_fraud = int(df["is_fraud"].sum())
    print(f"wrote {len(df)} transactions to {args.out} "
          f"({n_fraud} fraud, {len(df) - n_fraud} legit, "
          f"fraud_rate={n_fraud / len(df):.3f})")


if __name__ == "__main__":
    main()