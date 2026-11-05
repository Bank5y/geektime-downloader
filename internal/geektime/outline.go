package geektime

import (
	"strconv"

	"github.com/go-resty/resty/v2"
)

// OutlineArticle is an article entry of a column outline (no content)
type OutlineArticle struct {
	ID        int    `json:"id"`
	Title     string `json:"article_title"`
	ChapterID string `json:"chapter_id"`
	Ctime     int64  `json:"article_ctime"`
}

type outlineResponse struct {
	Code int `json:"code"`
	Data struct {
		List []OutlineArticle `json:"list"`
		Page struct {
			Count int  `json:"count"`
			More  bool `json:"more"`
		} `json:"page"`
	} `json:"data"`
}

// ArticleOutline returns up to size articles of a column in publish order (newest
// first when newest is set) and the total article count. It works for columns the
// user hasn't bought, titles and publish times are public.
func (c *Client) ArticleOutline(cid, size int, newest bool) ([]OutlineArticle, int, error) {
	order := "earliest"
	if newest {
		order = "newest"
	}
	var res outlineResponse
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V1ColumnArticlesPath, nil,
		map[string]interface{}{
			"cid":    strconv.Itoa(cid),
			"order":  order,
			"prev":   0,
			"sample": false,
			"size":   size,
		}, &res)
	if _, err := do(r); err != nil {
		return nil, 0, err
	}
	return res.Data.List, res.Data.Page.Count, nil
}
