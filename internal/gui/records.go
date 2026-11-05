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

	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

// courseDirName is the folder of a course inside the download folder
func courseDirName(title string) string {
	return filenamify.Filenamify(title)
}

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

// recordColumns are added to databases created before they existed
var recordColumns = []struct{ table, column, def string }{
	{"courses", "type_key", "TEXT NOT NULL DEFAULT ''"}, // product type key used to open the course
	{"courses", "dir", "TEXT NOT NULL DEFAULT ''"},      // course folder name in the download folder
	{"courses", "root", "TEXT NOT NULL DEFAULT ''"},     // download folder used, empty: the current one
	{"downloaded_articles", "title", "TEXT"},            // article title, NULL for old records
	// json array of output files relative to the course folder; NULL for old
	// records (unknown), [] when the article had nothing to download
	{"downloaded_articles", "files", "TEXT"},
}

func migrateRecords(db *sql.DB) error {
	for _, c := range recordColumns {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.column).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.column + ` ` + c.def); err != nil {
				return err
			}
		}
	}
	return nil
}

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
	if err := migrateRecords(db); err != nil {
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

// articleRecord is a finished download
type articleRecord struct {
	CourseID    int
	CourseTitle string
	TypeKey     string
	ArticleID   int
	Title       string
	Files       []string // output files relative to the course folder
	Root        string   // download folder the course folder is in
}

func (r *recordStore) addArticle(a articleRecord) {
	files := a.Files
	if files == nil {
		files = []string{}
	}
	b, _ := json.Marshal(files)
	r.withTx(func(tx *sql.Tx) error {
		now := time.Now().Unix()
		if err := upsertCourse(tx, a.CourseID, a.CourseTitle); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE courses SET updated_at = ?, dir = ?, root = ?,
			type_key = CASE WHEN ? <> '' THEN ? ELSE type_key END WHERE id = ?`,
			now, courseDirName(a.CourseTitle), a.Root, a.TypeKey, a.TypeKey, a.CourseID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO downloaded_articles (course_id, article_id, finished_at, title, files) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(course_id, article_id) DO UPDATE SET finished_at = excluded.finished_at, title = excluded.title, files = excluded.files`,
			a.CourseID, a.ArticleID, now, a.Title, string(b))
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

// storedArticle is one downloaded article with what is needed to verify it
type storedArticle struct {
	CourseID    int
	CourseTitle string
	TypeKey     string
	Dir         string // course folder name
	Root        string // download folder, empty: the current one
	ArticleID   int
	Title       string
	FinishedAt  int64
	Files       []string // nil when unknown (records written before files were stored)
}

// downloadedArticles lists all downloaded article records
func (r *recordStore) downloadedArticles() ([]storedArticle, error) {
	var out []storedArticle
	if r.db == nil {
		return out, nil
	}
	rows, err := r.db.Query(`SELECT a.course_id, c.title, c.type_key, c.dir, c.root, a.article_id, COALESCE(a.title, ''), a.finished_at, a.files
		FROM downloaded_articles a JOIN courses c ON c.id = a.course_id ORDER BY a.course_id, a.article_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var a storedArticle
		var files sql.NullString
		if err := rows.Scan(&a.CourseID, &a.CourseTitle, &a.TypeKey, &a.Dir, &a.Root, &a.ArticleID, &a.Title, &a.FinishedAt, &files); err != nil {
			return nil, err
		}
		if files.Valid {
			a.Files = []string{}
			if err := json.Unmarshal([]byte(files.String), &a.Files); err != nil {
				a.Files = nil // unreadable, treat as unknown
			}
		}
		if a.Dir == "" {
			a.Dir = courseDirName(a.CourseTitle)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// courseInfo is the stored state of a course
type courseInfo struct {
	Total     int
	Marked    bool
	UpdatedAt int64
}

// courseMeta returns total / mark / update time of all recorded courses
func (r *recordStore) courseMeta() map[int]courseInfo {
	out := make(map[int]courseInfo)
	if r.db == nil {
		return out
	}
	rows, err := r.db.Query(`SELECT id, total, marked, updated_at FROM courses`)
	if err != nil {
		logger.Warnf("Failed to read download records: %v", err)
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var c courseInfo
		if err := rows.Scan(&id, &c.Total, &c.Marked, &c.UpdatedAt); err == nil {
			out[id] = c
		}
	}
	return out
}

// courseDir returns the download folder (empty: the current one) and the
// folder name of a recorded course; dir is empty if the course is unknown
func (r *recordStore) courseDir(courseID int) (root, dir string) {
	if r.db == nil {
		return "", ""
	}
	var title string
	if err := r.db.QueryRow(`SELECT root, dir, title FROM courses WHERE id = ?`, courseID).Scan(&root, &dir, &title); err != nil {
		return "", ""
	}
	if dir == "" {
		dir = courseDirName(title)
	}
	return root, dir
}

// removeArticle deletes one downloaded article record
func (r *recordStore) removeArticle(courseID, articleID int) {
	r.exec(`DELETE FROM downloaded_articles WHERE course_id = ? AND article_id = ?`, courseID, articleID)
}

// setArticleFiles updates title and files of a downloaded article record
func (r *recordStore) setArticleFiles(courseID, articleID int, title string, files []string) {
	b, _ := json.Marshal(files)
	r.exec(`UPDATE downloaded_articles SET title = CASE WHEN ? <> '' THEN ? ELSE title END, files = ?
		WHERE course_id = ? AND article_id = ?`, title, title, string(b), courseID, articleID)
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
