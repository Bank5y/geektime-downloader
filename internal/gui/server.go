// Package gui provides a local web based graphical interface.
//
// The server only listens on the loopback interface by default and every API
// request must carry a random token which is handed to the browser on startup,
// so other local web pages cannot drive the downloader (CSRF / DNS rebinding).
package gui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/config"
	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

//go:embed web
var webFS embed.FS

const tokenHeader = "X-GD-Token"

// Options for starting the GUI server
type Options struct {
	Addr        string
	OpenBrowser bool
}

// Server is the GUI http server and holds all GUI state.
type Server struct {
	ctx   context.Context
	token string

	mu       sync.RWMutex
	cfg      config.AppConfig // cookies + base options from command line flags
	settings Settings
	client   *geektime.Client

	cacheMu     sync.Mutex
	products    []productDTO
	catalogMeta *catalogMeta
	introCache  map[string]introDTO
	outline     outlineCache // public article dates
	courseCache map[string]geektime.Course

	browserLogin sync.Mutex // one browser cookie import at a time

	tasks   *TaskManager
	records *recordStore

	port         string
	checkHost    bool
	allowedHosts map[string]bool
}

// Run starts the GUI server and blocks until ctx is canceled.
func Run(ctx context.Context, cfg config.AppConfig, opts Options) error {
	token, err := randomToken()
	if err != nil {
		return err
	}

	s := &Server{
		ctx:         ctx,
		token:       token,
		cfg:         cfg,
		settings:    settingsFromConfig(cfg),
		courseCache: make(map[string]geektime.Course),
		introCache:  make(map[string]introDTO),
		records:     newRecordStore(defaultRecordPath()),
	}
	s.tasks = newTaskManager(ctx, s)

	if cfg.Gcid != "" && cfg.Gcess != "" {
		s.client = geektime.NewClient(config.ReadCookiesFromInput(&cfg))
	}

	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return err
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	s.port = port
	s.checkHost = isLoopback(host)
	s.allowedHosts = map[string]bool{
		"127.0.0.1:" + port: true,
		"localhost:" + port: true,
		"[::1]:" + port:     true,
	}

	browseHost := host
	if !s.checkHost || browseHost == "" || browseHost == "::" || browseHost == "0.0.0.0" {
		browseHost = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s/#token=%s", net.JoinHostPort(browseHost, port), token)

	httpServer := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("图形界面已启动, 请在浏览器中打开:\n  %s\n按 Ctrl+C 退出\n", url)
	logger.Infof("GUI server listening on %s", ln.Addr().String())

	if opts.OpenBrowser {
		go func() {
			if err := openURL(url); err != nil {
				fmt.Printf("自动打开浏览器失败, 请手动打开上面的地址: %v\n", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		s.tasks.cancelAll()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		s.tasks.wait(5 * time.Second)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServer(http.FS(sub)))

	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("GET /api/login/browsers", s.handleBrowserSources)
	mux.HandleFunc("POST /api/login/browser", s.handleBrowserLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/products", s.handleProducts)
	mux.HandleFunc("GET /api/catalog/meta", s.handleCatalogMeta)
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/course", s.handleCourse)
	mux.HandleFunc("GET /api/course/intro", s.handleCourseIntro)
	mux.HandleFunc("GET /api/course/dates", s.handleCourseDates)
	mux.HandleFunc("GET /api/marks", s.handleMarks)
	mux.HandleFunc("POST /api/unlock", s.handleUnlock)
	mux.HandleFunc("POST /api/marks", s.handleSetMark)
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTasks)
	mux.HandleFunc("POST /api/tasks/action", s.handleTaskAction)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("POST /api/open", s.handleOpen)

	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.checkHost && !s.allowedHosts[strings.ToLower(r.Host)] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			got := r.Header.Get(tokenHeader)
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeError(w, http.StatusUnauthorized, "无效的访问令牌, 请使用启动时输出的地址重新打开页面")
				return
			}
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// currentClient returns the logged in client or nil
func (s *Server) currentClient() *geektime.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// snapshotConfig builds an AppConfig from cookies and current settings.
// Each task gets its own copy so settings changes don't affect running tasks.
func (s *Server) snapshotConfig() (config.AppConfig, Settings) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.apply(s.cfg), s.settings
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	return dec.Decode(v)
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
