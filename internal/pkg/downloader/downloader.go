package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

const retryAttempts = 3

// DownloadFileConcurrently downloads url into filepath and returns the number of bytes written.
//
// When concurrency > 1 and the server announces the file size and byte range support,
// the file is split into `concurrency` chunks that are downloaded in parallel and written
// directly at their offsets. Otherwise it falls back to a single streamed GET.
func DownloadFileConcurrently(ctx context.Context, filepath string, url string, headers map[string]string, concurrency int) (int64, error) {
	if concurrency <= 1 {
		return downloadSingle(ctx, filepath, url, headers)
	}

	fileSize, acceptRanges, err := probe(ctx, url, headers)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 0, err
		}
		logger.Warnf("HEAD request failed, fallback to single connection download, url: %s, err: %v", url, err)
		return downloadSingle(ctx, filepath, url, headers)
	}
	if fileSize <= 0 || !acceptRanges {
		return downloadSingle(ctx, filepath, url, headers)
	}

	if int64(concurrency) > fileSize {
		concurrency = int(fileSize)
	}

	out, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		return 0, err
	}
	removeOnError := true
	defer func() {
		_ = out.Close()
		if removeOnError {
			_ = os.Remove(filepath)
		}
	}()
	if err := out.Truncate(fileSize); err != nil {
		return 0, err
	}

	g, gctx := errgroup.WithContext(ctx)
	chunkSize := fileSize / int64(concurrency)
	for i := 0; i < concurrency; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == concurrency-1 {
			end = fileSize - 1
		}
		g.Go(func() error {
			return downloadChunk(gctx, out, url, headers, start, end)
		})
	}
	if err := g.Wait(); err != nil {
		return 0, err
	}

	removeOnError = false
	return fileSize, nil
}

// probe sends a HEAD request and returns content length and whether byte ranges are supported.
func probe(ctx context.Context, url string, headers map[string]string) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, false, err
	}
	setHeaders(req, headers)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, false, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, false, fmt.Errorf("unexpected HEAD status %d", resp.StatusCode)
	}
	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	acceptRanges := strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")
	return size, acceptRanges, nil
}

// downloadChunk downloads bytes [start, end] and writes them at offset start.
func downloadChunk(ctx context.Context, out *os.File, url string, headers map[string]string, start, end int64) error {
	// fix error: http2: server sent GOAWAY and closed the connection; LastStreamID=1999
	// error comes from io read, not request
	return retry(ctx, retryAttempts, 700*time.Millisecond, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		setHeaders(req, headers)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode != http.StatusPartialContent {
			return fmt.Errorf("range request not honored, status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if int64(len(body)) != end-start+1 {
			return fmt.Errorf("short chunk read, want %d bytes, got %d", end-start+1, len(body))
		}
		_, err = out.WriteAt(body, start)
		return err
	})
}

// downloadSingle downloads url with one streamed GET request.
func downloadSingle(ctx context.Context, filepath string, url string, headers map[string]string) (int64, error) {
	var written int64
	err := retry(ctx, retryAttempts, 700*time.Millisecond, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		setHeaders(req, headers)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		out, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(out, resp.Body)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		written = n
		return nil
	})
	if err != nil {
		_ = os.Remove(filepath)
		return 0, err
	}
	return written, nil
}

func setHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func retry(ctx context.Context, attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// backoff but allow cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
			sleep *= 2

			logger.Infof("retry hanppen, times: %s", strconv.Itoa(i))
		}
		err = f()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("after %d attempts, last error: %w", attempts, err)
}
