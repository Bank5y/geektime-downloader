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

// redirect sends every request of the geektime client to a local mock server
type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// mock geektime: product 1 free claimable, 2 owned, 3 赠一得一, 4 paid, 5 free but claim rejected
func newMockGeektime(t *testing.T, claims *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case geektime.V3ProductInfoPath:
			id := int(body["id"].(float64))
			saleType, owned := map[int]int{1: 6, 2: 6, 3: 7, 4: 2, 5: 6}[id], id == 2
			fmt.Fprintf(w, `{"code":0,"data":{"info":{"title":"课程%d","type":"p35","price":{"sale_type":%d},"extra":{"sub":{"had_done":%v}}}}}`, id, saleType, owned)
		case geektime.V3SaleMflqGetPath:
			atomic.AddInt32(claims, 1)
			if int(body["pid"].(float64)) == 5 {
				fmt.Fprint(w, `{"code":-1,"data":[],"error":{"code":-3301,"msg":"活动已结束"}}`)
				return
			}
			fmt.Fprint(w, `{"code":0,"data":{"is_success":true}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
}

func TestUnlockOnlyClaimsFreeCourses(t *testing.T) {
	var claims int32
	mock := newMockGeektime(t, &claims)
	defer mock.Close()
	target, _ := url.Parse(mock.URL)

	s, h := newTestServer(t)
	client := geektime.NewClient(nil)
	client.RestyClient.SetTransport(redirect{target})
	s.client = client
	s.products = []productDTO{{ID: 9}}

	cases := []struct {
		id      int
		status  string
		claimed int32
	}{
		{1, "claimed", 1},
		{2, "owned", 1},       // no claim request for owned course
		{3, "unsupported", 1}, // 赠一得一
		{4, "unsupported", 1}, // paid course
		{5, "error", 2},       // claim attempted, server message surfaced
	}
	for _, c := range cases {
		rec := do(h, "POST", "/api/unlock", "127.0.0.1:1234", "secret", fmt.Sprintf(`{"productId":%d}`, c.id))
		if rec.Code != http.StatusOK {
			t.Fatalf("id %d: http %d %s", c.id, rec.Code, rec.Body.String())
		}
		var res unlockResult
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Status != c.status || atomic.LoadInt32(&claims) != c.claimed {
			t.Fatalf("id %d: got %+v, claims %d", c.id, res, claims)
		}
		if c.id == 5 && !strings.Contains(res.Message, "活动已结束") {
			t.Fatalf("server message not surfaced: %q", res.Message)
		}
	}
	if s.products != nil {
		t.Fatal("my courses cache should be invalidated after a claim")
	}
}
