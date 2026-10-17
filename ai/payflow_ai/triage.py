"""Rules-based failure triage for payflow.

Given a payment's event history, explain in plain English why it reached a
terminal failure state and suggest a next action. This is deliberately a
deterministic rules engine, not an ML model: the set of failure reasons in a
state machine is small and known, so rules are the correct, auditable tool.
(An optional LLM rephrasing layer can be added on top; it would restate these
same facts more naturally, never override them.)

Input is the event history as returned by payflow's GET /payments/{id} plus its
event log — here modelled as a list of (from_state, to_state, event_type) steps,
which is what the Go core records.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class Triage:
    payment_id: str
    terminal_state: str
    reason: str
    suggested_action: str
    severity: str  # "info", "warning", or "action_required"

    def as_dict(self) -> dict:
        return {
            "payment_id": self.payment_id,
            "terminal_state": self.terminal_state,
            "reason": self.reason,
            "suggested_action": self.suggested_action,
            "severity": self.severity,
        }


def _last_non_terminal(history: list[dict]) -> str | None:
    """The state the payment was in just before it failed/expired, if any."""
    if len(history) < 2:
        return None
    # history[-1] is the terminal step; its from_state is where it fell from.
    return history[-1].get("from_state")


def triage(payment_id: str, history: list[dict]) -> Triage:
    """Explain why a payment ended where it did.

    history: ordered list of steps, each {"from_state","to_state","event_type"}.
    """
    if not history:
        return Triage(payment_id, "unknown",
                      "No event history found for this payment.",
                      "Verify the payment ID exists.", "warning")

    terminal = history[-1].get("to_state", "unknown")
    came_from = _last_non_terminal(history)

    # --- Failure: distinguish auth-time failure from capture-time failure ---
    if terminal == "failed":
        if came_from == "initiated":
            return Triage(
                payment_id, terminal,
                "Authorization was declined by the issuer before any funds were held.",
                "Ask the customer to retry with another payment method or contact their bank.",
                "action_required",
            )
        if came_from == "authorized":
            return Triage(
                payment_id, terminal,
                "The payment was authorized but then failed before capture.",
                "Re-initiate the payment; the authorization is no longer usable.",
                "action_required",
            )
        return Triage(
            payment_id, terminal,
            f"The payment failed from the '{came_from}' state.",
            "Review the event log for the failing step.",
            "warning",
        )

    # --- Expiry: an authorization that was never captured in time ---
    if terminal == "expired":
        return Triage(
            payment_id, terminal,
            "The authorization lapsed before the payment was captured.",
            "Re-authorize and capture promptly; authorizations are time-limited.",
            "action_required",
        )

    # --- Flagged hold: risk layer intervened ---
    if terminal == "flagged_hold":
        return Triage(
            payment_id, terminal,
            "The risk layer flagged this payment and placed it on hold.",
            "Send to manual review or step-up verification before clearing the hold.",
            "action_required",
        )

    # --- Not a failure at all ---
    if terminal in ("settled", "captured", "authorized", "initiated", "refunded"):
        return Triage(
            payment_id, terminal,
            f"The payment is in a non-failure state ('{terminal}'); nothing to triage.",
            "No action needed.",
            "info",
        )

    return Triage(
        payment_id, terminal,
        f"Unrecognized terminal state '{terminal}'.",
        "Inspect the event log directly.",
        "warning",
    )


def main() -> None:
    """Demo with a few hand-built histories so the module runs standalone."""
    import json

    examples = {
        "declined-at-auth": [
            {"from_state": "", "to_state": "initiated", "event_type": "payment_initiated"},
            {"from_state": "initiated", "to_state": "failed", "event_type": "payment_failed"},
        ],
        "expired-auth": [
            {"from_state": "", "to_state": "initiated", "event_type": "payment_initiated"},
            {"from_state": "initiated", "to_state": "authorized", "event_type": "payment_authorized"},
            {"from_state": "authorized", "to_state": "expired", "event_type": "payment_expired"},
        ],
        "held": [
            {"from_state": "", "to_state": "initiated", "event_type": "payment_initiated"},
            {"from_state": "initiated", "to_state": "authorized", "event_type": "payment_authorized"},
            {"from_state": "authorized", "to_state": "flagged_hold", "event_type": "payment_hold_flagged"},
        ],
    }
    for pid, hist in examples.items():
        print(json.dumps(triage(pid, hist).as_dict(), indent=2))
        print()


if __name__ == "__main__":
    main()