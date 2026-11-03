package video

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/pkg/progress"
)

type recordReporter struct {
	mu    sync.Mutex
	total int64
	done  int64
}

func (r *recordReporter) SetStage(string) {}
func (r *recordReporter) SetTotal(t int64) {
	r.mu.Lock()
	r.total = t
	r.mu.Unlock()
}
func (r *recordReporter) Add(n int64) {
	r.mu.Lock()
	r.done += n
	r.mu.Unlock()
}

func TestDownloadSegmentsParallelKeepsPlaylistOrder(t *testing.T) {
	// names deliberately not lexically ordered to make sure merge follows playlist order
	names := []string{"seg-10.ts", "seg-9.ts", "seg-2.ts", "seg-1.ts", "seg-100.ts"}
	segs := make(map[string][]byte)
	var want bytes.Buffer
	for i, n := range names {
		b := bytes.Repeat([]byte{byte('a' + i)}, 1000+i)
		segs[n] = b
		want.Write(b)
	}
	rnd := rand.New(rand.NewSource(1))
	var rndMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// random latency so segments finish out of order
		rndMu.Lock()
		d := time.Duration(rnd.Intn(30)) * time.Millisecond
		rndMu.Unlock()
		time.Sleep(d)
		b, ok := segs[strings.TrimPrefix(r.URL.Path, "/v/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	dir := t.TempDir()
	rep := &recordReporter{}
	ctx := progress.WithReporter(context.Background(), rep)
	if err := download(ctx, srv.URL+"/v/", "title", dir, names, nil, int64(want.Len()), false, 4); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "title"+TSExtension))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatal("merged video is not in playlist order")
	}
	if rep.total != int64(want.Len()) || rep.done != int64(want.Len()) {
		t.Fatalf("progress total=%d done=%d, want %d", rep.total, rep.done, want.Len())
	}
	if _, err := os.Stat(filepath.Join(dir, "title")); !os.IsNotExist(err) {
		t.Fatal("temp dir should be removed")
	}
}

func TestDownloadSegmentsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "bad.ts") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	dir := t.TempDir()
	ctx := progress.WithReporter(context.Background(), &recordReporter{})
	err := download(ctx, srv.URL+"/", "title", dir, []string{"a.ts", "bad.ts", "c.ts"}, nil, 6, false, 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Join(dir, "title"+TSExtension)); !os.IsNotExist(err) {
		t.Fatal("final video should not exist on failure")
	}
}
