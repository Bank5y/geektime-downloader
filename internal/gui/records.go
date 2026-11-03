package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

// courseRecord remembers what has been downloaded for one course
type courseRecord struct {
	Title     string           `json:"title"`
	Total     int              `json:"total"`
	Articles  map[string]int64 `json:"articles"` // article id -> finished unix time
	Marked    bool             `json:"marked"`   // manually marked as downloaded
	UpdatedAt int64            `json:"updatedAt"`
}

// markDTO is the download state of a course sent to the browser
type markDTO struct {
	Done   int  `json:"done"`
	Total  int  `json:"total"`
	Marked bool `json:"marked"`
}

// recordStore persists download records as a small json file.
// Courses are keyed by the id used to open them (class id for university).
type recordStore struct {
	mu   sync.Mutex
	path string
	data map[string]*courseRecord
}

func newRecordStore(path string) *recordStore {
	r := &recordStore{path: path, data: make(map[string]*courseRecord)}
	if path == "" {
		return r
	}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &r.data); err != nil {
			logger.Warnf("Failed to parse download records %s: %v", path, err)
			r.data = make(map[string]*courseRecord)
		}
	}
	return r
}

func defaultRecordPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "geektime-downloader", "downloads.json")
}

func (r *recordStore) get(courseID int, title string) *courseRecord {
	k := strconv.Itoa(courseID)
	rec := r.data[k]
	if rec == nil {
		rec = &courseRecord{Articles: make(map[string]int64)}
		r.data[k] = rec
	}
	if rec.Articles == nil {
		rec.Articles = make(map[string]int64)
	}
	if title != "" {
		rec.Title = title
	}
	return rec
}

// setTotal remembers the number of articles of a course, only for courses that have a record
// or when create is set
func (r *recordStore) setTotal(courseID int, title string, total int, create bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[strconv.Itoa(courseID)]; !ok && !create {
		return
	}
	rec := r.get(courseID, title)
	if rec.Total == total {
		return
	}
	rec.Total = total
	r.saveLocked()
}

func (r *recordStore) addArticle(courseID int, title string, articleID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.get(courseID, title)
	now := time.Now().Unix()
	rec.Articles[strconv.Itoa(articleID)] = now
	rec.UpdatedAt = now
	r.saveLocked()
}

func (r *recordStore) setMarked(courseID int, title string, marked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.get(courseID, title)
	rec.Marked = marked
	rec.UpdatedAt = time.Now().Unix()
	r.saveLocked()
}

func (r *recordStore) marks() map[string]markDTO {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]markDTO, len(r.data))
	for k, rec := range r.data {
		if len(rec.Articles) == 0 && !rec.Marked {
			continue
		}
		out[k] = markDTO{Done: len(rec.Articles), Total: rec.Total, Marked: rec.Marked}
	}
	return out
}

// articles returns downloaded article ids and the manual mark of a course
func (r *recordStore) articles(courseID int) ([]int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.data[strconv.Itoa(courseID)]
	if rec == nil {
		return []int{}, false
	}
	ids := make([]int, 0, len(rec.Articles))
	for k := range rec.Articles {
		if id, err := strconv.Atoi(k); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, rec.Marked
}

func (r *recordStore) saveLocked() {
	if r.path == "" {
		return
	}
	b, err := json.Marshal(r.data)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		logger.Warnf("Failed to create record dir: %v", err)
		return
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		logger.Warnf("Failed to write download records: %v", err)
		return
	}
	if err := os.Rename(tmp, r.path); err != nil {
		logger.Warnf("Failed to save download records: %v", err)
	}
}
