package response

import "encoding/json"

// V3LearnProductResponse is the response of "我的课程" list api.
// Only fields used by the GUI are declared; uncertain numeric fields use json.Number
// so a type change on the server side does not break decoding.
type V3LearnProductResponse struct {
	Code int `json:"code"`
	Data struct {
		Products   []LearnProduct `json:"products"`
		LearnCount struct {
			Total int `json:"total"`
		} `json:"learn_count"`
		Page struct {
			More bool `json:"more"`
		} `json:"page"`
	} `json:"data"`
}

// LearnProduct is a geektime product (course), shared by "我的课程" and the course catalog apis
type LearnProduct struct {
	ID           int    `json:"id"`
	Type         string `json:"type"`
	IsVideo      bool   `json:"is_video"`
	IsAudio      bool   `json:"is_audio"`
	IsFinish     bool   `json:"is_finish"`
	BeginTime    int64  `json:"begin_time"` // launch time (unix seconds), 0 for some video courses
	IsUniversity bool   `json:"is_university"`
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
	Unit         string `json:"unit"`
	Cover        struct {
		Square    string `json:"square"`
		Rectangle string `json:"rectangle"`
	} `json:"cover"`
	Author struct {
		Name  string `json:"name"`
		Intro string `json:"intro"`
	} `json:"author"`
	Article struct {
		Count json.Number `json:"count"`
	} `json:"article"`
	Price struct {
		Sale     json.Number `json:"sale"`
		SaleType int         `json:"sale_type"`
	} `json:"price"`
	University struct {
		ClassID int `json:"class_id"`
	} `json:"university"`
	Extra struct {
		Sub struct {
			HadDone bool        `json:"had_done"`
			Count   json.Number `json:"count"`
		} `json:"sub"`
	} `json:"extra"`
}
