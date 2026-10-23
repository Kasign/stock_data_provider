// Package taskqueue 提供基于 MariaDB 持久化任务表的有界分析 Worker。
package taskqueue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

var (
	ErrTaskNotFound = errors.New("analysis task not found")
	ErrQueueFull    = errors.New("analysis task queue is full")
	ErrIdempotency  = errors.New("idempotency key conflicts with a different request")
)

// Runner 与同步分析流程共用同一个 Runner，避免复制业务逻辑。
type Runner interface {
	Run(context.Context, domain.RunRequest) (domain.RunResult, error)
}

type Task struct {
	TaskID          string     `json:"task_id"`
	RunID           string     `json:"run_id"`
	Status          string     `json:"status"`
	TaskType        string     `json:"task_type"`
	CurrentStage    string     `json:"current_stage,omitempty"`
	CompletedStages int        `json:"completed_stages"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	FailureCode     string     `json:"failure_code,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
}

type Event struct {
	Sequence int64             `json:"sequence"`
	Event    domain.StageEvent `json:"event"`
}

type Queue struct {
	db        *sql.DB
	runner    Runner
	workers   int
	maxQueued int
	timeout   time.Duration
	pollEvery time.Duration
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	closeOnce sync.Once
	createMu  sync.Mutex
}

func New(db *sql.DB, runner Runner, workers, maxQueued int, timeout, pollEvery time.Duration) (*Queue, error) {
	if db == nil || runner == nil {
		return nil, errors.New("task queue requires a database and analysis runner")
	}
	if workers < 1 {
		workers = 2
	}
	if workers > 32 {
		workers = 32
	}
	if maxQueued < workers {
		maxQueued = workers * 10
	}
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	if pollEvery <= 0 {
		pollEvery = 250 * time.Millisecond
	}
	return &Queue{db: db, runner: runner, workers: workers, maxQueued: maxQueued, timeout: timeout, pollEvery: pollEvery}, nil
}

// Start 会恢复仍在队列中的任务；进程异常退出时正在执行的任务会被明确标记为失败，
// 避免状态永久停留在 running。已写入的分析阶段事件仍保留供排查。
func (q *Queue) Start(parent context.Context) error {
	var startErr error
	q.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		q.cancel = cancel
		tx, err := q.db.BeginTx(ctx, nil)
		if err != nil {
			startErr = err
			cancel()
			return
		}
		// 将中断的 run 一并终结，避免旧同步查询接口永远返回 run_not_finished。
		_, startErr = tx.ExecContext(ctx, `UPDATE analysis_runs r JOIN analysis_tasks t ON t.run_id=r.run_id
			SET r.status='failed', r.finished_at=UTC_TIMESTAMP(6),
			r.result_json=JSON_OBJECT('run_id', r.run_id, 'status', 'failed',
				'request', JSON_EXTRACT(t.request_json, '$'),
				'failure', JSON_OBJECT('code', 'worker_interrupted', 'stage', 'worker_recovery', 'message', 'Worker process restarted while task was running'))
			WHERE t.status='running'`)
		if startErr == nil {
			_, startErr = tx.ExecContext(ctx, `UPDATE analysis_tasks SET status='failed', failure_code='worker_interrupted', error_message='Worker process restarted while task was running', finished_at=UTC_TIMESTAMP(6), updated_at=UTC_TIMESTAMP(6) WHERE status='running'`)
		}
		if startErr == nil {
			startErr = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if startErr != nil {
			cancel()
			return
		}
		for i := 0; i < q.workers; i++ {
			q.wg.Add(1)
			go q.worker(ctx, i+1)
		}
		slog.Info("异步分析 Worker 已启动", "component", "taskqueue", "workers", q.workers, "max_queued", q.maxQueued)
	})
	return startErr
}

func (q *Queue) Close() {
	q.closeOnce.Do(func() {
		if q.cancel != nil {
			q.cancel()
		}
		q.wg.Wait()
	})
}

