package gui

import (
	"context"
	"testing"
	"time"
)

func TestTaskReporterBytesAcrossStages(t *testing.T) {
	s, _ := newTestServer(t)
	m := newTaskManager(context.Background(), s)
	task := &Task{Status: StatusRunning, lastTick: time.Now().Add(-2 * time.Second)}
	r := &taskReporter{m: m, t: task}

	// several small files, each one resets the stage total
	for i := 0; i < 3; i++ {
		r.SetTotal(1000)
		r.Add(1000)
	}
	if task.Bytes != 3000 || task.Done != 1000 {
		t.Fatalf("bytes=%d done=%d", task.Bytes, task.Done)
	}
	if task.Speed <= 0 {
		t.Fatal("speed should be measured over the small files")
	}

	task.Status = StatusFailed
	task.reset()
	if task.Bytes != 0 || task.lastBytes != 0 || task.Speed != 0 {
		t.Fatalf("reset should clear progress: %+v", task)
	}
}
