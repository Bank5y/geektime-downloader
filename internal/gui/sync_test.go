package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/ui"
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

func TestSyncRecords(t *testing.T) {
	s, _ := newTestServer(t)
	var option ui.ProductTypeSelectOption
	for _, o := range ui.ProductTypeOptions(false) {
		if o.NeedSelectArticle && !o.IsUniversity() {
			option = o
			break
		}
	}
	key := typeKeyOf(option)
	course := geektime.Course{Access: true, ID: 100, Title: "专栏 A", Articles: []geektime.Article{
		{AID: 1, Title: "01 | 开篇"}, {AID: 2, Title: "02 | 第二"}, {AID: 3, Title: "03 | 第三"},
	}}
	s.courseCache[key+":100"] = course
	products := []productDTO{{CourseID: 100, Title: course.Title, TypeKey: key, Openable: true}}

	dir := filepath.Join(s.cfg.DownloadFolder, filenamify.Filenamify(course.Title))
	touch(t, filepath.Join(dir, filenamify.Filenamify("01 | 开篇")+".md"))
	touch(t, filepath.Join(dir, "第一章", filenamify.Filenamify("02 | 第二")+".mp3"))
	touch(t, filepath.Join(dir, "images", "1", "a.png"))
	// embedded videos of a text article do not count as the article itself
	touch(t, filepath.Join(dir, "videos", filenamify.Filenamify("03 | 第三"), filenamify.Filenamify("03 | 第三")+".ts"))

	s.records.addArticle(100, course.Title, 3) // file is gone
	s.records.addArticle(200, "已删除的课程", 9)     // folder is gone
	s.records.setMarked(300, "手动标记", true)     // folder is gone, mark kept

	res, err := s.syncRecords(geektime.NewClient(nil), products)
	if err != nil {
		t.Fatal(err)
	}
	if res.Courses != 2 || res.Added != 2 || res.Removed != 2 || len(res.Unresolved) != 0 || res.Aborted != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	ids, _ := s.records.articles(100)
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("course 100 articles: %v", ids)
	}
	m := s.records.marks()
	if m["100"] != (markDTO{Done: 2, Total: 3}) {
		t.Fatalf("course 100 mark: %+v", m["100"])
	}
	if _, ok := m["200"]; ok {
		t.Fatalf("course 200 should have no downloads: %+v", m)
	}
	if !m["300"].Marked {
		t.Fatalf("manual mark lost: %+v", m)
	}

	// a second sync changes nothing
	res, _ = s.syncRecords(geektime.NewClient(nil), products)
	if res.Added != 0 || res.Removed != 0 {
		t.Fatalf("second sync should be a no-op: %+v", res)
	}
}
