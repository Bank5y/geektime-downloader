package response

import "encoding/json"

// V3LabelListResponse is the response of course direction labels api
type V3LabelListResponse struct {
	Code int `json:"code"`
	Data struct {
		List []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			PID   int    `json:"pid"`
			Count int    `json:"count"`
			Sort  int    `json:"sort"`
		} `json:"list"`
	} `json:"data"`
}

// CateOption is one filter option of the course catalog
type CateOption struct {
	Title string `json:"title"`
	Value int    `json:"value"`
	Count int    `json:"count"`
}

// V4ProductCatesResponse is the response of course catalog filter options api
type V4ProductCatesResponse struct {
	Code int `json:"code"`
	Data struct {
		ProductTypes []CateOption `json:"product_types"`
		ProductForms []CateOption `json:"product_forms"`
		ProductPvips []CateOption `json:"product_pvips"`
	} `json:"data"`
}

// V4ProductListResponse is the response of course catalog list api
type V4ProductListResponse struct {
	Code int `json:"code"`
	Data struct {
		Products []LearnProduct `json:"products"`
		Page     struct {
			More  bool `json:"more"`
			Total int  `json:"total"`
		} `json:"page"`
	} `json:"data"`
}

// V3SearchResponse is the response of search api with category "product"
type V3SearchResponse struct {
	Code int `json:"code"`
	Data struct {
		List []struct {
			ItemType string        `json:"item_type"`
			Product  SearchProduct `json:"product"`
		} `json:"list"`
		Page struct {
			More  bool `json:"more"`
			Count int  `json:"count"`
		} `json:"page"`
	} `json:"data"`
}

// SearchProduct is a product item in search results
type SearchProduct struct {
	ID          int         `json:"id"`
	Type        string      `json:"type"`
	Title       string      `json:"title"`
	Subtitle    string      `json:"subtitle"`
	Cover       string      `json:"cover"`
	TotalLesson json.Number `json:"total_lesson"`
	Unit        string      `json:"uint"`
	AuthorName  string      `json:"author_name"`
	AuthorIntro string      `json:"author_intro"`
	Subscribe   bool        `json:"subscribe"`
}
