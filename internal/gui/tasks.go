package gui

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/course"
	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/progress"
	"github.com/nicoxiang/geektime-downloader/internal/ui"
)

// TaskStatus of a download task
type TaskStatus string

// task statuses
const (
	StatusQueued   TaskStatus = "queued"
	StatusRunning  TaskStatus = "running"
	StatusDone     TaskStatus = "done"
	StatusFailed   TaskStatus = "failed"
	StatusCanceled TaskStatus = "canceled"
	StatusSkipped  TaskStatus = "skipped"
)

func (s TaskStatus) finished() bool {
	return s == StatusDone || s == StatusFailed || s == StatusCanceled || s == StatusSkipped
}

type taskKind int

const (
	kindArticle taskKind = iota // one article of a course
	kindProduct                 // single video product, e.g. 每日一课, 大厂案例
)

// Task is one download job. Exported fields are sent to the browser.
type Task struct {
	ID          int64      `json:"id"`
	CourseID    int        `json:"courseId"`
	CourseTitle string     `json:"courseTitle"`
	ArticleID   int        `json:"articleId"`
	Title       string     `json:"title"`
	TypeKey     string     `json:"typeKey"`
	TypeName    string     `json:"typeName"`
	IsVideo     bool       `json:"isVideo"`
	Status      TaskStatus `json:"status"`
	Stage       string     `json:"stage"`
	Error       string     `json:"error,omitempty"`
	Total       int64      `json:"total"`
	Done        int64      `json:"done"`
	Speed       float64    `json:"speed"`
	Bytes       int64      `json:"bytes"` // bytes downloaded by this run over all stages
	CreatedAt   int64      `json:"createdAt"`
	StartedAt   int64      `json:"startedAt,omitempty"`
	FinishedAt  int64      `json:"finishedAt,omitempty"`

	kind       taskKind
	course     geektime.Course
	option     ui.ProductTypeSelectOption
	article    geektime.Article
	dir        string
	cancel     context.CancelFunc
	removed    bool
	overwrite  bool // re-download even if files exist
	lastBytes  int64
	lastTick   time.Time
	lastUpdate time.Time
}

func (t *Task) key() string {
	return t.TypeKey + ":" + itoa(t.CourseID) + ":" + itoa(t.ArticleID)
}

// TaskManager queues and runs download tasks with limited concurrency.
type TaskManager struct {
	ctx context.Context
	srv *Server

	mu      sync.Mutex
	tasks   []*Task
	nextID  int64
	running int
	wg      sync.WaitGroup
	rnd     *rand.Rand
}

func newTaskManager(ctx context.Context, srv *Server) *TaskManager {
	return &TaskManager{
		ctx: ctx,
		srv: srv,
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// add enqueues tasks, skipping those already queued or running. Returns number added.
func (m *TaskManager) add(tasks []*Task) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := make(map[string]*Task)
	for _, t := range m.tasks {
		active[t.key()] = t
	}
	added := 0
	now := time.Now()
	for _, t := range tasks {
		if existing, ok := active[t.key()]; ok {
			if !existing.Status.finished() {
				continue
			}
			// re-queue the finished one instead of creating a duplicate entry
			existing.reset()
			existing.removed = false
			existing.overwrite = t.overwrite
			added++
			continue
		}
		m.nextID++
		t.ID = m.nextID
		t.Status = StatusQueued
		t.CreatedAt = now.UnixMilli()
		m.tasks = append(m.tasks, t)
		active[t.key()] = t
		added++
	}
	m.scheduleLocked()
	return added
}

func (t *Task) reset() {
	t.Status = StatusQueued
	t.Stage = ""
	t.Error = ""
	t.Total, t.Done, t.Speed, t.Bytes, t.lastBytes = 0, 0, 0, 0, 0
	t.StartedAt, t.FinishedAt = 0, 0
	t.CreatedAt = time.Now().UnixMilli()
}

// list returns copies of all tasks
func (m *TaskManager) list() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, len(m.tasks))
	now := time.Now()
	for i, t := range m.tasks {
		c := *t
		if c.Status != StatusRunning || now.Sub(t.lastUpdate) > 3*time.Second {
			c.Speed = 0
		}
		out[i] = c
	}
	return out
}

func (m *TaskManager) find(id int64) *Task {
	for _, t := range m.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (m *TaskManager) cancel(ids []int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if t := m.find(id); t != nil {
			m.cancelLocked(t)
		}
	}
}

func (m *TaskManager) cancelLocked(t *Task) {
	switch t.Status {
	case StatusQueued:
		t.Status = StatusCanceled
		t.FinishedAt = time.Now().UnixMilli()
	case StatusRunning:
		if t.cancel != nil {
			t.cancel()
		}
	}
}

func (m *TaskManager) cancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		m.cancelLocked(t)
	}
}

func (m *TaskManager) retry(ids []int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if t := m.find(id); t != nil && t.Status.finished() && t.Status != StatusDone {
			t.reset()
		}
	}
	m.scheduleLocked()
}

func (m *TaskManager) retryFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		if t.Status == StatusFailed || t.Status == StatusCanceled {
			t.reset()
		}
	}
	m.scheduleLocked()
}

// remove deletes tasks, canceling running ones first
func (m *TaskManager) remove(ids []int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	drop := make(map[int64]bool, len(ids))
	for _, id := range ids {
		drop[id] = true
	}
	m.filterLocked(func(t *Task) bool {
		if !drop[t.ID] {
			return true
		}
		if t.Status == StatusRunning {
			// keep running ones until they observe the cancellation
			t.removed = true
			m.cancelLocked(t)
			return true
		}
		return false
	})
}

