// Package progress lets long running downloads report their progress to
// whoever started them (terminal progress bar, GUI task manager, ...)
// without the download code knowing about the consumer.
package progress

import "context"

// Reporter receives progress updates of a download.
// Implementations must be safe for concurrent use.
type Reporter interface {
	// SetStage describes what is currently being done, e.g. "下载视频", "生成 PDF".
	SetStage(stage string)
	// SetTotal sets the total number of bytes expected for the current stage.
	SetTotal(total int64)
	// Add reports n more bytes downloaded.
	Add(n int64)
}

type ctxKey struct{}

// WithReporter returns a copy of ctx carrying r.
func WithReporter(ctx context.Context, r Reporter) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// FromContext returns the Reporter stored in ctx, or nil.
func FromContext(ctx context.Context) Reporter {
	r, _ := ctx.Value(ctxKey{}).(Reporter)
	return r
}

// Stage is a nil-safe helper to call SetStage on the reporter in ctx.
func Stage(ctx context.Context, stage string) {
	if r := FromContext(ctx); r != nil {
		r.SetStage(stage)
	}
}
