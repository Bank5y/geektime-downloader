package gui

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
)

const (
	catalogDefaultPageSize = 24
	catalogMaxPageSize     = 60
)

type labelDTO struct {
	ID       int        `json:"id"`
	Name     string     `json:"name"`
	Count    int        `json:"count"`
	Children []labelDTO `json:"children,omitempty"`
}

type catalogMeta struct {
	Labels []labelDTO            `json:"labels"`
	Types  []response.CateOption `json:"types"`
	Forms  []response.CateOption `json:"forms"`
	Pvips  []response.CateOption `json:"pvips"`
}

type catalogPageDTO struct {
	Items []productDTO `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Size  int          `json:"size"`
	More  bool         `json:"more"`
}

// handleCatalogMeta returns filter options of the course catalog ("全部课程")
func (s *Server) handleCatalogMeta(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	s.cacheMu.Lock()
	cached := s.catalogMeta
	s.cacheMu.Unlock()
	if cached != nil {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	labels, err := client.Labels()
	if err != nil {
		writeAPIError(w, err)
		return
	}
	cates, err := client.ProductCates()
	if err != nil {
		writeAPIError(w, err)
		return
	}
	meta := &catalogMeta{
		Labels: buildLabelTree(labels),
		Types:  cates.Data.ProductTypes,
		Forms:  cates.Data.ProductForms,
		Pvips:  cates.Data.ProductPvips,
	}
	s.cacheMu.Lock()
	s.catalogMeta = meta
	s.cacheMu.Unlock()
	writeJSON(w, http.StatusOK, meta)
}

func buildLabelTree(res response.V3LabelListResponse) []labelDTO {
	list := res.Data.List
	sort.SliceStable(list, func(i, j int) bool { return list[i].Sort > list[j].Sort })
	var roots []labelDTO
	index := make(map[int]int)
	for _, l := range list {
		if l.PID == 0 {
			index[l.ID] = len(roots)
			roots = append(roots, labelDTO{ID: l.ID, Name: l.Name, Count: l.Count})
		}
	}
	for _, l := range list {
		if l.PID == 0 {
			continue
		}
		if i, ok := index[l.PID]; ok {
			roots[i].Children = append(roots[i].Children, labelDTO{ID: l.ID, Name: l.Name, Count: l.Count})
		}
	}
	return roots
}

// handleCatalog returns one page of the course catalog, or search results when q is set
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	qs := r.URL.Query()
	atoi := func(k string, def int) int {
		if v, err := strconv.Atoi(qs.Get(k)); err == nil {
			return v
		}
		return def
	}
	page := atoi("page", 1)
	if page < 1 {
		page = 1
	}
	size := atoi("size", catalogDefaultPageSize)
	if size < 1 || size > catalogMaxPageSize {
		size = catalogDefaultPageSize
	}

	keyword := strings.TrimSpace(qs.Get("q"))
	if keyword != "" {
		res, err := client.SearchProducts(keyword, page, size)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		out := catalogPageDTO{Page: page, Size: size, Total: res.Data.Page.Count, More: res.Data.Page.More}
		seen := make(map[int]bool)
		for i, it := range res.Data.List {
			if it.ItemType != "product" || it.Product.ID == 0 || seen[it.Product.ID] {
				continue
			}
			seen[it.Product.ID] = true
			out.Items = append(out.Items, searchProductDTO(it.Product, (page-1)*size+i))
		}
		out.Items = s.withLocal(out.Items)
		writeJSON(w, http.StatusOK, out)
		return
	}

	res, err := client.ProductList(geektime.CatalogQuery{
		TagID:       atoi("tag", 0),
		ProductType: atoi("type", 0),
		ProductForm: atoi("form", 0),
		Pvip:        atoi("pvip", 0),
		Sort:        atoi("sort", 8),
		Page:        page,
		Size:        size,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	out := catalogPageDTO{Page: page, Size: size, Total: res.Total, More: res.More, Items: make([]productDTO, 0, len(res.Products))}
	for i, p := range res.Products {
		out.Items = append(out.Items, toProductDTO(p, (page-1)*size+i))
	}
	out.Items = s.withLocal(out.Items)
	writeJSON(w, http.StatusOK, out)
}

func searchProductDTO(p response.SearchProduct, order int) productDTO {
	dto := productDTO{
		ID:          p.ID,
		CourseID:    p.ID,
		Title:       p.Title,
		Subtitle:    p.Subtitle,
		Author:      p.AuthorName,
		AuthorIntro: p.AuthorIntro,
		Cover:       p.Cover,
		Code:        p.Type,
		Purchased:   p.Subscribe,
		Order:       order,
		IsVideo:     p.Type == "c3",
	}
	if n, err := p.TotalLesson.Int64(); err == nil {
		dto.ArticleCount = int(n)
	}
	// search results don't carry university class ids
	classifyProduct(&dto, false, 0)
	// search results don't carry the sale type either, the unlock api verifies it before claiming
	dto.Claimable = !dto.Purchased && isOpenCourseCode(dto.Code)
	return dto
}