func (q *Queue) Create(ctx context.Context, req domain.RunRequest, idempotencyKey string) (Task, bool, error) {
	q.createMu.Lock()
	defer q.createMu.Unlock()
	var zero Task
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 128 {
		return zero, false, errors.New("Idempotency-Key must be at most 128 characters")
	}
	if idempotencyKey != "" {
		existing, storedRequest, err := q.findByIdempotencyKey(ctx, idempotencyKey)
		if err == nil {
			if !sameRequest(storedRequest, req) {
				return zero, false, ErrIdempotency
			}
			current, getErr := q.Get(ctx, existing.TaskID)
			if getErr != nil {
				return zero, false, getErr
			}
			return current, false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return zero, false, err
		}
	}
	var queued int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM analysis_tasks WHERE status='queued'`).Scan(&queued); err != nil {
		return zero, false, fmt.Errorf("count queued analysis tasks: %w", err)
	}
	if queued >= q.maxQueued {
		return zero, false, ErrQueueFull
	}
	taskID, err := newID()
	if err != nil {
		return zero, false, err
	}
	runID, err := newID()
	if err != nil {
		return zero, false, err
	}
	req.RunID = runID
	raw, err := json.Marshal(req)
	if err != nil {
		return zero, false, fmt.Errorf("encode analysis task request: %w", err)
	}
	now := time.Now().UTC()
	_, err = q.db.ExecContext(ctx, `INSERT INTO analysis_tasks (task_id, run_id, status, task_type, request_json, idempotency_key, created_at, updated_at) VALUES (?, ?, 'queued', ?, ?, NULLIF(?, ''), ?, ?)`, taskID, runID, req.TaskType, string(raw), idempotencyKey, now, now)
	if err != nil {
		if idempotencyKey != "" {
			existing, storedRequest, findErr := q.findByIdempotencyKey(ctx, idempotencyKey)
			if findErr == nil && sameRequest(storedRequest, req) {
				current, getErr := q.Get(ctx, existing.TaskID)
				if getErr == nil {
					return current, false, nil
				}
			}
		}
		return zero, false, fmt.Errorf("persist queued analysis task: %w", err)
	}
	return Task{TaskID: taskID, RunID: runID, Status: "queued", TaskType: string(req.TaskType), CreatedAt: now}, true, nil
}

func (q *Queue) Get(ctx context.Context, taskID string) (Task, error) {
	var t Task
	var started, finished sql.NullTime
	var failureCode, errorMessage, currentStage sql.NullString
	err := q.db.QueryRowContext(ctx, `SELECT t.task_id,t.run_id,t.status,t.task_type,t.created_at,t.started_at,t.finished_at,t.failure_code,t.error_message,
		(SELECT e.stage FROM analysis_stage_events e WHERE e.run_id=t.run_id ORDER BY e.id DESC LIMIT 1),
		(SELECT COUNT(DISTINCT e.stage) FROM analysis_stage_events e WHERE e.run_id=t.run_id AND e.status IN ('succeeded','skipped'))
		FROM analysis_tasks t WHERE t.task_id=?`, taskID).Scan(&t.TaskID, &t.RunID, &t.Status, &t.TaskType, &t.CreatedAt, &started, &finished, &failureCode, &errorMessage, &currentStage, &t.CompletedStages)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrTaskNotFound
	}
	if err != nil {
		return t, fmt.Errorf("read analysis task: %w", err)
	}
	if started.Valid {
		v := started.Time.UTC()
		t.StartedAt = &v
	}
	if finished.Valid {
		v := finished.Time.UTC()
		t.FinishedAt = &v
	}
	if failureCode.Valid {
		t.FailureCode = failureCode.String
	}
	if errorMessage.Valid {
		t.ErrorMessage = errorMessage.String
	}
	if currentStage.Valid {
		t.CurrentStage = currentStage.String
	}
	return t, nil
}

func (q *Queue) Events(ctx context.Context, taskID string, after int64, limit int) ([]Event, error) {
	if _, err := q.Get(ctx, taskID); err != nil {
		return nil, err
	}
	if after < 0 {
		return nil, errors.New("after_sequence must be non-negative")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var runID string
	if err := q.db.QueryRowContext(ctx, `SELECT run_id FROM analysis_tasks WHERE task_id=?`, taskID).Scan(&runID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	rows, err := q.db.QueryContext(ctx, `SELECT id,event_json FROM analysis_stage_events WHERE run_id=? AND id>? ORDER BY id ASC LIMIT ?`, runID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("query task events: %w", err)
	}
	defer rows.Close()
	out := make([]Event, 0)
	for rows.Next() {
		var sequence int64
		var raw string
		if err := rows.Scan(&sequence, &raw); err != nil {
			return nil, err
		}
		var event domain.StageEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("decode task event: %w", err)
		}
		out = append(out, Event{Sequence: sequence, Event: event})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queue) findByIdempotencyKey(ctx context.Context, key string) (Task, domain.RunRequest, error) {
	var t Task
	var raw string
	err := q.db.QueryRowContext(ctx, `SELECT task_id,run_id,status,task_type,request_json,created_at FROM analysis_tasks WHERE idempotency_key=?`, key).Scan(&t.TaskID, &t.RunID, &t.Status, &t.TaskType, &raw, &t.CreatedAt)
	if err != nil {
		return t, domain.RunRequest{}, err
	}
	var req domain.RunRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return t, req, err
	}
	return t, req, nil
}

func sameRequest(a, b domain.RunRequest) bool {
	a.RunID = ""
	b.RunID = ""
	if a.AsOfMode == domain.AsOfModeRealtimeSnapshot && b.AsOfMode == domain.AsOfModeRealtimeSnapshot {
		a.AsOf = time.Time{}
		b.AsOf = time.Time{}
	}
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}

func (q *Queue) worker(ctx context.Context, workerID int) {
	defer q.wg.Done()
	ticker := time.NewTicker(q.pollEvery)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		claimed, err := q.claim(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("领取分析任务失败", "component", "taskqueue", "worker_id", workerID, "error", err.Error())
		} else if claimed != nil {
			q.execute(ctx, *claimed, workerID)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type claimedTask struct {
	taskID  string
	runID   string
	request domain.RunRequest
}

func (q *Queue) claim(ctx context.Context) (*claimedTask, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var taskID, runID, raw string
	err = tx.QueryRowContext(ctx, `SELECT task_id,run_id,request_json FROM analysis_tasks WHERE status='queued' ORDER BY created_at,task_id LIMIT 1 FOR UPDATE`).Scan(&taskID, &runID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE analysis_tasks SET status='running',started_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6),failure_code=NULL,error_message=NULL WHERE task_id=? AND status='queued'`, taskID)
	if err != nil {
		return nil, err
	}
	if affected, err := res.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var req domain.RunRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		_ = q.finish(context.Background(), taskID, "failed", "invalid_task_payload", "Persisted task request could not be decoded")
		return nil, nil
	}
	req.RunID = runID
	return &claimedTask{taskID: taskID, runID: runID, request: req}, nil
}

