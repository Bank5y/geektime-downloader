package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/config"
	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
)

func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	cfg := config.AppConfig{
		DownloadFolder:         t.TempDir(),
		Quality:                "sd",
		ColumnOutputType:       1,
		DownloadComments:       1,
		PrintPDFWaitSeconds:    5,
		PrintPDFTimeoutSeconds: 60,
		Interval:               1,
		Threads:                5,
		LogLevel:               "none",
	}
	s := &Server{
		ctx:          context.Background(),
		token:        "secret",
		cfg:          cfg,
		settings:     settingsFromConfig(cfg),
		courseCache:  make(map[string]geektime.Course),
		introCache:   make(map[string]introDTO),
		checkHost:    true,
		allowedHosts: map[string]bool{"127.0.0.1:1234": true},
	}
	s.tasks = newTaskManager(context.Background(), s)
	return s, s.routes()
}

func do(h http.Handler, method, path, host, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = host
	if token != "" {
		req.Header.Set(tokenHeader, token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMiddleware(t *testing.T) {
	_, h := newTestServer(t)

	if rec := do(h, "GET", "/api/state", "127.0.0.1:1234", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/state", "127.0.0.1:1234", "wrong", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: got %d", rec.Code)
	}
	// DNS rebinding: foreign host header must be rejected even with valid token
	if rec := do(h, "GET", "/api/state", "evil.example.com:1234", "secret", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign host: got %d", rec.Code)
	}
	rec := do(h, "GET", "/api/state", "127.0.0.1:1234", "secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("valid: got %d", rec.Code)
	}
	var st struct {
		LoggedIn bool             `json:"loggedIn"`
		Types    []productTypeDTO `json:"types"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.LoggedIn || len(st.Types) != 8 {
		t.Fatalf("unexpected state: %+v", st)
	}
	// static assets don't need token
	if rec := do(h, "GET", "/app.js", "127.0.0.1:1234", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("static: got %d", rec.Code)
	}
}

func TestSettingsValidation(t *testing.T) {
	s, h := newTestServer(t)
	_, st := s.snapshotConfig()

	st.Threads = 64
	b, _ := json.Marshal(st)
	if rec := do(h, "PUT", "/api/settings", "127.0.0.1:1234", "secret", string(b)); rec.Code != http.StatusBadRequest {
		t.Fatalf("threads 64 should be rejected, got %d", rec.Code)
	}

	st.Threads = 16
	st.ConcurrentTasks = 3
	b, _ = json.Marshal(st)
	if rec := do(h, "PUT", "/api/settings", "127.0.0.1:1234", "secret", string(b)); rec.Code != http.StatusOK {
		t.Fatalf("valid settings rejected: %d %s", rec.Code, rec.Body.String())
	}
	cfg, got := s.snapshotConfig()
	if got.ConcurrentTasks != 3 || cfg.Threads != 16 {
		t.Fatalf("settings not applied: %+v", got)
	}
}

func TestRequiresLogin(t *testing.T) {
	_, h := newTestServer(t)
	for _, p := range []string{"/api/products", "/api/course?type=n0&id=1"} {
		if rec := do(h, "GET", p, "127.0.0.1:1234", "secret", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: got %d", p, rec.Code)
		}
	}
}

func TestProductTypeKeys(t *testing.T) {
	for _, pt := range productTypes() {
		o, ok := productTypeByKey(pt.Key)
		if !ok || typeKeyOf(o) != pt.Key {
			t.Fatalf("round trip failed for %s", pt.Key)
		}
	}
	if _, ok := productTypeByKey("x1"); ok {
		t.Fatal("unknown key accepted")
	}
	o, _ := productTypeByKey("n4")
	if !o.IsUniversity() {
		t.Fatal("n4 should be university")
	}
}

func TestTaskManagerQueueAndCancel(t *testing.T) {
	s, _ := newTestServer(t)
	// not logged in: tasks fail fast with "未登录" instead of hitting the network
	opt, _ := productTypeByKey("n0")
	tasks := []*Task{
		{CourseID: 1, ArticleID: 1, Title: "a", TypeKey: "n0", option: opt},
		{CourseID: 1, ArticleID: 2, Title: "b", TypeKey: "n0", option: opt},
	}
	if n := s.tasks.add(tasks); n != 2 {
		t.Fatalf("added %d", n)
	}
	s.tasks.wait(5e9)
	for _, tk := range s.tasks.list() {
		if tk.Status != StatusFailed || tk.Error != "未登录" {
			t.Fatalf("unexpected task state: %+v", tk)
		}
	}
	// re-adding finished tasks re-queues them rather than duplicating
	if n := s.tasks.add([]*Task{{CourseID: 1, ArticleID: 1, Title: "a", TypeKey: "n0", option: opt}}); n != 1 {
		t.Fatalf("re-add: %d", n)
	}
	s.tasks.wait(5e9)
	if got := len(s.tasks.list()); got != 2 {
		t.Fatalf("expected 2 tasks, got %d", got)
	}
	s.tasks.clearFinished()
	s.tasks.remove([]int64{1, 2})
	if got := len(s.tasks.list()); got != 0 {
		t.Fatalf("expected 0 tasks, got %d", got)
	}
}

func TestClassifyProduct(t *testing.T) {
	var p response.LearnProduct
	p.ID, p.Type, p.IsUniversity, p.University.ClassID = 100826402, "u21", true, 746
	dto := toProductDTO(p, 0)
	if dto.TypeKey != "n4" || dto.CourseID != 746 || !dto.Openable || dto.Direct {
		t.Fatalf("university product: %+v", dto)
	}

	p = response.LearnProduct{ID: 1, Type: "u27"} // university without class id
	if dto := toProductDTO(p, 0); dto.Openable {
		t.Fatalf("university without class id should not be openable: %+v", dto)
	}

	cases := map[string]struct {
		key    string
		direct bool
	}{
		"c1": {"n0", false}, "c3": {"n0", false}, "d": {"n1", true}, "q": {"n3", true},
		"p35": {"n2", false}, "c6": {"n5", false}, "x50": {autoKey, false},
	}
	for code, want := range cases {
		dto := toProductDTO(response.LearnProduct{ID: 7, Type: code}, 0)
		if dto.TypeKey != want.key || dto.Direct != want.direct || dto.CourseID != 7 || !dto.Openable {
			t.Fatalf("%s: %+v", code, dto)
		}
	}
}

func TestBuildLabelTree(t *testing.T) {
	var res response.V3LabelListResponse
	_ = json.Unmarshal([]byte(`{"code":0,"data":{"list":[
		{"id":3,"name":"后端","pid":0,"count":9,"sort":96},
		{"id":101,"name":"AI","pid":0,"count":3,"sort":98},
		{"id":11,"name":"Go","pid":3,"count":2,"sort":1},
		{"id":999,"name":"orphan","pid":12345,"count":1,"sort":1}]}}`), &res)
	tree := buildLabelTree(res)
	if len(tree) != 2 || tree[0].ID != 101 || tree[1].ID != 3 || len(tree[1].Children) != 1 || tree[1].Children[0].ID != 11 {
		t.Fatalf("unexpected tree: %+v", tree)
	}
}
