package gui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
)

func TestReaderArticle(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case geektime.V1ChaptersPath:
			if body["cid"] == "8" {
				fmt.Fprint(w, `{"code":0,"data":[]}`)
				return
			}
		case geektime.V1ColumnArticlesPath:
			// course 8's table of contents puts 2 before 1, unlike the api neighbors
			fmt.Fprint(w, `{"code":0,"data":{"list":[{"id":2,"article_title":"第二讲"},{"id":1,"article_title":"开篇"}],"page":{"count":2}}}`)
			return
		}
		switch body["id"] {
		case "1": // owned, first article
			fmt.Fprint(w, `{"code":0,"data":{"id":1,"article_title":"开篇","article_content":"<p onclick=\"x()\">正文<script>alert(1)</script></p>","author_name":"王争","article_ctime":100,"audio_download_url":"https://static001.geekbang.org/a.mp3","neighbors":{"left":[],"right":{"id":2,"article_title":"第二讲"}}}}`)
		case "2": // not owned: only the excerpt
			fmt.Fprint(w, `{"code":0,"data":{"id":2,"article_title":"第二讲","article_content":"","article_cshort":"<p>试读</p>","audio_download_url":"javascript:alert(1)","neighbors":{"left":{"id":1,"article_title":"开篇"},"right":[]}}}`)
		default:
			fmt.Fprint(w, `{"code":-1,"data":[],"error":{"code":-2202,"msg":"无效的文章ID"}}`)
		}
	}))
	defer mock.Close()
	target, _ := url.Parse(mock.URL)
	s, h := newTestServer(t)
	client := geektime.NewClient(nil)
	client.RestyClient.SetTransport(redirect{target})
	s.client = client

	get := func(q string) (int, readerArticleDTO, string) {
		rec := do(h, "GET", "/api/article?"+q, "127.0.0.1:1234", "secret", "")
		var d readerArticleDTO
		_ = json.Unmarshal(rec.Body.Bytes(), &d)
		return rec.Code, d, rec.Body.String()
	}
	code, a, raw := get("type=n0&course=7&id=1")
	if code != http.StatusOK || a.Title != "开篇" || a.Trial || a.Prev != nil || a.Next == nil || a.Next.ID != 2 || a.Audio == "" || a.Ctime != 100 {
		t.Fatalf("article 1: %d %+v", code, a)
	}
	if strings.Contains(a.HTML, "script") || strings.Contains(a.HTML, "onclick") || !strings.Contains(a.HTML, "正文") {
		t.Fatalf("not sanitized: %s", raw)
	}
	if code, a, _ = get("type=n0&course=7&id=2"); code != http.StatusOK || !a.Trial || a.HTML != "<p>试读</p>" || a.Audio != "" || a.Prev == nil || a.Next != nil {
		t.Fatalf("article 2: %d %+v", code, a)
	}
	// neighbors follow the outline when the course outline is available
	if code, a, _ = get("type=n0&course=8&id=1"); code != http.StatusOK || a.Prev == nil || a.Prev.ID != 2 || a.Next != nil {
		t.Fatalf("outline neighbors: %d %+v %+v", code, a.Prev, a.Next)
	}
	if code, _, raw = get("type=n0&course=7&id=3"); code != http.StatusUnprocessableEntity || !strings.Contains(raw, "无效的文章ID") {
		t.Fatalf("bad id: %d %s", code, raw)
	}
	// single video products have no articles
	if code, _, _ = get("type=n1&course=7&id=1"); code != http.StatusBadRequest {
		t.Fatalf("direct type: %d", code)
	}
}
