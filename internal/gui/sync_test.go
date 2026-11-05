package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/markdown"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/progress"
)

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRecordsVerifiesFiles(t *testing.T) {
	s, _ := newTestServer(t)
	dir := filepath.Join(s.cfg.DownloadFolder, filenamify.Filenamify("专栏 A"))
	touch(t, filepath.Join(dir, "01.md"))
	touch(t, filepath.Join(dir, "第一章", "02.mp3"))
	touch(t, filepath.Join(dir, "04.md"))

	add := func(aid int, files ...string) {
		if files == nil {
			files = []string{}
		}
		s.records.addArticle(articleRecord{CourseID: 100, CourseTitle: "专栏 A", TypeKey: "c1", ArticleID: aid, Title: itoa(aid), Files: files})
	}
	add(1, "01.md")
	add(2, "第一章/02.mp3")
	add(3, "03.md", "03.pdf") // both deleted
	add(4, "04.md", "04.mp3") // audio deleted, markdown still there
	add(5)                    // nothing to download (no content)
	s.records.setMarked(300, "手动标记", true)

	// no client: verifying records with known files needs no api
	res, err := s.syncRecords(nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Checked != 5 || res.Removed != 1 || res.Backfilled != 0 || len(res.Unresolved) != 0 || res.Aborted != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	ids, _ := s.records.articles(100)
	if len(ids) != 4 || ids[0] != 1 || ids[1] != 2 || ids[2] != 4 || ids[3] != 5 {
		t.Fatalf("articles: %v", ids)
	}
	rows, _ := s.records.downloadedArticles()
	for _, a := range rows {
		if a.ArticleID == 4 && (len(a.Files) != 1 || a.Files[0] != "04.md") {
			t.Fatalf("files of 4 should shrink to what exists: %v", a.Files)
		}
	}
	if !s.records.marks()["300"].Marked {
		t.Fatal("manual mark lost")
	}

	// local files without a record are not discovered
	touch(t, filepath.Join(dir, "06.md"))
	if res, _ := s.syncRecords(nil); res.Removed != 0 || res.Checked != 4 {
		t.Fatalf("second sync: %+v", res)
	}
	if ids, _ := s.records.articles(100); len(ids) != 4 {
		t.Fatalf("no discovery expected: %v", ids)
	}
}

func TestSyncRecordsBackfillsOldRecords(t *testing.T) {
	s, _ := newTestServer(t)
	course := geektime.Course{Access: true, ID: 100, Title: "专栏 A", Articles: []geektime.Article{
		{AID: 1, Title: "01 | 开篇"}, {AID: 2, Title: "02 | 第二", SectionTitle: "第一章"}, {AID: 3, Title: "03 | 第三"},
	}}
	s.courseCache[autoKey+":100"] = course

	dir := filepath.Join(s.cfg.DownloadFolder, filenamify.Filenamify(course.Title))
	touch(t, filepath.Join(dir, filenamify.Filenamify("01 | 开篇")+".md"))
	touch(t, filepath.Join(dir, "第一章", filenamify.Filenamify("02 | 第二")+".ts"))

	// old records: no files stored
	for _, q := range []string{
		`INSERT INTO courses (id, title) VALUES (100, '专栏 A'), (200, '已删除的课程')`,
		`INSERT INTO downloaded_articles (course_id, article_id, finished_at) VALUES (100, 1, 1), (100, 2, 1), (100, 3, 1), (200, 9, 1)`,
	} {
		if _, err := s.records.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	res, err := s.syncRecords(geektime.NewClient(nil))
	if err != nil {
		t.Fatal(err)
	}
	// course 200: folder gone, removed without api; course 100: 3 has no file
	if res.Checked != 4 || res.Removed != 2 || res.Backfilled != 3 || len(res.Unresolved) != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	rows, _ := s.records.downloadedArticles()
	if len(rows) != 2 || rows[0].Title != "01 | 开篇" || rows[1].Files[0] != "第一章/"+filenamify.Filenamify("02 | 第二")+".ts" {
		t.Fatalf("backfilled rows: %+v", rows)
	}

	// afterwards no api is needed
	delete(s.courseCache, autoKey+":100")
	if res, _ := s.syncRecords(nil); res.Backfilled != 0 || res.Removed != 0 || len(res.Unresolved) != 0 {
		t.Fatalf("second sync: %+v", res)
	}
}

func TestRelativeFiles(t *testing.T) {
	dir := filepath.Join("/d", "课程")
	got := relativeFiles(dir, []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "章", "b.ts"), "/other/c.md"})
	if len(got) != 2 || got[0] != "a.md" || got[1] != "章/b.ts" {
		t.Fatalf("got %v", got)
	}
}

func TestTaskRecordsReportedFiles(t *testing.T) {
	s, _ := newTestServer(t)
	m := newTaskManager(context.Background(), s)
	dir := filepath.Join(s.cfg.DownloadFolder, "专栏 A")
	task := &Task{dir: dir}
	ctx := withTaskReporter(context.Background(), m, task)
	if err := markdown.Download(ctx, "<p>正文</p>", "01 | 开篇", dir, 1, nil); err != nil {
		t.Fatal(err)
	}
	progress.File(ctx, filepath.Join(dir, filenamify.Filenamify("01 | 开篇")+".md")) // reported twice, kept once
	got := relativeFiles(task.dir, task.files)
	if len(got) != 1 || got[0] != filenamify.Filenamify("01 | 开篇")+".md" {
		t.Fatalf("reported files: %v", got)
	}
}
