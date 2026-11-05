package gui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
)

func TestCatalogLaunchSort(t *testing.T) {
	// 65 products over 3 pages (total reported as 70, like the real api counts delisted
	// courses); begin_time increases with the id, id 5 has none and falls back to ctime,
	// id 6 has neither
	var calls int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var body map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		if r.URL.Path != geektime.V4ProductListPath || body["sort"].(float64) != 8 || body["size"].(float64) != 30 {
			t.Errorf("unexpected request %s %v", r.URL.Path, body)
		}
		page := int(body["prev"].(float64))
		var items []string
		for i := (page-1)*30 + 1; i <= page*30 && i <= 65; i++ {
			begin, ctime := 1000+i, 0
			if i == 5 {
				begin, ctime = 0, 5000
			}
			if i == 6 {
				begin = 0
			}
			items = append(items, fmt.Sprintf(`{"id":%d,"type":"c1","title":"c%d","begin_time":%d,"ctime":%d}`, i, i, begin, ctime))
		}
		if page == 2 { // duplicate across pages
			items = append(items, `{"id":1,"type":"c1","title":"c1","begin_time":1001}`)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"data":{"products":[%s],"page":{"total":70,"more":%v}}}`, strings.Join(items, ","), page*30 < 65)
	}))
	defer mock.Close()
	target, _ := url.Parse(mock.URL)
	s, h := newTestServer(t)
	client := geektime.NewClient(nil)
	client.RestyClient.SetTransport(redirect{target})
	s.client = client

	page := func(sortCode, p, size int) catalogPageDTO {
		var out catalogPageDTO
		rec := do(h, "GET", fmt.Sprintf("/api/catalog?sort=%d&page=%d&size=%d", sortCode, p, size), "127.0.0.1:1234", "secret", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	newest := page(sortLaunchNewest, 1, 3)
	if newest.Total != 65 || !newest.More || newest.Items[0].ID != 5 || newest.Items[1].ID != 65 || newest.Items[2].ID != 64 {
		t.Fatalf("newest: %+v", newest)
	}
	n := atomic.LoadInt32(&calls)
	oldest := page(sortLaunchOldest, 22, 3) // last page: 64 courses with a date, then the unknown one
	if atomic.LoadInt32(&calls) != n {
		t.Fatal("the collected list should be cached")
	}
	if len(oldest.Items) != 2 || oldest.Items[0].ID != 5 || oldest.Items[1].ID != 6 || oldest.More {
		t.Fatalf("oldest last page: %+v", oldest.Items)
	}
	if first := page(sortLaunchOldest, 1, 2); first.Items[0].ID != 1 || first.Items[1].ID != 2 {
		t.Fatalf("oldest: %+v", first.Items)
	}
}
