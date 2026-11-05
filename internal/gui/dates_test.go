package gui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
)

// mock column api: course 7 has 3 articles in 2 chapters, course 8 fails
func newMockOutline(t *testing.T, calls *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		var body map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if body["cid"] == "8" {
			fmt.Fprint(w, `{"code":-1,"error":{"code":-1,"msg":"x"},"data":[]}`)
			return
		}
		switch r.URL.Path {
		case "/serv/v1/chapters":
			fmt.Fprint(w, `{"code":0,"data":[{"id":"11","title":"开篇","article_count":1},{"id":"12","title":"正文","article_count":2}]}`)
		case geektime.V1ColumnArticlesPath:
			list := `[{"id":1,"article_title":"开篇词","chapter_id":"11","article_ctime":100},{"id":2,"article_title":"第一讲","chapter_id":"12","article_ctime":200},{"id":3,"article_title":"第二讲","chapter_id":"12","article_ctime":300}]`
			if body["order"] == "newest" {
				list = `[{"id":3,"article_title":"第二讲","chapter_id":"12","article_ctime":300}]`
			} else if body["size"].(float64) == 1 {
				list = `[{"id":1,"article_title":"开篇词","chapter_id":"11","article_ctime":100}]`
			}
			fmt.Fprintf(w, `{"code":0,"data":{"list":%s,"page":{"count":3,"more":false}}}`, list)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
}

// newOutlineServer returns a gui server whose geektime client talks to the mock
func newOutlineServer(t *testing.T, calls *int32) (*Server, http.Handler, func(string, interface{}) int) {
	t.Helper()
	mock := newMockOutline(t, calls)
	t.Cleanup(mock.Close)
	target, _ := url.Parse(mock.URL)

	s, h := newTestServer(t)
	client := geektime.NewClient(nil)
	client.RestyClient.SetTransport(redirect{target})
	s.client = client
	get := func(path string, v interface{}) int {
		rec := do(h, "GET", path, "127.0.0.1:1234", "secret", "")
		_ = json.Unmarshal(rec.Body.Bytes(), v)
		return rec.Code
	}
	return s, h, get
}

func TestCourseDates(t *testing.T) {
	var calls int32
	_, _, get := newOutlineServer(t, &calls)

	var dates struct {
		Dates map[string]courseDates `json:"dates"`
	}
	if code := get("/api/course/dates?ids=7,8,x,7&first=7", &dates); code != http.StatusOK {
		t.Fatalf("dates: %d", code)
	}
	if d := dates.Dates["7"]; d.First != 100 || d.Last != 300 || d.Count != 3 || len(dates.Dates) != 1 {
		t.Fatalf("unexpected dates: %+v", dates.Dates)
	}
	// cached: no more upstream calls for 7
	before := atomic.LoadInt32(&calls)
	get("/api/course/dates?ids=7&first=7", &dates)
	if atomic.LoadInt32(&calls) != before {
		t.Fatal("dates should be cached")
	}
}
