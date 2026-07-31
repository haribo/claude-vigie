package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
)

// usageLeaseTTL is longer than the fetch cadence so a missed cycle doesn't hand
// the lease around; a killed holder's lease expires after this.
const usageLeaseTTL = 12 * time.Minute

// usageMetaKey is where the latest usage snapshot is stored.
const usageMetaKey = "usage"

func (s *Server) handleUsageLease(w http.ResponseWriter, r *http.Request) {
	var req api.LeaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Holder == "" {
		s.writeError(w, http.StatusBadRequest, "holder is required")
		return
	}
	acquired, expiry, err := s.store.AcquireLease(r.Context(), req.Holder, usageLeaseTTL, s.now())
	if err != nil {
		s.log.Error("acquiring lease", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, api.LeaseResponse{Acquired: acquired, ExpiresAt: expiry})
}

func (s *Server) handlePostUsage(w http.ResponseWriter, r *http.Request) {
	var rep api.UsageReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	blob, err := json.Marshal(rep)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.store.SetMeta(r.Context(), usageMetaKey, string(blob)); err != nil {
		s.log.Error("storing usage", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	v, ok, err := s.store.GetMeta(r.Context(), usageMetaKey)
	if err != nil {
		s.log.Error("reading usage", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var rep api.UsageReport
	if ok {
		if err := json.Unmarshal([]byte(v), &rep); err != nil {
			s.log.Error("decoding usage", "error", err)
		}
	}
	s.writeJSON(w, http.StatusOK, rep)
}
