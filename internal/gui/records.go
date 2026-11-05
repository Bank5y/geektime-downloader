package gui

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // pure Go driver, keeps CGO_ENABLED=0 cross builds working

	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

// markDTO is the download state of a course sent to the browser
type markDTO struct {
	Done   int  `json:"done"`
	Total  int  `json:"total"`
	Marked bool `json:"marked"`
}

const recordSchema = `
CREATE TABLE IF NOT EXISTS courses (
	id         INTEGER PRIMARY KEY, -- id used to open the course (class id for university)
	title      TEXT    NOT NULL DEFAULT '',
	total      INTEGER NOT NULL DEFAULT 0,
	marked     INTEGER NOT NULL DEFAULT 0, -- manually marked as downloaded
	updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS downloaded_articles (
	course_id   INTEGER NOT NULL,
	article_id  INTEGER NOT NULL,
	finished_at INTEGER NOT NULL,
	PRIMARY KEY (course_id, article_id)
);`

// recordStore persists download records in a SQLite database.
// Failures are logged and never break downloads.
type recordStore struct {
	db *sql.DB
}

// newRecordStore opens (or creates) the database at path; an empty path
// keeps records in memory only.
func newRecordStore(path string) *recordStore {
	r, err := openRecordStore(path)
	if err == nil {
		return r
	}
	logger.Warnf("Failed to open download records %s, falling back to memory: %v", path, err)
	r, err = openRecordStore("")
	if err != nil {
		logger.Errorf(err, "Failed to open in-memory download records")
		return &recordStore{}
	}
	return r
}

func openRecordStore(path string) (*recordStore, error) {
	dsn := ":memory:"
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		dsn = "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// a single connection serializes writes and keeps :memory: one database
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(recordSchema); err != nil {
		_ = db.Close()
		return nil, err
	}
	r := &recordStore{db: db}
	if path != "" {
		r.importLegacyJSON(filepath.Join(filepath.Dir(path), "downloads.json"))
	}
	return r, nil
}

func defaultRecordPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "geektime-downloader", "downloads.db")
}

func (r *recordStore) exec(query string, args ...interface{}) {
	if r.db == nil {
		return
	}
	if _, err := r.db.Exec(query, args...); err != nil {
		logger.Warnf("Failed to save download records: %v", err)
	}
}

// upsertCourse creates the course row if missing and updates its title
func upsertCourse(ex interface {
	Exec(string, ...interface{}) (sql.Result, error)
}, courseID int, title string) error {
	_, err := ex.Exec(`INSERT INTO courses (id, title) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET title = CASE WHEN excluded.title <> '' THEN excluded.title ELSE title END`,
		courseID, title)
	return err
}

// setTotal remembers the number of articles of a course, only for courses that have a record
// or when create is set
func (r *recordStore) setTotal(courseID int, title string, total int, create bool) {
	if r.db == nil {
		return
	}
	if create {
		if err := upsertCourse(r.db, courseID, title); err != nil {
			logger.Warnf("Failed to save download records: %v", err)
			return
		}
	}
	r.exec(`UPDATE courses SET total = ?, title = CASE WHEN ? <> '' THEN ? ELSE title END WHERE id = ?`,
		total, title, title, courseID)
}

func (r *recordStore) addArticle(courseID int, title string, articleID int) {
	r.withTx(func(tx *sql.Tx) error {
		now := time.Now().Unix()
		if err := upsertCourse(tx, courseID, title); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE courses SET updated_at = ? WHERE id = ?`, now, courseID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO downloaded_articles (course_id, article_id, finished_at) VALUES (?, ?, ?)
			ON CONFLICT(course_id, article_id) DO UPDATE SET finished_at = excluded.finished_at`,
			courseID, articleID, now)
		return err
	})
}

