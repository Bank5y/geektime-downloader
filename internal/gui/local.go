package gui

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// localFileDTO is one output file of a downloaded article
type localFileDTO struct {
	Path   string `json:"path"` // relative to the course folder, slash separated
	Size   int64  `json:"size"`
	Exists bool   `json:"exists"`
}

// localArticleDTO is one downloaded article
type localArticleDTO struct {
	ID         int            `json:"id"`
	Title      string         `json:"title"`
	FinishedAt int64          `json:"finishedAt"`
	Legacy     bool           `json:"legacy"` // old record, files not known until synced
	Files      []localFileDTO `json:"files"`
}

// localCourseDTO is one course stored in the download folder
type localCourseDTO struct {
	ID        int               `json:"id"`
	Title     string            `json:"title"`
	TypeKey   string            `json:"typeKey"`
	TypeName  string            `json:"typeName"`
	Dir       string            `json:"dir"`
	Exists    bool              `json:"exists"` // course folder exists
	Total     int               `json:"total"`
	Marked    bool              `json:"marked"`
	UpdatedAt int64             `json:"updatedAt"`
	Size      int64             `json:"size"`
	Articles  []localArticleDTO `json:"articles"`
}

func (s *Server) handleLocal(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.snapshotConfig()
	rows, err := s.records.downloadedArticles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取下载记录失败: "+err.Error())
		return
	}
	meta := s.records.courseMeta()

	courses := make(map[int]*localCourseDTO)
	var order []int
	for _, a := range rows {
		c := courses[a.CourseID]
		if c == nil {
			m := meta[a.CourseID]
			c = &localCourseDTO{ID: a.CourseID, Title: a.CourseTitle, TypeKey: a.TypeKey, Dir: a.Dir,
				Total: m.Total, Marked: m.Marked, UpdatedAt: m.UpdatedAt, Articles: []localArticleDTO{}}
			c.TypeName = typeNameOf(a.TypeKey)
			if st, err := os.Stat(filepath.Join(cfg.DownloadFolder, a.Dir)); err == nil && st.IsDir() {
				c.Exists = true
			}
			courses[a.CourseID] = c
			order = append(order, a.CourseID)
		}
		art := localArticleDTO{ID: a.ArticleID, Title: a.Title, FinishedAt: a.FinishedAt, Legacy: a.Files == nil, Files: []localFileDTO{}}
		for _, f := range a.Files {
			fd := localFileDTO{Path: f}
			if st, err := os.Stat(filepath.Join(cfg.DownloadFolder, a.Dir, filepath.FromSlash(f))); err == nil {
				fd.Exists, fd.Size = true, st.Size()
				c.Size += st.Size()
			}
			art.Files = append(art.Files, fd)
		}
		c.Articles = append(c.Articles, art)
	}

	out := make([]localCourseDTO, 0, len(order))
	for _, id := range order {
		c := courses[id]
		sort.SliceStable(c.Articles, func(i, j int) bool { return c.Articles[i].ID < c.Articles[j].ID })
		out = append(out, *c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	writeJSON(w, http.StatusOK, map[string]interface{}{"root": cfg.DownloadFolder, "courses": out})
}

// typeNameOf returns the display name of a product type key
func typeNameOf(key string) string {
	if key == "" {
		return ""
	}
	o, ok := productTypeByKey(key)
	if !ok {
		return ""
	}
	if o.Index == autoIndex {
		return ""
	}
	if o.IsUniversity() {
		return "训练营"
	}
	return o.Text
}

// localPath resolves a course folder (and optionally a file in it) recorded
// in the download records; it never leaves the download folder
func (s *Server) localPath(courseID int, file string) (string, bool) {
	cfg, _ := s.snapshotConfig()
	dir := s.records.courseDir(courseID)
	if dir == "" {
		return "", false
	}
	root := filepath.Clean(cfg.DownloadFolder)
	p := filepath.Join(root, dir)
	if file != "" {
		p = filepath.Join(p, filepath.FromSlash(file))
	}
	if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return "", false
	}
	return p, true
}
