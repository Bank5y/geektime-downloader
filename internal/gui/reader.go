package gui

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/ui"
)

type articleRefDTO struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type readerArticleDTO struct {
	ID     int            `json:"id"`
	Title  string         `json:"title"`
	HTML   string         `json:"html"`
	Trial  bool           `json:"trial"` // only an excerpt is readable
	Author string         `json:"author,omitempty"`
	Ctime  int64          `json:"ctime,omitempty"`
	Audio  string         `json:"audio,omitempty"`
	Video  bool           `json:"video"`
	Prev   *articleRefDTO `json:"prev"`
	Next   *articleRefDTO `json:"next"`
}

func refDTO(r *geektime.ArticleRef) *articleRefDTO {
	if r == nil {
		return nil
	}
	return &articleRefDTO{r.ID, r.Title}
}

// handleArticle returns one article for the reader page:
// GET /api/article?type=n0&course=100017301&id=39922
func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	q := r.URL.Query()
	option, ok := productTypeByKey(q.Get("type"))
	courseID, err1 := strconv.Atoi(q.Get("course"))
	id, err2 := strconv.Atoi(q.Get("id"))
	if !ok || err1 != nil || err2 != nil || id <= 0 || courseID <= 0 || option.IsEnterpriseMode || !option.NeedSelectArticle {
		writeError(w, http.StatusBadRequest, "参数错误")
		return
	}

	var out readerArticleDTO
	if option.IsUniversity() {
		c, status, err := s.loadCourse(client, option, courseID, false)
		if err != nil {
			if status == 0 {
				status = http.StatusBadGateway
			}
			writeError(w, status, err.Error())
			return
		}
		idx := -1
		for i, a := range c.Articles {
			if a.AID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			writeError(w, http.StatusNotFound, "文章不在该课程中")
			return
		}
		detail, err := client.UniversityClassArticleDetail(c.ID, id)
		if err != nil {
			writeArticleError(w, err)
			return
		}
		out = readerArticleDTO{ID: id, Title: c.Articles[idx].Title, HTML: sanitizeHTML(detail.Data.ArticleContent), Video: detail.Data.VideoID != ""}
		if detail.Data.ArticleTitle != "" {
			out.Title = detail.Data.ArticleTitle
		}
		if idx > 0 {
			out.Prev = &articleRefDTO{c.Articles[idx-1].AID, c.Articles[idx-1].Title}
		}
		if idx+1 < len(c.Articles) {
			out.Next = &articleRefDTO{c.Articles[idx+1].AID, c.Articles[idx+1].Title}
		}
	} else {
		a, err := client.ArticleForReading(id)
		if err != nil {
			writeArticleError(w, err)
			return
		}
		out = readerArticleDTO{
			ID: id, Title: a.Title, Author: a.Author, Ctime: a.Ctime, Audio: httpURL(a.AudioURL),
			Video: a.VideoID != "", Prev: refDTO(a.Prev), Next: refDTO(a.Next),
		}
		// the api's neighbors don't always follow the table of contents order
		if prev, next, ok := s.outlineNeighbors(client, option, q.Get("type"), courseID, id); ok {
			out.Prev, out.Next = prev, next
		}
		out.HTML = sanitizeHTML(a.Content)
		if visibleTextLen(out.HTML) == 0 && !strings.Contains(out.HTML, "<img") {
			out.HTML, out.Trial = sanitizeHTML(a.Excerpt), true
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// httpURL keeps only absolute http(s) urls
func httpURL(raw string) string {
	u, _ := safeURL(raw)
	return u
}

func writeArticleError(w http.ResponseWriter, err error) {
	if errors.Is(err, geektime.ErrAuthFailed) || errors.Is(err, geektime.ErrGeekTimeRateLimit) {
		writeAPIError(w, err)
		return
	}
	if msg := geektime.APIErrorMessage(err); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	writeAPIError(w, err)
}

// outlineNeighbors finds the articles before and after id in the course outline
func (s *Server) outlineNeighbors(client *geektime.Client, option ui.ProductTypeSelectOption, typeKey string, courseID, id int) (prev, next *articleRefDTO, ok bool) {
	p, _, err := s.coursePreview(client, option, typeKey, courseID)
	if err != nil {
		return nil, nil, false
	}
	var flat []previewArticle
	for _, c := range p.Chapters {
		flat = append(flat, c.Articles...)
	}
	for i, a := range flat {
		if a.ID != id {
			continue
		}
		if i > 0 {
			prev = &articleRefDTO{flat[i-1].ID, flat[i-1].Title}
		}
		if i+1 < len(flat) {
			next = &articleRefDTO{flat[i+1].ID, flat[i+1].Title}
		}
		return prev, next, true
	}
	return nil, nil, false
}
