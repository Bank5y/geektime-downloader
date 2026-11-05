package gui

import (
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
	r.addArticle(42, "课程", 7)
	r.addArticle(42, "课程", 8)
	r.addArticle(42, "课程", 8) // repeated download counts once
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
	r.addArticle(1, "c", 2)
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
