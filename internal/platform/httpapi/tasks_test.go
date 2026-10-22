package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
	"example.com/stock-analysis-flow/internal/platform/taskqueue"
)

type fakeTaskService struct {
	task       taskqueue.Task
	events     []taskqueue.Event
	createErr  error
	getErr     error
	created    bool
	gotRequest domain.RunRequest
	gotKey     string
	gotAfter   int64
}

func (f *fakeTaskService) Create(_ context.Context, req domain.RunRequest, key string) (taskqueue.Task, bool, error) {
	f.gotRequest, f.gotKey = req, key
	return f.task, f.created, f.createErr
}
func (f *fakeTaskService) Get(_ context.Context, id string) (taskqueue.Task, error) {
	if f.getErr != nil {
		return taskqueue.Task{}, f.getErr
	}
	if id != f.task.TaskID {
		return taskqueue.Task{}, taskqueue.ErrTaskNotFound
	}
	return f.task, nil
}
func (f *fakeTaskService) Events(_ context.Context, id string, after int64, limit int) ([]taskqueue.Event, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if id != f.task.TaskID {
		return nil, taskqueue.ErrTaskNotFound
	}
	f.gotAfter = after
	out := make([]taskqueue.Event, 0)
	for _, event := range f.events {
		if event.Sequence > after && len(out) < limit {
			out = append(out, event)
		}
	}
	return out, nil
}

func TestCreateTaskReturnsAcceptedAndIdempotencyKey(t *testing.T) {
	task := taskqueue.Task{TaskID: "task-1", RunID: "run-1", Status: "queued", TaskType: "post_market_scan", CreatedAt: time.Now().UTC()}
	tasks := &fakeTaskService{task: task, created: true}
	api, err := NewWithTasks(&fakeRunner{}, &fakeStore{}, time.Second, tasks)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/tasks", strings.NewReader(`{"symbols":["600519"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "client-request-1")
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tasks.gotKey != "client-request-1" {
		t.Fatalf("idempotency key not forwarded: %q", tasks.gotKey)
	}
	if tasks.gotRequest.TaskType != domain.TaskPostMarketScan || len(tasks.gotRequest.Symbols) != 1 {
		t.Fatalf("defaults/request not prepared: %+v", tasks.gotRequest)
	}
	if w.Header().Get("Location") != "/api/v1/analysis/tasks/task-1" {
		t.Fatalf("unexpected Location: %q", w.Header().Get("Location"))
	}
}

func TestCreateTaskRejectsHistoricalRequestWithoutAsOf(t *testing.T) {
	tasks := &fakeTaskService{task: taskqueue.Task{TaskID: "task-1"}, created: true}
	api, err := NewWithTasks(&fakeRunner{}, &fakeStore{}, time.Second, tasks)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(domain.RunRequest{TaskType: domain.TaskHistoricalRun, Symbols: []string{"600519"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/tasks", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tasks.gotRequest.TaskType != "" {
		t.Fatal("task was persisted despite invalid historical request")
	}
}

func TestGetTaskEventsSupportsIncrementalSequence(t *testing.T) {
	tasks := &fakeTaskService{
		task: taskqueue.Task{TaskID: "task-1", RunID: "run-1", Status: "running"},
		events: []taskqueue.Event{
			{Sequence: 4, Event: domain.StageEvent{RunID: "run-1", Stage: "collect_data"}},
			{Sequence: 7, Event: domain.StageEvent{RunID: "run-1", Stage: "quality_gate"}},
		},
	}
	api, err := NewWithTasks(&fakeRunner{}, &fakeStore{}, time.Second, tasks)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/tasks/task-1/events?after_sequence=4", nil)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tasks.gotAfter != 4 {
		t.Fatalf("after_sequence=%d", tasks.gotAfter)
	}
	var payload struct {
		Events []taskqueue.Event `json:"events"`
		Next   int64             `json:"next_after_sequence"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Events) != 1 || payload.Events[0].Sequence != 7 || payload.Next != 7 {
		t.Fatalf("unexpected incremental events: %+v", payload)
	}
}

func TestTaskQueueFullReturns429(t *testing.T) {
	tasks := &fakeTaskService{createErr: taskqueue.ErrQueueFull}
	api, err := NewWithTasks(&fakeRunner{}, &fakeStore{}, time.Second, tasks)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/tasks", strings.NewReader(`{"symbols":["600519"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUnknownTaskReturns404(t *testing.T) {
	tasks := &fakeTaskService{getErr: taskqueue.ErrTaskNotFound}
	api, err := NewWithTasks(&fakeRunner{}, &fakeStore{}, time.Second, tasks)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/tasks/missing", nil)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIdempotencyConflictReturns409(t *testing.T) {
	tasks := &fakeTaskService{createErr: taskqueue.ErrIdempotency}
	api, err := NewWithTasks(&fakeRunner{}, &fakeStore{}, time.Second, tasks)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/tasks", strings.NewReader(`{"symbols":["600519"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !errors.Is(tasks.createErr, taskqueue.ErrIdempotency) {
		t.Fatal("unexpected fake error")
	}
}
