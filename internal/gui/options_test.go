package gui

import (
	"path/filepath"
	"testing"
)

func TestTaskOptions(t *testing.T) {
	s, _ := newTestServer(t)
	s.settings.ConcurrentTasks = 3
	s.settings.Interval = 2

	if got, err := s.taskOptions(nil); got != nil || err != nil {
		t.Fatalf("nil options: %v %v", got, err)
	}
	opts := s.settings
	opts.DownloadFolder = filepath.Join(t.TempDir(), "other")
	opts.ColumnOutputType = 2
	opts.Threads = 7
	opts.ConcurrentTasks = 8 // scheduling options are not per task
	opts.Interval = 9
	got, err := s.taskOptions(&opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConcurrentTasks != 3 || got.Interval != 2 || got.Threads != 7 {
		t.Fatalf("options: %+v", got)
	}
	cfg, _ := s.configWith(*got)
	if cfg.DownloadFolder != opts.DownloadFolder || cfg.ColumnOutputType != 2 || cfg.Threads != 7 {
		t.Fatalf("config: %+v", cfg)
	}

	bad := s.settings
	bad.DownloadFolder = " "
	if _, err := s.taskOptions(&bad); err == nil {
		t.Fatal("empty folder should be rejected")
	}
	bad = s.settings
	bad.ColumnOutputType = 0
	if _, err := s.taskOptions(&bad); err == nil {
		t.Fatal("no output type should be rejected")
	}
}

func TestRecordsFollowTaskFolder(t *testing.T) {
	s, _ := newTestServer(t)
	other := t.TempDir()
	dir := filepath.Join(other, "专栏A")
	touch(t, filepath.Join(dir, "01.md"))
	s.records.addArticle(articleRecord{CourseID: 100, CourseTitle: "专栏 A", ArticleID: 1, Title: "01", Files: []string{"01.md"}, Root: other})

	if p, ok := s.localPath(100, "01.md"); !ok || p != filepath.Join(dir, "01.md") {
		t.Fatalf("localPath: %q %v", p, ok)
	}
	// the file is in the folder the task used, not the current download folder
	if res, _ := s.syncRecords(nil); res.Removed != 0 || res.Checked != 1 {
		t.Fatalf("sync: %+v", res)
	}
}
