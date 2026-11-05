package gui

import (
	"context"
	"net/http"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/browsercookie"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

const geektimeCookieURL = "https://time.geekbang.org/"

// replaced in tests
var (
	browserSources     = browsercookie.Sources
	readBrowserCookies = func(ctx context.Context, src browsercookie.Source) (map[string]string, error) {
		return browsercookie.Read(ctx, src, geektimeCookieURL, "GCID", "GCESS")
	}
	authCookies = geektime.Auth
)

// handleBrowserSources lists the local browser profiles quick login can read from
func (s *Server) handleBrowserSources(w http.ResponseWriter, r *http.Request) {
	list := browserSources()
	if list == nil {
		list = []browsercookie.Source{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sources": list})
}

// handleBrowserLogin reads GCID / GCESS of time.geekbang.org from the local browsers
// and logs in with the first pair that passes verification. The cookie values never
// leave the server.
func (s *Server) handleBrowserLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"` // optional, a source id from /api/login/browsers
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if !s.browserLogin.TryLock() {
		writeError(w, http.StatusConflict, "正在读取浏览器登录信息, 请稍候")
		return
	}
	defer s.browserLogin.Unlock()

	var sources []browsercookie.Source
	for _, src := range browserSources() {
		if req.ID == "" || src.ID == req.ID {
			sources = append(sources, src)
		}
	}
	if len(sources) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "未在本机找到 Chrome / Edge / Brave / Chromium 浏览器的用户数据, 请手动填写 Cookie")
		return
	}

	var reasons []string
	for _, src := range sources {
		cookies, err := readBrowserCookies(r.Context(), src)
		if err != nil {
			logger.Warnf("quick login: read cookies from %s failed: %v", src, err)
			reasons = append(reasons, src.String()+": "+err.Error())
			continue
		}
		gcid, gcess := strings.TrimSpace(cookies["GCID"]), strings.TrimSpace(cookies["GCESS"])
		if gcid == "" || gcess == "" {
			reasons = append(reasons, src.String()+": 未登录极客时间")
			continue
		}
		if err := s.applyLogin(gcid, gcess); err != nil {
			reasons = append(reasons, src.String()+": 登录信息已失效 ("+err.Error()+")")
			continue
		}
		logger.Infof("quick login: logged in with cookies from %s", src)
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "source": src.String()})
		return
	}
	writeError(w, http.StatusUnprocessableEntity, "未能从浏览器获取有效的登录信息 — "+strings.Join(reasons, "; "))
}
