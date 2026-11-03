package gui

import (
	"net/http"
	"strconv"
	"strings"
)

type introDTO struct {
	Subtitle    string `json:"subtitle"`
	IntroHTML   string `json:"introHtml"`
	AuthorName  string `json:"authorName"`
	AuthorIntro string `json:"authorIntro"`
	AuthorHTML  string `json:"authorHtml"`
	Cover       string `json:"cover"`
}

// handleCourseIntro returns the sanitized introduction of a course
func (s *Server) handleCourseIntro(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	option, ok := productTypeByKey(r.URL.Query().Get("type"))
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if !ok || err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "参数错误")
		return
	}
	if option.IsEnterpriseMode {
		writeJSON(w, http.StatusOK, introDTO{})
		return
	}

	productID := id
	if option.IsUniversity() {
		c, status, err := s.loadCourse(client, option, id, false)
		if err != nil {
			if status == 0 {
				status = http.StatusBadGateway
			}
			writeError(w, status, err.Error())
			return
		}
		productID = c.SKU
		if productID == 0 {
			writeJSON(w, http.StatusOK, introDTO{})
			return
		}
	}

	key := "intro:" + strconv.Itoa(productID)
	s.cacheMu.Lock()
	cached, hit := s.introCache[key]
	s.cacheMu.Unlock()
	if hit {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	info, err := client.ProductIntro(productID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	dto := introDTO{
		Subtitle:    info.Subtitle,
		AuthorName:  info.AuthorName,
		AuthorIntro: info.AuthorIntro,
		AuthorHTML:  sanitizeHTML(info.AuthorBriefHTML),
		Cover:       info.Cover,
	}
	// the column intro page has the richest content, fall back to product intro
	full := ""
	if !option.IsUniversity() {
		if h, err := client.ColumnIntroHTML(productID); err == nil {
			full = h
		}
	}
	if visibleTextLen(full) < 20 {
		full = info.IntroHTML
	} else {
		// full intro already contains the instructor section
		dto.AuthorHTML = ""
	}
	dto.IntroHTML = sanitizeHTML(full)
	if visibleTextLen(dto.IntroHTML) == 0 && !strings.Contains(dto.IntroHTML, "<img") {
		dto.IntroHTML = ""
	}

	s.cacheMu.Lock()
	s.introCache[key] = dto
	s.cacheMu.Unlock()
	writeJSON(w, http.StatusOK, dto)
}

func visibleTextLen(h string) int {
	n := 0
	in := false
	for _, r := range h {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in && r != ' ' && r != '\n' && r != '\t':
			n++
		}
	}
	return n
}
