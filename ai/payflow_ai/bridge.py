"""Fraud bridge: score a transaction, and if it is risky enough, ask payflow to
place a hold — through the audited HTTP API, never by touching the ledger.

This is the ONLY component that acts on the payment core, and it acts only by
calling POST /payments/{id}/flag-hold, the same endpoint any client uses. That
produces an 'authorized -> flagged_hold' transition the Go core validates and
records. The bridge cannot move money, skip states, or write to the database.
If payflow rejects the hold (e.g. the payment is not in 'authorized' state),
the bridge surfaces that and does nothing further.

Operating threshold is a deliberate, recall-leaning choice (see threshold.py):
catch more fraud at the cost of more false holds, because in payments a missed
fraud is a realized loss while a false hold is reviewable.
"""

from __future__ import annotations

import argparse
import uuid

import joblib
import numpy as np
import requests

from payflow_ai.generate import FEATURE_COLUMNS

# Recall-leaning operating point chosen from the threshold sweep. Scores at or
# above this are treated as high-risk. Higher score = more anomalous.
DEFAULT_THRESHOLD = -0.041


def load_model(path: str = "data/fraud_model.pkl"):
    bundle = joblib.load(path)
    return bundle["model"], bundle["features"]


def risk_score(model, features: list[str], txn: dict) -> float:
    """Return the anomaly score for one transaction dict. Higher = riskier."""
    row = np.array([[txn[f] for f in features]], dtype=float)
    # Negate decision_function so higher means more fraud-like, matching how the
    # threshold was chosen in the sweep.
    return float(-model.decision_function(row)[0])


def evaluate_and_maybe_hold(
    txn: dict,
    payment_id: str,
    base_url: str,
    model,
    features: list[str],
    threshold: float = DEFAULT_THRESHOLD,
    timeout: float = 5.0,
) -> dict:
    """Score txn; if risky, request a hold via the payflow API. Returns a report
    describing what was decided and what the API said. Side effect is at most one
    POST to /flag-hold."""
    score = risk_score(model, features, txn)
    decision = {"payment_id": payment_id, "score": round(score, 4),
                "threshold": threshold, "flagged": score >= threshold}

    if not decision["flagged"]:
        decision["action"] = "none (below threshold)"
        return decision

    # High risk: ask payflow to hold. We do NOT mutate anything ourselves.
    url = f"{base_url.rstrip('/')}/payments/{payment_id}/flag-hold"
    key = f"fraud-hold-{uuid.uuid4()}"  # idempotency key for this hold request
    try:
        resp = requests.post(url, headers={"Idempotency-Key": key}, timeout=timeout)
    except requests.RequestException as e:
        decision["action"] = "hold requested"
        decision["api_error"] = str(e)
        return decision

    decision["action"] = "hold requested"
    decision["api_status"] = resp.status_code
    try:
        decision["api_response"] = resp.json()
    except ValueError:
        decision["api_response"] = resp.text
    # 200 => hold placed; 409 => payment not in a holdable state (core refused).
    return decision


def main() -> None:
    parser = argparse.ArgumentParser(description="Score a transaction and maybe hold it.")
    parser.add_argument("--payment-id", required=True)
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--model", default="data/fraud_model.pkl")
    parser.add_argument("--threshold", type=float, default=DEFAULT_THRESHOLD)
    # Transaction features (default to a clearly-risky example).
    parser.add_argument("--amount", type=float, default=900.0)
    parser.add_argument("--hour", type=float, default=3.0)
    parser.add_argument("--velocity", type=float, default=12.0)
    parser.add_argument("--ratio", type=float, default=8.0,
                        help="amount_to_account_avg")
    args = parser.parse_args()

    model, features = load_model(args.model)
    txn = {
        "amount": args.amount,
        "hour": args.hour,
        "velocity": args.velocity,
        "amount_to_account_avg": args.ratio,
    }
    report = evaluate_and_maybe_hold(
        txn, args.payment_id, args.base_url, model, features, args.threshold
    )
    import json
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()