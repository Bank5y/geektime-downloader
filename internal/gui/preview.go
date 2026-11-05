package gui

import (
	"net/http"
	"strconv"
	"time"
)

const maxOutline = 500 // articles fetched for a preview

type previewChapter struct {
	Title    string   `json:"title"`
	Count    int      `json:"count"`
	Articles []string `json:"articles"`
}

type previewDTO struct {
	Total    int              `json:"total"`
	Chapters []previewChapter `json:"chapters"`
	courseDates
}

type cachedPreview struct {
	dto previewDTO
	at  time.Time
}

func (c *outlineCache) getPreview(key string) (previewDTO, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.previews[key]
	if !ok || time.Since(p.at) > outlineCacheTTL {
		return previewDTO{}, false
	}
	return p.dto, true
}

func (c *outlineCache) putPreview(key string, p previewDTO) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.previews == nil {
		c.previews = make(map[string]cachedPreview)
	}
	c.previews[key] = cachedPreview{p, time.Now()}
}

// handleCoursePreview returns the chapter outline of a course for the hover preview
func (s *Server) handleCoursePreview(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	typeKey := r.URL.Query().Get("type")
	option, ok := productTypeByKey(typeKey)
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if !ok || err != nil || id <= 0 || option.IsEnterpriseMode {
		writeError(w, http.StatusBadRequest, "参数错误")
		return
	}
	key := typeKey + ":" + strconv.Itoa(id)
	if p, ok := s.outline.getPreview(key); ok {
		writeJSON(w, http.StatusOK, p)
		return
	}

	var p previewDTO
	switch {
	case option.IsUniversity():
		c, status, err := s.loadCourse(client, option, id, false)
		if err != nil {
			if status == 0 {
				status = http.StatusBadGateway
			}
			writeError(w, status, err.Error())
			return
		}
		titles := make([]string, len(c.Articles))
		chapters := make([]string, len(c.Articles))
		for i, a := range c.Articles {
			titles[i], chapters[i] = a.Title, a.Chapter
		}
		p = groupOutline(titles, chapters, nil)
	case !option.NeedSelectArticle:
		// 每日一课 / 大厂案例: a single video, nothing to outline
	default:
		chapters, err := client.Chapters(id)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		arts, total, err := client.ArticleOutline(id, maxOutline, false)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		names := make(map[string]string, len(chapters))
		var order []string
		for _, ch := range chapters {
			names[ch.ID] = ch.Title
			order = append(order, ch.Title)
		}
		titles := make([]string, len(arts))
		groups := make([]string, len(arts))
		for i, a := range arts {
			titles[i] = a.Title
			if n, ok := names[a.ChapterID]; ok {
				groups[i] = n
			} else if len(chapters) > 0 {
				groups[i] = "其他"
			}
		}
		p = groupOutline(titles, groups, order)
		if total > p.Total {
			p.Total = total
		}
		if len(arts) > 0 {
			p.First = arts[0].Ctime
			// the newest article is only known when the whole list was fetched
			if len(arts) >= total {
				for _, a := range arts {
					if a.Ctime > p.Last {
						p.Last = a.Ctime
					}
				}
				p.Count = total
				s.outline.putDates(id, p.courseDates, true)
			} else if d, ok := s.outline.getDates(id, false); ok {
				p.Last, p.Count = d.Last, d.Count
			}
		}
	}
	if p.Chapters == nil {
		p.Chapters = []previewChapter{}
	}
	s.outline.putPreview(key, p)
	writeJSON(w, http.StatusOK, p)
}

// groupOutline groups article titles by chapter, keeping chapter order (order first,
// then first appearance); articles with an empty chapter form one untitled group
func groupOutline(titles, chapters []string, order []string) previewDTO {
	idx := make(map[string]int)
	var out []previewChapter
	add := func(name string) int {
		if i, ok := idx[name]; ok {
			return i
		}
		idx[name] = len(out)
		out = append(out, previewChapter{Title: name})
		return idx[name]
	}
	for _, name := range order {
		add(name)
	}
	for i, t := range titles {
		c := &out[add(chapters[i])]
		c.Count++
		c.Articles = append(c.Articles, t)
	}
	// drop declared chapters without articles
	kept := out[:0]
	for _, c := range out {
		if c.Count > 0 {
			kept = append(kept, c)
		}
	}
	return previewDTO{Total: len(titles), Chapters: kept}
}
