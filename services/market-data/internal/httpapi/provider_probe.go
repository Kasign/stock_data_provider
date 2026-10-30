package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

type providerProbeRequest struct {
	Scope          string   `json:"scope"`
	Capabilities   []string `json:"capabilities"`
	TimeoutSeconds *int     `json:"timeout_seconds"`
}

func (s *Server) dataSourceProbe(w http.ResponseWriter, r *http.Request) {
	request := providerProbeRequest{Scope: "all"}
	if r.Body != nil {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil && err != io.EOF {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "request body must be valid JSON and no larger than 8 KB", requestID(r))
			return
		}
		if len(raw) > 0 {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || trimmed[0] != '{' {
				writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "request body must be a JSON object", requestID(r))
				return
			}
			if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
				writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "request body must contain exactly one JSON object", requestID(r))
				return
			}
			requestDecoder := json.NewDecoder(bytes.NewReader(raw))
			requestDecoder.DisallowUnknownFields()
			if err := requestDecoder.Decode(&request); err != nil {
				writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "request body contains unsupported fields or invalid values", requestID(r))
				return
			}
		}
	}
	if request.Scope == "" {
		request.Scope = "all"
	}
	if request.Scope != "all" && request.Scope != "core" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "scope must be all or core", requestID(r))
		return
	}
	timeoutSeconds := 12
	if request.TimeoutSeconds != nil {
		timeoutSeconds = *request.TimeoutSeconds
	}
	if timeoutSeconds < 1 || timeoutSeconds > 30 {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "timeout_seconds must be between 1 and 30", requestID(r))
		return
	}
	known := make(map[string]bool)
	for _, capability := range model.ProviderProbeCapabilities() {
		known[capability] = true
	}
	seen := make(map[string]bool, len(request.Capabilities))
	for _, capability := range request.Capabilities {
		if !known[capability] {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "unsupported capability: "+capability, requestID(r))
			return
		}
		if seen[capability] {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "capabilities must not contain duplicates", requestID(r))
			return
		}
		seen[capability] = true
	}
	if request.Scope == "core" {
		core := make(map[string]bool)
		for _, capability := range model.ProviderProbeCoreCapabilities() {
			core[capability] = true
		}
		for _, capability := range request.Capabilities {
			if !core[capability] {
				writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "capability is outside core scope: "+capability, requestID(r))
				return
			}
		}
	}
	run, err := s.svc.ProbeProviderCapabilitiesWithOptions(r.Context(), service.ProviderProbeOptions{
		Scope: request.Scope, Capabilities: request.Capabilities, TimeoutSeconds: timeoutSeconds,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) dataSourceProbeHistory(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be between 1 and 100", requestID(r))
			return
		}
		limit = value
	}
	history, err := s.svc.ProviderProbeHistory(r.Context(), limit)
	if err != nil {
		if strings.Contains(err.Error(), "storage is not enabled") {
			writeErr(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "provider-probe history storage is not enabled", requestID(r))
			return
		}
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, history)
}
