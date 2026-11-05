package gui

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/audio"
	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/markdown"
	"github.com/nicoxiang/geektime-downloader/internal/pdf"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
	"github.com/nicoxiang/geektime-downloader/internal/video"
)

// syncCourseGap spaces out course info requests needed by old records
const syncCourseGap = 300 * time.Millisecond

// outputExts are the files that mean an article has been downloaded
var outputExts = []string{markdown.MDExtension, pdf.PDFExtension, audio.MP3Extension, video.TSExtension}

// syncResult summarizes a records verification for the browser
type syncResult struct {
	Checked    int      `json:"checked"`    // article records verified
	Removed    int      `json:"removed"`    // records removed because their files are gone
	Backfilled int      `json:"backfilled"` // old records whose files were looked up once
	Unresolved []string `json:"unresolved"` // courses whose old records could not be verified
	Aborted    string   `json:"aborted,omitempty"`
}

func (s *Server) handleSyncRecords(w http.ResponseWriter, r *http.Request) {
	res, err := s.syncRecords(s.currentClient())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": res, "marks": s.records.marks()})
}

// syncRecords verifies download records against the download folder: records
// whose files are all gone are removed. Only records are checked, local files
// without a record are left alone. Records written before files were stored
// need the article titles once, which is the only time the api is used.
func (s *Server) syncRecords(client *geektime.Client) (syncResult, error) {
	res := syncResult{Unresolved: []string{}}
	cfg, _ := s.snapshotConfig()
	rows, err := s.records.downloadedArticles()
	if err != nil {
		return res, err
	}

	var legacy []storedArticle
	for _, a := range rows {
		if a.Files == nil {
			legacy = append(legacy, a)
			continue
		}
		res.Checked++
		if len(a.Files) == 0 {
			continue // nothing was downloaded for it (no content), nothing to check
		}
		courseDir := filepath.Join(cfg.DownloadFolder, a.Dir)
		existing := existingFiles(courseDir, a.Files)
		switch {
		case len(existing) == 0:
			s.records.removeArticle(a.CourseID, a.ArticleID)
			res.Removed++
		case len(existing) < len(a.Files):
			s.records.setArticleFiles(a.CourseID, a.ArticleID, "", existing)
		}
	}

	if len(legacy) > 0 {
		if err := s.backfillLegacy(client, cfg.DownloadFolder, legacy, &res); err != nil {
			res.Aborted = err.Error()
		}
	}
	return res, nil
}

// existingFiles returns the files (relative to dir) that still exist
func existingFiles(dir string, files []string) []string {
	out := []string{}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// backfillLegacy looks up the files of old records by article title once and
// stores them, so later syncs need no api at all. It returns an error only
// when syncing has to stop (rate limit / login).
func (s *Server) backfillLegacy(client *geektime.Client, root string, legacy []storedArticle, res *syncResult) error {
	byCourse := make(map[int][]storedArticle)
	var order []int
	for _, a := range legacy {
		if _, ok := byCourse[a.CourseID]; !ok {
			order = append(order, a.CourseID)
		}
		byCourse[a.CourseID] = append(byCourse[a.CourseID], a)
	}

	for _, id := range order {
		arts := byCourse[id]
		first := arts[0]
		courseDir := filepath.Join(root, first.Dir)
		if _, err := os.Stat(courseDir); err != nil {
			// the whole course folder is gone, no need to know the titles
			for _, a := range arts {
				s.records.removeArticle(a.CourseID, a.ArticleID)
				res.Checked++
				res.Removed++
			}
			continue
		}
		if client == nil {
			res.Unresolved = append(res.Unresolved, first.CourseTitle)
			continue
		}

		titles, sections, err := s.articleTitles(client, first)
		if err != nil {
			if errors.Is(err, geektime.ErrGeekTimeRateLimit) || errors.Is(err, geektime.ErrAuthFailed) {
				return err
			}
			logger.Warnf("Failed to verify old download records of %s (%d): %v", first.CourseTitle, id, err)
			res.Unresolved = append(res.Unresolved, first.CourseTitle)
			continue
		}
		for _, a := range arts {
			title, ok := titles[a.ArticleID]
			if !ok {
				continue // no longer in the course, leave it as it is
			}
			res.Checked++
			res.Backfilled++
			found := expectedFiles(courseDir, sections[a.ArticleID], title)
			if len(found) == 0 {
				s.records.removeArticle(a.CourseID, a.ArticleID)
				res.Removed++
				continue
			}
			s.records.setArticleFiles(a.CourseID, a.ArticleID, title, found)
		}
	}
	return nil
}

// articleTitles loads article titles and section folders of a course
func (s *Server) articleTitles(client *geektime.Client, a storedArticle) (map[int]string, map[int]string, error) {
	key := a.TypeKey
	if key == "" {
		key = autoKey
	}
	option, ok := productTypeByKey(key)
	if !ok {
		return nil, nil, errors.New("未知的课程类型")
	}
	if !option.NeedSelectArticle {
		return nil, nil, errors.New("单视频产品的旧记录无法校验")
	}
	s.cacheMu.Lock()
	_, cached := s.courseCache[typeKeyOf(option)+":"+itoa(a.CourseID)]
	s.cacheMu.Unlock()
	course, _, err := s.loadCourse(client, option, a.CourseID, false)
	if err != nil {
		return nil, nil, err
	}
	if !cached {
		time.Sleep(syncCourseGap)
	}
	titles := make(map[int]string, len(course.Articles))
	sections := make(map[int]string, len(course.Articles))
	for _, art := range course.Articles {
		titles[art.AID] = art.Title
		sections[art.AID] = art.SectionTitle
	}
	return titles, sections, nil
}

// expectedFiles returns the output files of an article that exist, named the
// way the downloader names them, relative to the course folder
func expectedFiles(courseDir, section, title string) []string {
	name := filenamify.Filenamify(title)
	dirs := []string{""}
	if section != "" {
		dirs = append(dirs, filenamify.Filenamify(section))
	}
	out := []string{}
	for _, d := range dirs {
		for _, ext := range outputExts {
			rel := filepath.Join(d, name+ext)
			if _, err := os.Stat(filepath.Join(courseDir, rel)); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
	}
	return out
}