func (r *recordStore) setMarked(courseID int, title string, marked bool) {
	r.withTx(func(tx *sql.Tx) error {
		if err := upsertCourse(tx, courseID, title); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE courses SET marked = ?, updated_at = ? WHERE id = ?`, marked, time.Now().Unix(), courseID)
		return err
	})
}

func (r *recordStore) withTx(fn func(tx *sql.Tx) error) {
	if r.db == nil {
		return
	}
	tx, err := r.db.Begin()
	if err == nil {
		if err = fn(tx); err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	if err != nil {
		logger.Warnf("Failed to save download records: %v", err)
	}
}

// marks returns the download state of courses that have downloads or a mark
func (r *recordStore) marks() map[string]markDTO {
	out := make(map[string]markDTO)
	if r.db == nil {
		return out
	}
	rows, err := r.db.Query(`SELECT c.id, c.total, c.marked, COUNT(a.article_id)
		FROM courses c LEFT JOIN downloaded_articles a ON a.course_id = c.id
		GROUP BY c.id HAVING COUNT(a.article_id) > 0 OR c.marked`)
	if err != nil {
		logger.Warnf("Failed to read download records: %v", err)
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var m markDTO
		if err := rows.Scan(&id, &m.Total, &m.Marked, &m.Done); err != nil {
			logger.Warnf("Failed to read download records: %v", err)
			return out
		}
		out[strconv.Itoa(id)] = m
	}
	return out
}

// articles returns downloaded article ids and the manual mark of a course
func (r *recordStore) articles(courseID int) ([]int, bool) {
	ids := []int{}
	if r.db == nil {
		return ids, false
	}
	var marked bool
	err := r.db.QueryRow(`SELECT marked FROM courses WHERE id = ?`, courseID).Scan(&marked)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Warnf("Failed to read download records: %v", err)
	}
	rows, err := r.db.Query(`SELECT article_id FROM downloaded_articles WHERE course_id = ? ORDER BY article_id`, courseID)
	if err != nil {
		logger.Warnf("Failed to read download records: %v", err)
		return ids, marked
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, marked
}

// recordCourse is a course that has a record
type recordCourse struct {
	ID    int
	Title string
}

// courses lists all courses that have a record
func (r *recordStore) courses() []recordCourse {
	var out []recordCourse
	if r.db == nil {
		return out
	}
	rows, err := r.db.Query(`SELECT id, title FROM courses ORDER BY id`)
	if err != nil {
		logger.Warnf("Failed to read download records: %v", err)
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c recordCourse
		if err := rows.Scan(&c.ID, &c.Title); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// syncArticles makes the downloaded articles of a course exactly ids, keeping
// the finish time of articles already recorded and the manual mark. total < 0
// keeps the stored total. A course without records is only created when ids
// is not empty.
func (r *recordStore) syncArticles(courseID int, title string, total int, ids []int) (added, removed int) {
	r.withTx(func(tx *sql.Tx) error {
		added, removed = 0, 0
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM courses WHERE id = ?)`, courseID).Scan(&exists); err != nil {
			return err
		}
		if !exists && len(ids) == 0 {
			return nil
		}
		if err := upsertCourse(tx, courseID, title); err != nil {
			return err
		}
		if total >= 0 {
			if _, err := tx.Exec(`UPDATE courses SET total = ? WHERE id = ?`, total, courseID); err != nil {
				return err
			}
		}

		keep := make(map[int]bool, len(ids))
		for _, id := range ids {
			keep[id] = true
		}
		rows, err := tx.Query(`SELECT article_id FROM downloaded_articles WHERE course_id = ?`, courseID)
		if err != nil {
			return err
		}
		have := make(map[int]bool)
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			have[id] = true
		}
		_ = rows.Close()

		now := time.Now().Unix()
		for id := range have {
			if keep[id] {
				continue
			}
			if _, err := tx.Exec(`DELETE FROM downloaded_articles WHERE course_id = ? AND article_id = ?`, courseID, id); err != nil {
				return err
			}
			removed++
		}
		for id := range keep {
			if have[id] {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO downloaded_articles (course_id, article_id, finished_at) VALUES (?, ?, ?)`, courseID, id, now); err != nil {
				return err
			}
			added++
		}
		if added+removed > 0 {
			_, err = tx.Exec(`UPDATE courses SET updated_at = ? WHERE id = ?`, now, courseID)
		}
		return err
	})
	return added, removed
}

// legacyCourseRecord is one course of the old downloads.json
type legacyCourseRecord struct {
	Title     string           `json:"title"`
	Total     int              `json:"total"`
	Articles  map[string]int64 `json:"articles"` // article id -> finished unix time
	Marked    bool             `json:"marked"`
	UpdatedAt int64            `json:"updatedAt"`
}

// importLegacyJSON moves records of the old json file into the database once,
// then renames the file to *.bak so it is not imported again.
func (r *recordStore) importLegacyJSON(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var data map[string]*legacyCourseRecord
	if err := json.Unmarshal(b, &data); err != nil {
		logger.Warnf("Failed to parse legacy download records %s: %v", path, err)
		return
	}
	tx, err := r.db.Begin()
	if err != nil {
		logger.Warnf("Failed to import legacy download records: %v", err)
		return
	}
	err = func() error {
		for k, rec := range data {
			id, err := strconv.Atoi(k)
			if err != nil || rec == nil {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO courses (id, title, total, marked, updated_at) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING`, id, rec.Title, rec.Total, rec.Marked, rec.UpdatedAt); err != nil {
				return err
			}
			for ak, at := range rec.Articles {
				aid, err := strconv.Atoi(ak)
				if err != nil {
					continue
				}
				if _, err := tx.Exec(`INSERT INTO downloaded_articles (course_id, article_id, finished_at) VALUES (?, ?, ?)
					ON CONFLICT(course_id, article_id) DO NOTHING`, id, aid, at); err != nil {
					return err
				}
			}
		}
		return nil
	}()
	if err == nil {
		err = tx.Commit()
	} else {
		_ = tx.Rollback()
	}
	if err != nil {
		logger.Warnf("Failed to import legacy download records: %v", err)
		return
	}
	if err := os.Rename(path, path+".bak"); err != nil {
		logger.Warnf("Failed to rename legacy download records: %v", err)
	}
	logger.Infof("Imported %d courses from legacy download records %s", len(data), path)
}
