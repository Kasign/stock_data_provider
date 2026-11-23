package bootstrap

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

type researchflowHTTPHandler struct {
	authenticator ownerSessionRequestAuthenticator
	cookieName    string
	composition   *researchflowOwnerComposition
}

type researchflowRunResponse struct {
	SchemaVersion        string                       `json:"schema_version"`
	RunRef               string                       `json:"run_ref"`
	Status               researchflowdomain.RunStatus `json:"status"`
	Version              uint64                       `json:"version"`
	SystemAsOf           time.Time                    `json:"system_as_of"`
	DeadlineAt           time.Time                    `json:"deadline_at"`
	TemplateRef          string                       `json:"template_ref"`
	TemplateVersion      string                       `json:"template_version"`
	TemplateFingerprint  string                       `json:"template_fingerprint"`
	InputSnapshotRefs    []string                     `json:"input_snapshot_refs"`
	ResultSummaryRef     *string                      `json:"result_summary_ref"`
	OriginatingRequestID *string                      `json:"originating_request_id"`
	CorrelationID        string                       `json:"correlation_id"`
	TerminationReason    *string                      `json:"termination_reason"`
	CreatedAt            time.Time                    `json:"created_at"`
	StartedAt            *time.Time                   `json:"started_at"`
	FinishedAt           *time.Time                   `json:"finished_at"`
}

type researchflowStageResponse struct {
	SchemaVersion     string                         `json:"schema_version"`
	StageRef          string                         `json:"stage_ref"`
	RunRef            string                         `json:"run_ref"`
	StageCode         string                         `json:"stage_code"`
	Status            researchflowdomain.StageStatus `json:"status"`
	Version           uint64                         `json:"version"`
	Required          bool                           `json:"required"`
	Dependencies      []string                       `json:"dependency_stage_refs"`
	RetryPolicyRef    string                         `json:"retry_policy_ref"`
	RetryAt           *time.Time                     `json:"retry_at"`
	OutputRef         *string                        `json:"output_ref"`
	ReasonCodes       []string                       `json:"reason_codes"`
	CreatedAt         time.Time                      `json:"created_at"`
	StartedAt         *time.Time                     `json:"started_at"`
	FinishedAt        *time.Time                     `json:"finished_at"`
	QualitySummaryRef *string                        `json:"quality_summary_ref"`
	DiagnosticRefs    []string                       `json:"diagnostic_refs"`
}

type researchflowAggregateResponse struct {
	SchemaVersion string                            `json:"schema_version"`
	Run           researchflowRunResponse           `json:"run"`
	Stages        []researchflowStageResponse       `json:"stages"`
	ResultSummary *researchflowdomain.ResultSummary `json:"result_summary"`
}

func newResearchflowHTTPHandler(owner *ownerSessionComposition) (http.Handler, error) {
	if owner == nil || owner.authenticator == nil || owner.researchflow == nil ||
		owner.researchflow.createRun == nil || owner.researchflow.getRun == nil || owner.researchflow.cancelRun == nil {
		return nil, errors.New("Owner Session and Researchflow Run use cases are required")
	}
	handler := &researchflowHTTPHandler{
		authenticator: owner.authenticator,
		cookieName:    owner.cookie.Name,
		composition:   owner.researchflow,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/researchflow/runs", handler.create)
	mux.HandleFunc("GET /api/v1/researchflow/runs/{run_ref}", handler.get)
	mux.HandleFunc("POST /api/v1/researchflow/runs/{run_ref}/cancel", handler.cancel)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticated, ok := handler.authenticate(w, r)
		if !ok {
			return
		}
		mux.ServeHTTP(w, authenticated)
	}), nil
}