func (q *Queue) execute(parent context.Context, task claimedTask, workerID int) {
	ctx, cancel := context.WithTimeout(parent, q.timeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("异步分析 Worker 捕获到任务 panic", "component", "taskqueue", "task_id", task.taskID, "run_id", task.runID, "panic_type", fmt.Sprintf("%T", recovered))
			persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer persistCancel()
			query := "UPDATE analysis_runs r JOIN analysis_tasks t ON t.run_id=r.run_id " +
				"SET r.status='failed', r.finished_at=UTC_TIMESTAMP(6), " +
				"r.result_json=JSON_OBJECT('run_id', r.run_id, 'status', 'failed', " +
				"'request', JSON_EXTRACT(t.request_json, '$'), " +
				"'failure', JSON_OBJECT('code', 'worker_panic', 'stage', 'worker', 'message', 'Analysis worker recovered from an internal panic')) " +
				"WHERE t.task_id=?"
			if _, err := q.db.ExecContext(persistCtx, query, task.taskID); err != nil {
				slog.Error("保存 panic 分析 run 终态失败", "component", "taskqueue", "task_id", task.taskID, "run_id", task.runID, "error", err.Error())
			}
			if err := q.finish(persistCtx, task.taskID, "failed", "worker_panic", "analysis worker recovered from an internal panic"); err != nil {
				slog.Error("保存 panic 任务终态失败", "component", "taskqueue", "task_id", task.taskID, "run_id", task.runID, "error", err.Error())
			}
		}
	}()
	started := time.Now()
	slog.Info("异步分析任务开始", "component", "taskqueue", "task_id", task.taskID, "run_id", task.runID, "worker_id", workerID, "task_type", task.request.TaskType)
	result, err := q.runner.Run(ctx, task.request)
	status := "succeeded"
	failureCode, message := "", ""
	if err != nil {
		status, failureCode, message = "failed", "analysis_failed", "analysis execution failed; inspect server logs using task_id and run_id"
	} else {
		status = taskStatusForRun(result.Status)
		if result.Failure != nil {
			failureCode = result.Failure.Code
			message = "analysis did not complete successfully; inspect stage events for details"
		}
	}
	if ctx.Err() != nil && status == "failed" && failureCode == "analysis_failed" {
		failureCode, message = "analysis_timeout_or_cancelled", "Analysis execution exceeded its time budget or worker is shutting down"
	}
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer persistCancel()
	if err := q.finish(persistCtx, task.taskID, status, failureCode, message); err != nil {
		slog.Error("保存异步任务终态失败", "component", "taskqueue", "task_id", task.taskID, "run_id", task.runID, "error", err.Error())
	}
	slog.Info("异步分析任务结束", "component", "taskqueue", "task_id", task.taskID, "run_id", task.runID, "status", status, "duration_ms", time.Since(started).Milliseconds(), "failure_code", failureCode)
}

// taskStatusForRun 将 Run 的终态映射为异步 Task 状态。Run 被拒绝时，Task 不能误报为 succeeded。
func taskStatusForRun(status domain.RunStatus) string {
	switch status {
	case domain.RunStatusPartial:
		return "partial"
	case domain.RunStatusFailed, domain.RunStatusRejected:
		return "failed"
	default:
		return "succeeded"
	}
}

func (q *Queue) finish(ctx context.Context, taskID, status, failureCode, message string) error {
	_, err := q.db.ExecContext(ctx, `UPDATE analysis_tasks SET status=?,failure_code=NULLIF(?,''),error_message=NULLIF(?,''),finished_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE task_id=?`, status, failureCode, safeMessage(message), taskID)
	return err
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate task ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func safeMessage(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' {
			return -1
		}
		return r
	}, value)
	if len(value) > 300 {
		return value[:300]
	}
	return value
}
