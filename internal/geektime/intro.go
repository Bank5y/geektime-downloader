package geektime

import (
	"strconv"

	"github.com/go-resty/resty/v2"
)

const (
	// V1ColumnIntroPath full rich text introduction of a column
	V1ColumnIntroPath = "/serv/v1/column/intro"
)

// CourseIntro is the introduction of a course
type CourseIntro struct {
	Subtitle        string
	IntroHTML       string
	AuthorName      string
	AuthorIntro     string
	AuthorBriefHTML string
	Cover           string
}

// ColumnIntroHTML returns the full introduction html shown on the course intro page, may be empty
func (c *Client) ColumnIntroHTML(productID int) (string, error) {
	var res struct {
		Code int `json:"code"`
		Data struct {
			ColumnIntro string `json:"column_intro"`
		} `json:"data"`
	}
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V1ColumnIntroPath, nil,
		map[string]interface{}{"cid": strconv.Itoa(productID), "with_groupbuy": false}, &res)
	if _, err := do(r); err != nil {
		return "", err
	}
	return res.Data.ColumnIntro, nil
}

// ProductIntro returns the basic introduction from product info
func (c *Client) ProductIntro(productID int) (CourseIntro, error) {
	var res struct {
		Code int `json:"code"`
		Data struct {
			Info struct {
				Subtitle  string `json:"subtitle"`
				IntroHTML string `json:"intro_html"`
				Cover     struct {
					Square string `json:"square"`
				} `json:"cover"`
				Author struct {
					Name      string `json:"name"`
					Intro     string `json:"intro"`
					BriefHTML string `json:"brief_html"`
				} `json:"author"`
			} `json:"info"`
		} `json:"data"`
	}
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V3ProductInfoPath, nil, map[string]interface{}{"id": productID}, &res)
	if _, err := do(r); err != nil {
		return CourseIntro{}, err
	}
	i := res.Data.Info
	return CourseIntro{
		Subtitle:        i.Subtitle,
		IntroHTML:       i.IntroHTML,
		AuthorName:      i.Author.Name,
		AuthorIntro:     i.Author.Intro,
		AuthorBriefHTML: i.Author.BriefHTML,
		Cover:           i.Cover.Square,
	}, nil
}
