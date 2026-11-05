package geektime

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/go-resty/resty/v2"
)

// ArticleRef is a neighbor article
type ArticleRef struct {
	ID    int    `json:"id"`
	Title string `json:"article_title"`
}

// ReadableArticle is an article for in-app reading
type ReadableArticle struct {
	ID       int
	Title    string
	Content  string // full html, empty when the user can't read it
	Excerpt  string // short html excerpt, available for everyone
	Author   string
	Ctime    int64
	AudioURL string
	VideoID  string
	Prev     *ArticleRef
	Next     *ArticleRef
}

// ArticleForReading fetches an article with its neighbors
func (c *Client) ArticleForReading(articleID int) (ReadableArticle, error) {
	// error responses carry "data": [], decode it only on success
	var res struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	var d struct {
		ID               int    `json:"id"`
		ArticleTitle     string `json:"article_title"`
		ArticleContent   string `json:"article_content"`
		ArticleCshort    string `json:"article_cshort"`
		AuthorName       string `json:"author_name"`
		ArticleCtime     int64  `json:"article_ctime"`
		AudioDownloadURL string `json:"audio_download_url"`
		VideoID          string `json:"video_id"`
		Neighbors        struct {
			// an empty side is [] instead of an object
			Left  json.RawMessage `json:"left"`
			Right json.RawMessage `json:"right"`
		} `json:"neighbors"`
	}
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V1ArticlePath, nil,
		map[string]interface{}{
			"id":                strconv.Itoa(articleID),
			"include_neighbors": true,
			"is_freelyread":     true,
			"reverse":           false,
		}, &res)
	if _, err := do(r); err != nil {
		return ReadableArticle{}, err
	}
	if err := json.Unmarshal(res.Data, &d); err != nil {
		return ReadableArticle{}, err
	}
	return ReadableArticle{
		ID:       d.ID,
		Title:    d.ArticleTitle,
		Content:  d.ArticleContent,
		Excerpt:  d.ArticleCshort,
		Author:   d.AuthorName,
		Ctime:    d.ArticleCtime,
		AudioURL: d.AudioDownloadURL,
		VideoID:  d.VideoID,
		Prev:     neighbor(d.Neighbors.Left),
		Next:     neighbor(d.Neighbors.Right),
	}, nil
}

func neighbor(raw json.RawMessage) *ArticleRef {
	var ref ArticleRef
	if json.Unmarshal(raw, &ref) != nil || ref.ID == 0 {
		return nil
	}
	return &ref
}

// APIErrorMessage returns the message geektime put in an error response, if any
func APIErrorMessage(err error) string {
	var bad ErrGeekTimeAPIBadCode
	if errors.As(err, &bad) {
		return apiErrorMessage(bad.ResponseString)
	}
	return ""
}
