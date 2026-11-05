package gui

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/audio"
	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/markdown"
	"github.com/nicoxiang/geektime-downloader/internal/pdf"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
	"github.com/nicoxiang/geektime-downloader/internal/video"
)

// syncCourseGap spaces out course info requests that are not cached yet
const syncCourseGap = 300 * time.Millisecond

// outputExts are the files that mean an article has been downloaded,
// any one of them is enough whatever the current output settings are
var outputExts = map[string]bool{
	markdown.MDExtension: true,
	pdf.PDFExtension:     true,
	audio.MP3Extension:   true,
	video.TSExtension:    true,
}

// syncResult summarizes a records sync for the browser
type syncResult struct {
	Courses    int      `json:"courses"`    // courses whose records were synced
	Added      int      `json:"added"`      // article records added because files exist
	Removed    int      `json:"removed"`    // article records removed because files are gone
	Unresolved []string `json:"unresolved"` // local course folders that could not be synced
	Aborted    string   `json:"aborted,omitempty"`
}

// localOutputs returns the file names (without extension) of downloaded
// articles below a course folder, nil if the folder does not exist.
func localOutputs(dir string) map[string]bool {
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	out := make(map[string]bool)
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// images of markdown and videos embedded in text articles
			if p != dir && (d.Name() == "images" || d.Name() == "videos") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if outputExts[ext] {
			out[strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))] = true
		}
		return nil
	})
	return out
}

// syncCandidate is a course whose records may need syncing
type syncCandidate struct {
	id      int
	title   string
	typeKey string // empty when unknown
	direct  bool   // one video product (每日一课 etc.), not a list of articles
}

func (s *Server) handleSyncRecords(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	products, err := s.myProducts(client)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	res, err := s.syncRecords(client, products)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": res, "marks": s.records.marks()})
}

// myProducts returns the cached product list, loading it when needed
func (s *Server) myProducts(client *geektime.Client) ([]productDTO, error) {
	s.cacheMu.Lock()
	cached := s.products
	s.cacheMu.Unlock()
	if cached != nil {
		return cached, nil
	}
	list, err := client.MyProducts()
	if err != nil {
		return nil, err
	}
	products := make([]productDTO, 0, len(list))
	for i, p := range list {
		dto := toProductDTO(p, i)
		dto.Purchased = true
		products = append(products, dto)
	}
	s.cacheMu.Lock()
	s.products = products
	s.cacheMu.Unlock()
	return products, nil
}

// syncRecords makes download records match the files in the download folder.
// Candidates are the purchased courses plus courses that already have records.
func (s *Server) syncRecords(client *geektime.Client, products []productDTO) (syncResult, error) {
	res := syncResult{Unresolved: []string{}}
	cfg, _ := s.snapshotConfig()

	seen := make(map[int]bool)
	var cands []syncCandidate
	for _, p := range products {
		if !p.Openable || seen[p.CourseID] {
			continue
		}
		seen[p.CourseID] = true
		cands = append(cands, syncCandidate{id: p.CourseID, title: p.Title, typeKey: p.TypeKey, direct: p.Direct})
	}
	for _, c := range s.records.courses() {
		if !seen[c.ID] {
			seen[c.ID] = true
			cands = append(cands, syncCandidate{id: c.ID, title: c.Title})
		}
	}

	for _, c := range cands {
		local := localOutputs(filepath.Join(cfg.DownloadFolder, filenamify.Filenamify(c.title)))
		if local == nil {
			// folder is gone: nothing is downloaded, no need to ask the api
			if _, removed := s.records.syncArticles(c.id, c.title, -1, nil); removed > 0 {
				res.Courses++
				res.Removed += removed
			}
			continue
		}

		ids, total, err := s.syncCourseArticles(client, c, local)
		if err != nil {
			if errors.Is(err, geektime.ErrGeekTimeRateLimit) || errors.Is(err, geektime.ErrAuthFailed) {
				res.Aborted = err.Error()
				return res, nil
			}
			logger.Warnf("Failed to sync download records of %s (%d): %v", c.title, c.id, err)
			res.Unresolved = append(res.Unresolved, c.title)
			continue
		}
		added, removed := s.records.syncArticles(c.id, c.title, total, ids)
		res.Courses++
		res.Added += added
		res.Removed += removed
	}
	return res, nil
}

// syncCourseArticles returns ids of the articles of a course found in local
func (s *Server) syncCourseArticles(client *geektime.Client, c syncCandidate, local map[string]bool) ([]int, int, error) {
	key := c.typeKey
	if key == "" {
		key = autoKey
	}
	option, ok := productTypeByKey(key)
	if !ok {
		return nil, 0, errors.New("未知的课程类型")
	}

	if c.direct || !option.NeedSelectArticle {
		if len(local) == 0 {
			return []int{}, 1, nil
		}
		info, err := client.ProductInfo(c.id)
		if err != nil {
			return nil, 0, err
		}
		return []int{info.Data.Info.Article.ID}, 1, nil
	}

	s.cacheMu.Lock()
	_, cached := s.courseCache[typeKeyOf(option)+":"+itoa(c.id)]
	s.cacheMu.Unlock()
	course, _, err := s.loadCourse(client, option, c.id, false)
	if err != nil {
		return nil, 0, err
	}
	if !cached {
		time.Sleep(syncCourseGap)
	}
	ids := []int{}
	for _, a := range course.Articles {
		if local[filenamify.Filenamify(a.Title)] {
			ids = append(ids, a.AID)
		}
	}
	return ids, len(course.Articles), nil
}
