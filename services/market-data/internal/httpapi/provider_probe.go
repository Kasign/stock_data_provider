package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

type providerProbeRequest struct {
	Scope          string   `json:"scope"`
	Capabilities   []string `json:"capabilities"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

func (s *Server) dataSourceProbe(w http.ResponseWriter, r *http.Request) {
	request := providerProbeRequest{Scope: "all", TimeoutSeconds: 12}
	if r.Body != nil {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil && err != io.EOF {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "request body must be valid JSON and no larger than 8 KB", requestID(r))
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "request body must contain exactly one JSON object", requestID(r))
			return
		}
	}
	if request.Scope == "" {
		request.Scope = "all"
	}
	if request.Scope != "all" && request.Scope != "core" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "scope must be all or core", requestID(r))
		return
	}
	if request.TimeoutSeconds == 0 {
		request.TimeoutSeconds = 12
	}
	if request.TimeoutSeconds < 1 || request.TimeoutSeconds > 30 {
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
	if len(request.Capabilities) == 0 && request.Scope == "all" {
		// 空列表表示使用 scope 默认能力集合。
	} else if len(request.Capabilities) == 0 && request.Scope == "core" {
		// core scope 本身提供默认能力集合。
	}
	run, err := s.svc.ProbeProviderCapabilitiesWithOptions(r.Context(), service.ProviderProbeOptions{
		Scope: request.Scope, Capabilities: request.Capabilities, TimeoutSeconds: request.TimeoutSeconds,
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
