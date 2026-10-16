// Package api exposes the payment Service over HTTP using only net/http.
// Handlers do no business logic: each one parses the request, calls the
// Service, and renders the Result (or error) as JSON. All state decisions
// remain in the domain core and the app service.
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Arjun7114/payflow/internal/app"
	"github.com/Arjun7114/payflow/internal/domain"
	"github.com/Arjun7114/payflow/internal/store"
)

// Server wires the Service to HTTP routes.
type Server struct {
	svc *app.Service
}

// NewServer builds a Server over the given service.
func NewServer(svc *app.Service) *Server {
	return &Server{svc: svc}
}

// Routes returns the HTTP handler with all payment routes registered. The
// stdlib mux (Go 1.22+) matches method and path pattern directly.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments", s.handleCreate)
	mux.HandleFunc("POST /payments/{id}/authorize", s.command(domain.CmdAuthorize))
	mux.HandleFunc("POST /payments/{id}/capture", s.command(domain.CmdCapture))
	mux.HandleFunc("POST /payments/{id}/settle", s.command(domain.CmdSettle))
	mux.HandleFunc("POST /payments/{id}/refund", s.command(domain.CmdRefund))
	mux.HandleFunc("POST /payments/{id}/expire", s.command(domain.CmdExpire))
	mux.HandleFunc("POST /payments/{id}/flag-hold", s.command(domain.CmdFlagHold))
	mux.HandleFunc("POST /payments/{id}/clear-hold", s.command(domain.CmdClearHold))
	mux.HandleFunc("GET /payments/{id}", s.handleStatus)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	return mux
}

// paymentResponse is the JSON shape returned for a payment's current state.
type paymentResponse struct {
	PaymentID string `json:"payment_id"`
	State     string `json:"state"`
	Event     string `json:"event,omitempty"`
	Replayed  bool   `json:"replayed,omitempty"`
}

// createRequest is the body for creating a payment. ID is optional; if empty
// the client should supply one. Idempotency key comes from a header.
type createRequest struct {
	PaymentID string `json:"payment_id"`
}

// idempotencyKey reads the Idempotency-Key header. It is required for every
// state-changing request so retries are safe.
func idempotencyKey(r *http.Request) string {
	return r.Header.Get("Idempotency-Key")
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	key := idempotencyKey(r)
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing Idempotency-Key header")
		return
	}
	var body createRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PaymentID == "" {
		writeError(w, http.StatusBadRequest, "body must be {\"payment_id\": \"...\"}")
		return
	}

	res, err := s.svc.Create(r.Context(), body.PaymentID, key)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, paymentResponse{
		PaymentID: body.PaymentID,
		State:     string(res.State),
		Event:     string(res.Event),
		Replayed:  res.Replayed,
	})
}

// command returns a handler that applies a fixed domain command to the payment
// named in the path. One closure serves every state-changing endpoint.
func (s *Server) command(cmd domain.Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := idempotencyKey(r)
		if key == "" {
			writeError(w, http.StatusBadRequest, "missing Idempotency-Key header")
			return
		}
		id := r.PathValue("id")

		res, err := s.svc.Handle(r.Context(), id, cmd, key)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, paymentResponse{
			PaymentID: id,
			State:     string(res.State),
			Event:     string(res.Event),
			Replayed:  res.Replayed,
		})
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	state, err := s.svc.Status(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, paymentResponse{PaymentID: id, State: string(state)})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeServiceError maps domain/store errors to HTTP status codes. This is the
// one place the transport layer interprets core errors, keeping the mapping in
// a single spot.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrIllegalTransition):
		writeError(w, http.StatusConflict, err.Error()) // 409: not valid from current state
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "payment not found")
	case errors.Is(err, store.ErrConcurrencyConflict):
		writeError(w, http.StatusConflict, "concurrent modification, retry")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
