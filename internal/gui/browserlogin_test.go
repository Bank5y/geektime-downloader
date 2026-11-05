package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/pkg/browsercookie"
)

func TestBrowserLogin(t *testing.T) {
	oldSources, oldRead, oldAuth := browserSources, readBrowserCookies, authCookies
	defer func() { browserSources, readBrowserCookies, authCookies = oldSources, oldRead, oldAuth }()

	s, h := newTestServer(t)
	post := func(body string) (int, map[string]interface{}) {
		rec := do(h, "POST", "/api/login/browser", "127.0.0.1:1234", "secret", body)
		var m map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}

	browserSources = func() []browsercookie.Source { return nil }
	if code, m := post(`{}`); code != http.StatusUnprocessableEntity || !strings.Contains(m["error"].(string), "未在本机找到") {
		t.Fatalf("no browsers: %d %v", code, m)
	}

	browserSources = func() []browsercookie.Source {
		return []browsercookie.Source{
			{ID: "chrome/Default", Browser: "Google Chrome", Profile: "A"},
			{ID: "chrome/Profile 1", Browser: "Google Chrome", Profile: "B"},
			{ID: "edge/Default", Browser: "Microsoft Edge"},
			{ID: "brave/Default", Browser: "Brave"},
		}
	}
	readBrowserCookies = func(_ context.Context, src browsercookie.Source) (map[string]string, error) {
		switch src.ID {
		case "chrome/Default":
			return nil, errors.New("locked")
		case "chrome/Profile 1":
			return map[string]string{"GCID": "only-gcid"}, nil
		case "edge/Default":
			return map[string]string{"GCID": "old", "GCESS": "expired"}, nil
		}
		return map[string]string{"GCID": " good-id ", "GCESS": "good-ess"}, nil
	}
	authCookies = func(cs []*http.Cookie) error {
		for _, c := range cs {
			if c.Name == "GCID" && c.Value == "good-id" {
				return nil
			}
		}
		return errors.New("auth failed")
	}

	// a specific source that has no valid login
	if code, m := post(`{"id":"edge/Default"}`); code != http.StatusUnprocessableEntity || !strings.Contains(m["error"].(string), "Microsoft Edge: 登录信息已失效") {
		t.Fatalf("edge only: %d %v", code, m)
	}
	if s.currentClient() != nil {
		t.Fatal("failed login must not create a client")
	}

	// auto: falls through to the first valid source
	code, m := post(`{}`)
	if code != http.StatusOK || m["source"] != "Brave" {
		t.Fatalf("auto: %d %v", code, m)
	}
	cfg, _ := s.snapshotConfig()
	if cfg.Gcid != "good-id" || cfg.Gcess != "good-ess" || s.currentClient() == nil {
		t.Fatalf("not logged in: %q %q", cfg.Gcid, cfg.Gcess)
	}
	// cookie values never appear in responses
	rec := do(h, "POST", "/api/login/browser", "127.0.0.1:1234", "secret", `{}`)
	if strings.Contains(rec.Body.String(), "good-ess") {
		t.Fatal("cookie value leaked in response")
	}
	// requires the access token like every api
	if rec := do(h, "POST", "/api/login/browser", "127.0.0.1:1234", "", `{}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
}
