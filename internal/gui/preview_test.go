package gui

import (
	"net/http"
	"sync/atomic"
	"testing"
)

func TestGroupOutline(t *testing.T) {
	p := groupOutline(
		[]string{"a", "b", "c", "d"},
		[]string{"二", "一", "二", "其他"},
		[]string{"一", "二", "空章"},
	)
	if p.Total != 4 || len(p.Chapters) != 3 {
		t.Fatalf("unexpected outline: %+v", p)
	}
	if p.Chapters[0].Title != "一" || p.Chapters[1].Title != "二" || p.Chapters[1].Count != 2 || p.Chapters[2].Title != "其他" {
		t.Fatalf("order / grouping: %+v", p.Chapters)
	}
	if u := groupOutline([]string{"x", "y"}, []string{"", ""}, nil); len(u.Chapters) != 1 || u.Chapters[0].Title != "" || u.Chapters[0].Count != 2 {
		t.Fatalf("untitled group: %+v", u)
	}
}

func TestCoursePreview(t *testing.T) {
	var calls int32
	_, h, get := newOutlineServer(t, &calls)

	var p previewDTO
	if code := get("/api/course/preview?type=n0&id=7", &p); code != http.StatusOK {
		t.Fatalf("preview: %d", code)
	}
	if p.Total != 3 || len(p.Chapters) != 2 || p.Chapters[1].Title != "正文" || len(p.Chapters[1].Articles) != 2 || p.First != 100 || p.Last != 300 {
		t.Fatalf("unexpected preview: %+v", p)
	}
	// single video products have no outline and need no upstream call
	before := atomic.LoadInt32(&calls)
	if code := get("/api/course/preview?type=n1&id=9", &p); code != http.StatusOK || p.Total != 0 || p.Chapters == nil || atomic.LoadInt32(&calls) != before {
		t.Fatalf("direct preview: %d %+v", code, p)
	}
	if rec := do(h, "GET", "/api/course/preview?type=zz&id=7", "127.0.0.1:1234", "secret", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad type: %d", rec.Code)
	}
}
