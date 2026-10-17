"""payflow_ai — the advisory AI layer for payflow.

This package NEVER moves money and NEVER mutates the payment ledger. It only
reads payment data and, when a transaction scores as high-risk, requests a hold
through payflow's audited HTTP API (POST /payments/{id}/flag-hold) — the same
endpoint any client uses. The deterministic Go core remains the sole authority
over state transitions.
"""