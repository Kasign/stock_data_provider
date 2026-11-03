package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
	"example.com/stock-analysis-flow/internal/platform/taskqueue"
)

type taskService interface {
	Create(context.Context, domain.RunRequest, string) (taskqueue.Task, bool, error)
	Get(context.Context, string) (taskqueue.Task, error)
	Events(context.Context, string, int64, int) ([]taskqueue.Event, error)
}

// handleCreateTask 在任务写入 MariaDB 后立即返回，不等待分析 Runner 执行完成。
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "async_tasks_unavailable", "asynchronous task worker is not configured")
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content_type_required", "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req domain.RunRequest
	if err := decoder.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 1 MiB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain one JSON object")
		return
	}
	asOfExplicit := !req.AsOf.IsZero()
	fillDefaults(&req)
	if req.TaskType == domain.TaskHistoricalRun && !asOfExplicit {
		writeError(w, http.StatusBadRequest, "as_of_required", "historical replay requires an explicit as_of timestamp")
		return
	}
	if asOfExplicit {
		req.AsOfMode = domain.AsOfModeExplicit
	} else {
		req.AsOfMode = domain.AsOfModeRealtimeSnapshot
	}

	task, created, err := s.tasks.Create(r.Context(), req, r.Header.Get("Idempotency-Key"))
	if err != nil {
		switch {
		case errors.Is(err, taskqueue.ErrQueueFull):
			writeError(w, http.StatusTooManyRequests, "task_queue_full", "task queue is full; retry later")
		case errors.Is(err, taskqueue.ErrIdempotency):
			writeError(w, http.StatusConflict, "idempotency_key_conflict", "Idempotency-Key was already used with a different request")
		default:
			slog.Error("创建异步分析任务失败", "service", "stock-analysis-api", "request_id", requestIDFromContext(r), "error", boundedLogText(err.Error(), 300))
			writeError(w, http.StatusInternalServerError, "create_task_failed", "could not persist analysis task")
		}
		return
	}
	slog.Info("异步分析任务已入队", "service", "stock-analysis-api", "request_id", requestIDFromContext(r), "task_id", task.TaskID, "run_id", task.RunID, "task_type", req.TaskType, "symbols_count", len(req.Symbols), "idempotent_replay", !created)
	w.Header().Set("Location", "/api/v1/analysis/tasks/"+task.TaskID)
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{
		"task":       task,
		"status_url": "/api/v1/analysis/tasks/" + task.TaskID,
		"events_url": "/api/v1/analysis/tasks/" + task.TaskID + "/events",
	})
}

// handleTaskRoute 提供任务状态和按数据库事件 ID 增量读取的阶段进度。
func (s *Server) handleTaskRoute(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "async_tasks_unavailable", "asynchronous task worker is not configured")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/tasks/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if len(parts) == 2 {
		if parts[1] != "events" {
			writeError(w, http.StatusNotFound, "not_found", "route not found")
			return
		}
		after := int64(0)
		if raw := r.URL.Query().Get("after_sequence"); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				writeError(w, http.StatusBadRequest, "invalid_after_sequence", "after_sequence must be a non-negative integer")
				return
			}
			after = value
		}
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 500 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500")
				return
			}
			limit = value
		}
		events, err := s.tasks.Events(ctx, parts[0], after, limit)
		if err != nil {
			s.writeTaskError(w, err)
			return
		}
		next := after
		if len(events) > 0 {
			next = events[len(events)-1].Sequence
		}
		writeJSON(w, http.StatusOK, map[string]any{"task_id": parts[0], "events": events, "next_after_sequence": next})
		return
	}
	task, err := s.tasks.Get(ctx, parts[0])
	if err != nil {
		s.writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (s *Server) writeTaskError(w http.ResponseWriter, err error) {
	if errors.Is(err, taskqueue.ErrTaskNotFound) {
		writeError(w, http.StatusNotFound, "task_not_found", "analysis task not found")
		return
	}
	slog.Error("读取异步分析任务失败", "service", "stock-analysis-api", "error", boundedLogText(err.Error(), 300))
	writeError(w, http.StatusInternalServerError, "read_task_failed", "could not read analysis task")
}