func (h *researchflowHTTPHandler) authenticate(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if len(r.Header.Values("Authorization")) > 0 {
		status, code := http.StatusUnauthorized, "server.session_unauthenticated"
		if hasCookie(r, h.cookieName) {
			status, code = http.StatusBadRequest, "server.ambiguous_authentication"
		}
		writePublicError(w, r, status, publicError{
			Class: "unauthenticated", Code: code,
			Message: "Researchflow API requires Owner Session authentication only",
		})
		return nil, false
	}
	authenticated, cookie, err := h.authenticator.Authenticate(r)
	if err != nil {
		if !writePlatformFault(w, r, err) {
			writePublicError(w, r, http.StatusInternalServerError, publicError{
				Class: "internal", Code: "server.session_authentication_failed",
				Message: "authentication is unavailable",
			})
		}
		return nil, false
	}
	if authenticated == nil {
		writePublicError(w, r, http.StatusInternalServerError, publicError{
			Class: "internal", Code: "server.session_authentication_failed",
			Message: "authentication is unavailable",
		})
		return nil, false
	}
	principal, ok := platformsecurity.PrincipalFromContextValue(authenticated.Context())
	if !ok || principal.Kind != platformsecurity.IdentityOwner ||
		principal.AuthMethod != platformsecurity.AuthMethodOwnerSession || principal.Subject != "owner" {
		writePublicError(w, r, http.StatusForbidden, publicError{
			Class: "forbidden", Code: "server.researchflow_owner_session_required",
			Message: "Researchflow API requires a verified Owner Session",
		})
		return nil, false
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	return authenticated, true
}

func (h *researchflowHTTPHandler) create(w http.ResponseWriter, r *http.Request) {
	var command researchflowdomain.CreateRunCommand
	if err := decodeOwnerJSON(r, &command); err != nil {
		writePublicError(w, r, http.StatusBadRequest, publicError{
			Class: "validation", Code: "server.invalid_researchflow_create_request",
			Message: "Researchflow CreateRun request is invalid",
		})
		return
	}
	result, err := h.composition.createRun.CreateRun(r.Context(), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeAggregate(w, http.StatusOK, result.Run, result.Stages)
}

func (h *researchflowHTTPHandler) get(w http.ResponseWriter, r *http.Request) {
	result, err := h.composition.getRun.GetRun(r.Context(), r.PathValue("run_ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeAggregate(w, http.StatusOK, result.Run, result.Stages)
}

func (h *researchflowHTTPHandler) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil || len(strings.TrimSpace(string(body))) != 0 {
			writePublicError(w, r, http.StatusBadRequest, publicError{
				Class: "validation", Code: "server.invalid_researchflow_cancel_request",
				Message: "Researchflow cancellation does not accept a request body",
			})
			return
		}
	}
	result, err := h.composition.cancelRun.CancelRun(r.Context(), r.PathValue("run_ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeAggregate(w, http.StatusOK, result.Run, result.Stages)
}

func (h *researchflowHTTPHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, researchflowapplication.ErrUnauthenticated):
		writePublicError(w, r, http.StatusUnauthorized, publicError{
			Class: "unauthenticated", Code: "server.session_unauthenticated",
			Message: "authentication required",
		})
	case errors.Is(err, researchflowapplication.ErrRunAccessDenied):
		writePublicError(w, r, http.StatusForbidden, publicError{
			Class: "forbidden", Code: "researchflow.run_access_denied",
			Message: "Researchflow Run access is denied",
		})
	case errors.Is(err, researchflowdomain.ErrInvalidCommand):
		writePublicError(w, r, http.StatusBadRequest, publicError{
			Class: "validation", Code: "researchflow.invalid_run_command",
			Message: "Researchflow Run request is invalid",
		})
	case errors.Is(err, researchflowapplication.ErrIdempotencyConflict):
		writePublicError(w, r, http.StatusConflict, publicError{
			Class: "conflict", Code: "researchflow.idempotency_conflict",
			Message: "idempotency key was already used for a different request",
		})
	case errors.Is(err, researchflowapplication.ErrRunVersionConflict),
		errors.Is(err, researchflowapplication.ErrRunCancellationDenied),
		errors.Is(err, researchflowapplication.ErrRunAlreadyTerminal),
		errors.Is(err, researchflowapplication.ErrRunCancellationUnavailable):
		writePublicError(w, r, http.StatusConflict, publicError{
			Class: "conflict", Code: "researchflow.run_state_conflict",
			Message: "Researchflow Run cannot be changed in its current state",
		})
	case errors.Is(err, sql.ErrNoRows):
		writePublicError(w, r, http.StatusNotFound, publicError{
			Class: "not_found", Code: "researchflow.run_not_found",
			Message: "Researchflow Run was not found",
		})
	default:
		if !writePlatformFault(w, r, err) {
			writePublicError(w, r, http.StatusInternalServerError, publicError{
				Class: "internal", Code: "server.researchflow_request_failed",
				Message: "Researchflow request failed",
			})
		}
	}
}

func (h *researchflowHTTPHandler) writeAggregate(
	w http.ResponseWriter,
	status int,
	run researchflowdomain.Run,
	stages []researchflowdomain.Stage,
) {
	response := researchflowAggregateResponse{
		SchemaVersion: "1.0.0",
		Run: researchflowRunResponse{
			SchemaVersion:        run.SchemaVersion,
			RunRef:               run.Ref,
			Status:               run.Status,
			Version:              run.Version,
			SystemAsOf:           run.SystemAsOf,
			DeadlineAt:           run.DeadlineAt,
			TemplateRef:          run.TemplateRef,
			TemplateVersion:      run.TemplateVersion,
			TemplateFingerprint:  run.TemplateFingerprint,
			InputSnapshotRefs:    append([]string{}, run.InputSnapshotRefs...),
			ResultSummaryRef:     cloneResearchflowString(run.ResultSummaryRef),
			OriginatingRequestID: cloneResearchflowString(run.OriginatingRequestID),
			CorrelationID:        run.CorrelationID,
			TerminationReason:    nonEmptyResearchflowString(run.TerminationReason),
			CreatedAt:            run.CreatedAt,
			StartedAt:            run.StartedAt,
			FinishedAt:           run.FinishedAt,
		},
		Stages:        make([]researchflowStageResponse, 0, len(stages)),
		ResultSummary: run.ResultSummary,
	}
	for _, stage := range stages {
		response.Stages = append(response.Stages, researchflowStageResponse{
			SchemaVersion:     "1.0.0",
			StageRef:          stage.Ref,
			RunRef:            stage.RunRef,
			StageCode:         stage.Code,
			Status:            stage.Status,
			Version:           stage.Version,
			Required:          stage.Required,
			Dependencies:      append([]string{}, stage.Dependencies...),
			RetryPolicyRef:    stage.RetryPolicyRef,
			RetryAt:           stage.RetryAt,
			OutputRef:         cloneResearchflowString(stage.OutputRef),
			ReasonCodes:       append([]string{}, stage.ReasonCodes...),
			CreatedAt:         stage.CreatedAt,
			StartedAt:         stage.StartedAt,
			FinishedAt:        stage.FinishedAt,
			QualitySummaryRef: cloneResearchflowString(stage.QualitySummaryRef),
			DiagnosticRefs:    append([]string{}, stage.DiagnosticRefs...),
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func cloneResearchflowString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nonEmptyResearchflowString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copy := value
	return &copy
}