// clearFinished removes all done and skipped tasks
func (m *TaskManager) clearFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.filterLocked(func(t *Task) bool {
		return t.Status != StatusDone && t.Status != StatusSkipped
	})
}

func (m *TaskManager) filterLocked(keep func(*Task) bool) {
	kept := m.tasks[:0]
	for _, t := range m.tasks {
		if keep(t) {
			kept = append(kept, t)
		}
	}
	for i := len(kept); i < len(m.tasks); i++ {
		m.tasks[i] = nil
	}
	m.tasks = kept
}

func (m *TaskManager) taskDir(id int64) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(id)
	if t == nil {
		return "", false
	}
	return t.dir, true
}

// reschedule starts queued tasks if settings allow more concurrency
func (m *TaskManager) reschedule() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduleLocked()
}

func (m *TaskManager) scheduleLocked() {
	if m.ctx.Err() != nil {
		return
	}
	_, st := m.srv.snapshotConfig()
	for _, t := range m.tasks {
		if m.running >= st.ConcurrentTasks {
			return
		}
		if t.Status != StatusQueued {
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		t.cancel = cancel
		t.Status = StatusRunning
		t.Stage = "准备中"
		t.StartedAt = time.Now().UnixMilli()
		t.lastTick = time.Now()
		t.lastUpdate = time.Now()
		m.running++
		m.wg.Add(1)
		go m.run(ctx, t)
	}
}

func (m *TaskManager) run(ctx context.Context, t *Task) {
	defer m.wg.Done()

	skipped, err := m.execute(ctx, t)
	if err == nil && !skipped {
		// be gentle with geektime api like the CLI does, keep the slot while waiting
		m.waitInterval(ctx)
	}

	if err == nil {
		// remember what has been downloaded, also for skipped (nothing to download) articles
		m.srv.records.addArticle(t.CourseID, t.CourseTitle, t.ArticleID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	t.cancel = nil
	t.FinishedAt = time.Now().UnixMilli()
	t.Speed = 0
	switch {
	case skipped:
		t.Status = StatusSkipped
	case err == nil:
		t.Status = StatusDone
		t.Stage = "已完成"
		if t.Total > 0 {
			t.Done = t.Total
		}
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		t.Status = StatusCanceled
		t.Stage = "已取消"
	default:
		t.Status = StatusFailed
		t.Error = err.Error()
		logger.Errorf(err, "GUI task failed, course: %s, title: %s", t.CourseTitle, t.Title)
	}
	m.running--
	if t.removed {
		m.filterLocked(func(x *Task) bool { return x != t })
	}
	m.scheduleLocked()
}

func (m *TaskManager) execute(ctx context.Context, t *Task) (skipped bool, err error) {
	client := m.srv.currentClient()
	if client == nil {
		return false, errors.New("未登录")
	}
	cfg, st := m.srv.snapshotConfig()
	overwrite := st.Overwrite || t.overwrite

	m.mu.Lock()
	t.dir = filepath.Join(cfg.DownloadFolder, filenamify.Filenamify(t.CourseTitle))
	m.mu.Unlock()

	ctx = withTaskReporter(ctx, m, t)
	d := course.NewCourseDownloader(ctx, &cfg, client, nil)

	if t.kind == kindProduct {
		return false, d.DownloadSingleVideoProduct(t.CourseTitle, t.ArticleID, t.option.SourceType)
	}

	if err := os.MkdirAll(cfg.DownloadFolder, os.ModePerm); err != nil {
		return false, err
	}

	// 训练营: 视频课下载视频, 图文文章保存为 Markdown
	if t.option.IsUniversity() {
		skipped, err := d.DownloadUniversityArticle(t.course, t.option, t.article, overwrite)
		if skipped {
			m.mu.Lock()
			t.Stage = "文章无可下载内容, 已跳过"
			m.mu.Unlock()
		}
		return skipped, err
	}

	return false, d.DownloadArticle(t.course, t.option, t.article, overwrite)
}

func (m *TaskManager) waitInterval(ctx context.Context) {
	cfg, _ := m.srv.snapshotConfig()
	m.mu.Lock()
	jitter := m.rnd.Intn(2000)
	m.mu.Unlock()
	wait := time.Duration(cfg.Interval*1000+jitter) * time.Millisecond
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

// wait blocks until all running tasks exit or timeout
func (m *TaskManager) wait(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// taskReporter implements progress.Reporter for a task
type taskReporter struct {
	m *TaskManager
	t *Task
}

func (r *taskReporter) SetStage(stage string) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	r.t.Stage = stage
	r.t.lastUpdate = time.Now()
}

func (r *taskReporter) SetTotal(total int64) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	r.t.Total = total
	r.t.Done = 0
}

func (r *taskReporter) Add(n int64) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	t := r.t
	t.Done += n
	if t.Total > 0 && t.Done > t.Total {
		t.Done = t.Total
	}
	t.Bytes += n
	now := time.Now()
	t.lastUpdate = now
	if t.lastTick.IsZero() {
		t.lastTick = now
	}
	// measured on the cumulative bytes so many small files in a row
	// (images of an article) still produce a speed
	if elapsed := now.Sub(t.lastTick).Seconds(); elapsed >= 1 {
		instant := float64(t.Bytes-t.lastBytes) / elapsed
		if t.Speed == 0 {
			t.Speed = instant
		} else {
			t.Speed = 0.6*instant + 0.4*t.Speed
		}
		t.lastBytes = t.Bytes
		t.lastTick = now
	}
}

func withTaskReporter(ctx context.Context, m *TaskManager, t *Task) context.Context {
	return progress.WithReporter(ctx, &taskReporter{m: m, t: t})
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
