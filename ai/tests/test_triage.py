"""Tests for the rules-based triage engine."""

from payflow_ai.triage import triage


def _hist(*steps):
    return [{"from_state": f, "to_state": t, "event_type": e} for f, t, e in steps]


def test_declined_at_authorization():
    h = _hist(("", "initiated", "payment_initiated"),
              ("initiated", "failed", "payment_failed"))
    r = triage("p", h)
    assert r.terminal_state == "failed"
    assert r.severity == "action_required"
    assert "declined" in r.reason.lower()


def test_expired_authorization():
    h = _hist(("", "initiated", "payment_initiated"),
              ("initiated", "authorized", "payment_authorized"),
              ("authorized", "expired", "payment_expired"))
    r = triage("p", h)
    assert r.terminal_state == "expired"
    assert "re-authorize" in r.suggested_action.lower()


def test_non_failure_is_info():
    h = _hist(("", "initiated", "payment_initiated"),
              ("initiated", "authorized", "payment_authorized"),
              ("authorized", "captured", "payment_captured"),
              ("captured", "settled", "payment_settled"))
    r = triage("p", h)
    assert r.severity == "info"


def test_empty_history():
    r = triage("p", [])
    assert r.severity == "warning"