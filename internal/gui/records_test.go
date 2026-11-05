package gui

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "downloads.db")
	r := newRecordStore(path)
	r.setTotal(42, "课程", 10, false) // no record yet, not created
	if len(r.marks()) != 0 {
		t.Fatal("setTotal without create should not create a record")
	}
	r.setTotal(42, "课程", 10, true)
	r.addArticle(articleRecord{CourseID: 42, CourseTitle: "课程", ArticleID: 7})
	r.addArticle(articleRecord{CourseID: 42, CourseTitle: "课程", ArticleID: 8})
	r.addArticle(articleRecord{CourseID: 42, CourseTitle: "课程", ArticleID: 8}) // repeated download counts once
	r.setMarked(5, "另一门", true)

	r2 := newRecordStore(path)
	m := r2.marks()
	if m["42"] != (markDTO{Done: 2, Total: 10}) || !m["5"].Marked {
		t.Fatalf("unexpected marks after reload: %+v", m)
	}
	ids, marked := r2.articles(42)
	if len(ids) != 2 || ids[0] != 7 || ids[1] != 8 || marked {
		t.Fatalf("articles: %v %v", ids, marked)
	}
	if ids, marked := r2.articles(999); len(ids) != 0 || marked {
		t.Fatalf("unknown course: %v %v", ids, marked)
	}
}

func TestRecordStoreMemory(t *testing.T) {
	r := newRecordStore("")
	r.addArticle(articleRecord{CourseID: 1, CourseTitle: "c", ArticleID: 2})
	if r.marks()["1"].Done != 1 {
		t.Fatalf("memory store lost records: %+v", r.marks())
	}
}

func TestRecordStoreImportsLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "downloads.json")
	if err := os.WriteFile(legacy, []byte(`{"42":{"title":"课程","total":10,"articles":{"7":100,"8":200},"marked":false},"5":{"title":"另一门","marked":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newRecordStore(filepath.Join(dir, "downloads.db"))
	m := r.marks()
	if m["42"] != (markDTO{Done: 2, Total: 10}) || !m["5"].Marked {
		t.Fatalf("unexpected marks after import: %+v", m)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy json should be renamed after import")
	}
	if _, err := os.Stat(legacy + ".bak"); err != nil {
		t.Fatal("legacy json backup missing")
	}
}

func TestRecordStoreMigratesOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	// schema and data of the first sqlite version
	if _, err := db.Exec(`CREATE TABLE courses (id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '', total INTEGER NOT NULL DEFAULT 0,
		marked INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL DEFAULT 0);
		CREATE TABLE downloaded_articles (course_id INTEGER NOT NULL, article_id INTEGER NOT NULL, finished_at INTEGER NOT NULL,
		PRIMARY KEY (course_id, article_id));
		INSERT INTO courses (id, title) VALUES (42, '课程');
		INSERT INTO downloaded_articles VALUES (42, 7, 1);`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	r := newRecordStore(path)
	rows, err := r.downloadedArticles()
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows: %v %v", rows, err)
	}
	if rows[0].Files != nil || rows[0].Dir != "课程" {
		t.Fatalf("old record should have unknown files and default dir: %+v", rows[0])
	}
	r.addArticle(articleRecord{CourseID: 42, CourseTitle: "课程", TypeKey: "c1", ArticleID: 8, Title: "第二", Files: []string{"第二.md"}})
	rows, _ = r.downloadedArticles()
	if len(rows) != 2 || rows[1].Title != "第二" || len(rows[1].Files) != 1 || rows[1].TypeKey != "c1" {
		t.Fatalf("new record: %+v", rows)
	}
}
