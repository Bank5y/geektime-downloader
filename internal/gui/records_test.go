package gui

import (
	"path/filepath"
	"testing"
)

func TestRecordStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "downloads.json")
	r := newRecordStore(path)
	r.setTotal(42, "课程", 10, false) // no record yet, not created
	if len(r.marks()) != 0 {
		t.Fatal("setTotal without create should not create a record")
	}
	r.setTotal(42, "课程", 10, true)
	r.addArticle(42, "课程", 7)
	r.addArticle(42, "课程", 8)
	r.setMarked(5, "另一门", true)

	r2 := newRecordStore(path)
	m := r2.marks()
	if m["42"] != (markDTO{Done: 2, Total: 10}) || !m["5"].Marked {
		t.Fatalf("unexpected marks after reload: %+v", m)
	}
	ids, marked := r2.articles(42)
	if len(ids) != 2 || marked {
		t.Fatalf("articles: %v %v", ids, marked)
	}
}
