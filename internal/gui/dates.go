package gui

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"golang.org/x/sync/errgroup"
)

const (
	outlineCacheTTL = 30 * time.Minute
	maxDatesPerCall = 40
)

// courseDates are publish times of the first and the newest article of a course
type courseDates struct {
	First int64 `json:"first,omitempty"` // unix seconds
	Last  int64 `json:"last,omitempty"`
	Count int   `json:"count,omitempty"`
}

type cachedDates struct {
	courseDates
	hasFirst bool
	at       time.Time
}

// outlineCache holds public course outline data
type outlineCache struct {
	mu    sync.Mutex
	dates map[int]cachedDates
}

func (c *outlineCache) getDates(id int, needFirst bool) (courseDates, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.dates[id]
	if !ok || time.Since(d.at) > outlineCacheTTL || (needFirst && !d.hasFirst) {
		return courseDates{}, false
	}
	return d.courseDates, true
}

func (c *outlineCache) putDates(id int, d courseDates, hasFirst bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dates == nil {
		c.dates = make(map[int]cachedDates)
	}
	if old, ok := c.dates[id]; ok && !hasFirst && old.hasFirst && time.Since(old.at) <= outlineCacheTTL {
		d.First, hasFirst = old.First, true
	}
	c.dates[id] = cachedDates{d, hasFirst, time.Now()}
}

func parseIDs(v string, max int) []int {
	var out []int
	seen := make(map[int]bool)
	for _, f := range strings.Split(v, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= max {
			break
		}
	}
	return out
}

// handleCourseDates returns the newest article time (and the first one for ids listed
// in "first") of column-like courses: GET /api/course/dates?ids=1,2&first=2
func (s *Server) handleCourseDates(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	ids := parseIDs(r.URL.Query().Get("ids"), maxDatesPerCall)
	needFirst := make(map[int]bool)
	for _, id := range parseIDs(r.URL.Query().Get("first"), maxDatesPerCall) {
		needFirst[id] = true
	}

	out := make(map[string]courseDates, len(ids))
	var mu sync.Mutex
	var firstErr error
	g := errgroup.Group{}
	g.SetLimit(4)
	for _, id := range ids {
		if d, ok := s.outline.getDates(id, needFirst[id]); ok {
			out[strconv.Itoa(id)] = d
			continue
		}
		g.Go(func() error {
			var d courseDates
			list, count, err := client.ArticleOutline(id, 1, true)
			if err == nil && len(list) > 0 {
				d.Last, d.Count = list[0].Ctime, count
				if needFirst[id] {
					if first, _, ferr := client.ArticleOutline(id, 1, false); ferr == nil && len(first) > 0 {
						d.First = first[0].Ctime
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return nil
			}
			s.outline.putDates(id, d, d.First != 0)
			out[strconv.Itoa(id)] = d
			return nil
		})
	}
	_ = g.Wait()
	if len(out) == 0 && firstErr != nil && (errors.Is(firstErr, geektime.ErrAuthFailed) || errors.Is(firstErr, geektime.ErrGeekTimeRateLimit)) {
		writeAPIError(w, firstErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"dates": out})
}
