# payflow_ai

The advisory AI layer for payflow — kept deliberately separate from the
deterministic payment core.

## Boundary (the important part)

- This service **only reads** payment data.
- The **one** action it can take is calling `POST /payments/{id}/flag-hold` on
  the payflow API — the same guarded endpoint any client uses, which produces an
  audited `authorized -> flagged_hold` transition the Go core validates.
- It **never** writes to the database, moves money, or invents a transition.
  If this service is switched off, payflow runs unaffected.

## Components

- **Fraud scorer** — an Isolation Forest (scikit-learn) trained on *synthetic*
  transaction data. All reported metrics are on synthetic data and demonstrate
  the pipeline, not production performance.
- **Failure triage** — a rules-based explainer (default, no dependencies or API
  keys) that maps a failed payment's history to a plain-English reason. An
  optional LLM-backed explainer can be enabled via environment variable.

## Setup

```cmd
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
```

Run from the `ai/` directory.