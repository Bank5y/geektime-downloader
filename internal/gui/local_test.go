package gui

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
)

func TestHandleLocal(t *testing.T) {
	s, h := newTestServer(t)
	dir := filepath.Join(s.cfg.DownloadFolder, filenamify.Filenamify("专栏 A"))
	touch(t, filepath.Join(dir, "01.md"))
	s.records.addArticle(articleRecord{CourseID: 100, CourseTitle: "专栏 A", TypeKey: "c1", ArticleID: 2, Title: "02", Files: []string{"02.md"}})
	s.records.addArticle(articleRecord{CourseID: 100, CourseTitle: "专栏 A", TypeKey: "c1", ArticleID: 1, Title: "01", Files: []string{"01.md"}})
	if _, err := s.records.db.Exec(`INSERT INTO courses (id, title) VALUES (200, '旧课程');
		INSERT INTO downloaded_articles (course_id, article_id, finished_at) VALUES (200, 9, 1)`); err != nil {
		t.Fatal(err)
	}

	rec := do(h, "GET", "/api/local", "127.0.0.1:1234", "secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Courses []localCourseDTO `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Courses) != 2 {
		t.Fatalf("courses: %+v", res.Courses)
	}
	var a, old localCourseDTO
	for _, c := range res.Courses {
		if c.ID == 100 {
			a = c
		} else {
			old = c
		}
	}
	if !a.Exists || a.Size != 1 || len(a.Articles) != 2 || a.Articles[0].ID != 1 {
		t.Fatalf("course 100: %+v", a)
	}
	if !a.Articles[0].Files[0].Exists || a.Articles[1].Files[0].Exists {
		t.Fatalf("file existence: %+v", a.Articles)
	}
	if old.Exists || !old.Articles[0].Legacy {
		t.Fatalf("old course: %+v", old)
	}
}

func TestLocalPathStaysInDownloadFolder(t *testing.T) {
	s, _ := newTestServer(t)
	s.records.addArticle(articleRecord{CourseID: 100, CourseTitle: "专栏 A", ArticleID: 1})
	if p, ok := s.localPath(100, "章/01.md"); !ok || p != filepath.Join(s.cfg.DownloadFolder, filenamify.Filenamify("专栏 A"), "章", "01.md") {
		t.Fatalf("got %q %v", p, ok)
	}
	if _, ok := s.localPath(100, "../../etc/passwd"); ok {
		t.Fatal("path traversal should be rejected")
	}
	if _, ok := s.localPath(999, ""); ok {
		t.Fatal("unknown course should be rejected")
	}
}
