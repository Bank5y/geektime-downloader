package gui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
)

// localExists reports whether a non-empty course folder exists in the download folder
func (s *Server) localExists(title string) bool {
	if strings.TrimSpace(title) == "" {
		return false
	}
	cfg, _ := s.snapshotConfig()
	entries, err := os.ReadDir(filepath.Join(cfg.DownloadFolder, filenamify.Filenamify(title)))
	return err == nil && len(entries) > 0
}

// withLocal returns a copy of products with the Local flag computed
func (s *Server) withLocal(list []productDTO) []productDTO {
	out := make([]productDTO, len(list))
	for i, p := range list {
		p.Local = s.localExists(p.Title)
		out[i] = p
	}
	return out
}

func (s *Server) handleMarks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"marks": s.records.marks()})
}

func (s *Server) handleSetMark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CourseID int    `json:"courseId"`
		Title    string `json:"title"`
		Marked   bool   `json:"marked"`
	}
	if err := readJSON(w, r, &req); err != nil || req.CourseID <= 0 {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	s.records.setMarked(req.CourseID, req.Title, req.Marked)
	writeJSON(w, http.StatusOK, map[string]interface{}{"marks": s.records.marks()})
}
