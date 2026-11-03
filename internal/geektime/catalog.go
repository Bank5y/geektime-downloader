package geektime

import (
	"regexp"

	"github.com/go-resty/resty/v2"

	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
)

const (
	// V3LabelListPath course direction labels (人工智能, 后端/架构 ...)
	V3LabelListPath = "/serv/v3/label/list"
	// V4ProductCatesPath course catalog filter options
	V4ProductCatesPath = "/serv/v4/pvip/product_cates"
	// V4ProductListPath course catalog list ("全部课程")
	V4ProductListPath = "/serv/v4/pvip/product_list"
	// V3SearchPath search api
	V3SearchPath = "/serv/v3/search"
)

// CatalogQuery is the filter of course catalog
type CatalogQuery struct {
	TagID       int // label id, 0 means all
	ProductType int // 0 all, 1 体系课, 4 公开课, 5 线下大会, 6 社区课
	ProductForm int // 0 all, 1 图文+音频, 2 视频
	Pvip        int // 0 all, 1 会员课程, 2 非会员课程
	Sort        int // 8 默认, 4 最热, 1 最新
	Page        int // 1-based
	Size        int
}

// CatalogPage is one page of products
type CatalogPage struct {
	Products []response.LearnProduct
	Total    int
	More     bool
}

// Labels returns course direction labels, top level labels have PID 0
func (c *Client) Labels() (response.V3LabelListResponse, error) {
	var res response.V3LabelListResponse
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V3LabelListPath, nil, map[string]interface{}{}, &res)
	_, err := do(r)
	return res, err
}

// ProductCates returns course catalog filter options with counts
func (c *Client) ProductCates() (response.V4ProductCatesResponse, error) {
	var res response.V4ProductCatesResponse
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V4ProductCatesPath, nil, map[string]interface{}{}, &res)
	_, err := do(r)
	return res, err
}

// ProductList returns one page of course catalog
func (c *Client) ProductList(q CatalogQuery) (CatalogPage, error) {
	tagIDs := []int{}
	if q.TagID > 0 {
		tagIDs = append(tagIDs, q.TagID)
	}
	if q.Sort == 0 {
		q.Sort = 8
	}
	var res response.V4ProductListResponse
	r := c.newRequest(
		resty.MethodPost,
		DefaultBaseURL,
		V4ProductListPath,
		nil,
		map[string]interface{}{
			"tag_ids":       tagIDs,
			"product_type":  q.ProductType,
			"product_form":  q.ProductForm,
			"pvip":          q.Pvip,
			"prev":          q.Page, // 1-based page number
			"size":          q.Size,
			"sort":          q.Sort,
			"with_articles": false,
		},
		&res,
	)
	if _, err := do(r); err != nil {
		return CatalogPage{}, err
	}
	return CatalogPage{Products: res.Data.Products, Total: res.Data.Page.Total, More: res.Data.Page.More}, nil
}

var emTagRegexp = regexp.MustCompile(`</?em>`)

// SearchProducts searches courses by keyword, page is 1-based
func (c *Client) SearchProducts(keyword string, page, size int) (response.V3SearchResponse, error) {
	var res response.V3SearchResponse
	r := c.newRequest(
		resty.MethodPost,
		DefaultBaseURL,
		V3SearchPath,
		nil,
		map[string]interface{}{
			"keyword":  keyword,
			"category": "product",
			"size":     size,
			"prev":     page,
			"platform": "pc",
		},
		&res,
	)
	if _, err := do(r); err != nil {
		return res, err
	}
	// strip highlight tags
	for i := range res.Data.List {
		p := &res.Data.List[i].Product
		p.Title = emTagRegexp.ReplaceAllString(p.Title, "")
		p.Subtitle = emTagRegexp.ReplaceAllString(p.Subtitle, "")
	}
	return res, nil
}
