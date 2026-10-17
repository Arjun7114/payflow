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
    legit_amount = rng.gamma(shape=2.0, scale=40.0, size=n_legit)           # ~ most under 200
    legit_hour = rng.normal(loc=14.0, scale=4.0, size=n_legit) % 24          # daytime-centered
    legit_velocity = rng.poisson(lam=1.5, size=n_legit).astype(float)        # few per window
    legit_ratio = rng.normal(loc=1.0, scale=0.3, size=n_legit).clip(0.1)     # near account average

    legit = pd.DataFrame({
        "amount": legit_amount,
        "hour": legit_hour,
        "velocity": legit_velocity,
        "amount_to_account_avg": legit_ratio,
        "is_fraud": 0,
    })

    # --- Fraudulent transactions ------------------------------------------
    # A mix of high-value fraud and tiny card-testing probes.
    high_value = rng.gamma(shape=3.0, scale=300.0, size=n_fraud)            # large amounts
    probes = rng.uniform(0.5, 3.0, size=n_fraud)                            # tiny test charges
    pick_probe = rng.random(n_fraud) < 0.3
    fraud_amount = np.where(pick_probe, probes, high_value)

    fraud_hour = rng.normal(loc=2.0, scale=2.5, size=n_fraud) % 24           # late-night-centered
    fraud_velocity = rng.poisson(lam=8.0, size=n_fraud).astype(float)       # bursty
    fraud_ratio = rng.normal(loc=6.0, scale=2.0, size=n_fraud).clip(0.1)    # far from account average

    fraud = pd.DataFrame({
        "amount": fraud_amount,
        "hour": fraud_hour,
        "velocity": fraud_velocity,
        "amount_to_account_avg": fraud_ratio,
        "is_fraud": 1,
    })

    df = pd.concat([legit, fraud], ignore_index=True)
    # Shuffle so fraud rows aren't all at the end.
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