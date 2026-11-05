package geektime

import (
	"encoding/json"
	"strconv"

	"github.com/go-resty/resty/v2"
)

// V1ChaptersPath lists the chapters of a column
const V1ChaptersPath = "/serv/v1/chapters"

// Chapter is a chapter of a column, ID matches OutlineArticle.ChapterID
type Chapter struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ArticleCount int    `json:"article_count"`
}

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

// Chapters returns the chapters of a column; columns without chapters return none
func (c *Client) Chapters(cid int) ([]Chapter, error) {
	var res struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V1ChaptersPath, nil,
		map[string]interface{}{"cid": strconv.Itoa(cid)}, &res)
	if _, err := do(r); err != nil {
		return nil, err
	}
	var list []Chapter
	// "data" is [] or a list; anything else means no chapters
	_ = json.Unmarshal(res.Data, &list)
	return list, nil
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
