package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Arjun7114/payflow/internal/app"
	"github.com/Arjun7114/payflow/internal/store"
)

func newTestServer() http.Handler {
	svc := app.New(store.NewMemory(), nil)
	return NewServer(svc).Routes()
}

// post sends a POST with an idempotency key and returns status + decoded body.
func post(t *testing.T, h http.Handler, path, key, body string) (int, paymentResponse) {
	t.Helper()
	req := httptest.NewRequest("POST", path, bytes.NewReader([]byte(body)))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var pr paymentResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &pr)
	return rec.Code, pr
}

// Full happy path over HTTP: create -> authorize -> capture -> settle.
func TestAPI_HappyPath(t *testing.T) {
	h := newTestServer()

	if code, _ := post(t, h, "/payments", "k1", `{"payment_id":"p1"}`); code != http.StatusCreated {
		t.Fatalf("create: got %d", code)
	}
	if code, pr := post(t, h, "/payments/p1/authorize", "k2", ""); code != http.StatusOK || pr.State != "authorized" {
		t.Fatalf("authorize: code=%d state=%q", code, pr.State)
	}
	if code, pr := post(t, h, "/payments/p1/capture", "k3", ""); code != http.StatusOK || pr.State != "captured" {
		t.Fatalf("capture: code=%d state=%q", code, pr.State)
	}
	if code, pr := post(t, h, "/payments/p1/settle", "k4", ""); code != http.StatusOK || pr.State != "settled" {
		t.Fatalf("settle: code=%d state=%q", code, pr.State)
	}
}

// An illegal transition over HTTP returns 409 Conflict.
func TestAPI_IllegalTransitionIs409(t *testing.T) {
	h := newTestServer()
	post(t, h, "/payments", "k1", `{"payment_id":"p1"}`)
	// Capture straight from initiated (skips authorize) is illegal.
	code, _ := post(t, h, "/payments/p1/capture", "k2", "")
	if code != http.StatusConflict {
		t.Fatalf("got %d, want 409", code)
	}
}

// A state-changing request without an idempotency key is rejected.
func TestAPI_MissingKeyIs400(t *testing.T) {
	h := newTestServer()
	code, _ := post(t, h, "/payments", "", `{"payment_id":"p1"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", code)
	}
}

// Retrying a capture with the SAME key over HTTP is idempotent (replayed).
func TestAPI_SameKeyReplayed(t *testing.T) {
	h := newTestServer()
	post(t, h, "/payments", "k1", `{"payment_id":"p1"}`)
	post(t, h, "/payments/p1/authorize", "k2", "")
	post(t, h, "/payments/p1/capture", "k3", "")

	code, pr := post(t, h, "/payments/p1/capture", "k3", "") // same key again
	if code != http.StatusOK || !pr.Replayed || pr.State != "captured" {
		t.Fatalf("replay: code=%d replayed=%v state=%q", code, pr.Replayed, pr.State)
	}
}

// Status of an unknown payment returns 404.
func TestAPI_StatusNotFound(t *testing.T) {
	h := newTestServer()
	req := httptest.NewRequest("GET", "/payments/ghost", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}
