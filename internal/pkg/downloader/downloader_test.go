package downloader

import (
	"bytes"
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testPayload(n int) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(1)).Read(b)
	return b
}

func TestDownloadFileConcurrently_Ranges(t *testing.T) {
	payload := testPayload(1<<20 + 123)
	var rangeRequests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "1" {
			t.Errorf("custom header not propagated to %s request", r.Method)
		}
		if r.Header.Get("Range") != "" {
			atomic.AddInt32(&rangeRequests, 1)
		}
		// ServeContent handles HEAD, Accept-Ranges and Range
		http.ServeContent(w, r, "f", time.Time{}, bytes.NewReader(payload))
	}))
	defer srv.Close()

	for _, c := range []int{1, 2, 7, 32} {
		dst := filepath.Join(t.TempDir(), "out")
		atomic.StoreInt32(&rangeRequests, 0)
		n, err := DownloadFileConcurrently(context.Background(), dst, srv.URL, map[string]string{"X-Test": "1"}, c)
		if err != nil {
			t.Fatalf("concurrency %d: %v", c, err)
		}
		if n != int64(len(payload)) {
			t.Fatalf("concurrency %d: size %d, want %d", c, n, len(payload))
		}
		got, _ := os.ReadFile(dst)
		if !bytes.Equal(got, payload) {
			t.Fatalf("concurrency %d: content mismatch", c)
		}
		want := int32(c)
		if c == 1 {
			want = 0
		}
		if got := atomic.LoadInt32(&rangeRequests); got != want {
			t.Fatalf("concurrency %d: %d range requests, want %d", c, got, want)
		}
	}
}

func TestDownloadFileConcurrently_NoRangeSupportFallsBack(t *testing.T) {
	payload := testPayload(50000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no Accept-Ranges, ignores Range header
		w.Header().Set("Content-Length", "50000")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out")
	n, err := DownloadFileConcurrently(context.Background(), dst, srv.URL, nil, 8)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if n != int64(len(payload)) || !bytes.Equal(got, payload) {
		t.Fatal("content mismatch")
	}
}

func TestDownloadFileConcurrently_HeadNotAllowedFallsBack(t *testing.T) {
	payload := []byte(strings.Repeat("x", 1000))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out")
	if _, err := DownloadFileConcurrently(context.Background(), dst, srv.URL, nil, 4); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Fatal("content mismatch")
	}
}

func TestDownloadFileConcurrently_ErrorStatusRemovesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out")
	if _, err := DownloadFileConcurrently(context.Background(), dst, srv.URL, nil, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("partial file should be removed")
	}
}

func TestDownloadFileConcurrently_Canceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dst := filepath.Join(t.TempDir(), "out")
	start := time.Now()
	_, err := DownloadFileConcurrently(ctx, dst, srv.URL, nil, 4)
	if err == nil {
		t.Fatal("expected error")
	}
	if time.Since(start) > time.Second {
		t.Fatal("canceled download should return immediately, not retry")
	}
}
