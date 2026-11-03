package geektime

import (
	"encoding/json"
	"errors"

	"github.com/go-resty/resty/v2"
)

const (
	// V3SaleMflqGetPath claims a free open course ("免费领取")
	V3SaleMflqGetPath = "/serv/v3/sale/mflq/get"

	// SaleTypeFreeClaim open course that can be claimed for free
	SaleTypeFreeClaim = 6
	// SaleTypeGiveOneGetOne open course unlocked by "赠一得一", can't be claimed directly
	SaleTypeGiveOneGetOne = 7
)

// SaleState is the ownership and sale type of a product
type SaleState struct {
	Title    string
	Type     string
	SaleType int
	Owned    bool
}

// ProductSaleState returns whether the user owns the product and how it is sold
func (c *Client) ProductSaleState(productID int) (SaleState, error) {
	var res struct {
		Code int `json:"code"`
		Data struct {
			Info struct {
				Title string `json:"title"`
				Type  string `json:"type"`
				Price struct {
					SaleType int `json:"sale_type"`
				} `json:"price"`
				Extra struct {
					Sub struct {
						HadDone bool `json:"had_done"`
					} `json:"sub"`
				} `json:"extra"`
			} `json:"info"`
		} `json:"data"`
	}
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V3ProductInfoPath, nil, map[string]interface{}{"id": productID}, &res)
	if _, err := do(r); err != nil {
		return SaleState{}, err
	}
	i := res.Data.Info
	return SaleState{Title: i.Title, Type: i.Type, SaleType: i.Price.SaleType, Owned: i.Extra.Sub.HadDone}, nil
}

// ClaimFreeCourse claims a free open course, same as clicking "免费领取" on the website.
func (c *Client) ClaimFreeCourse(productID int) error {
	// data is an object on success but an empty array on failure, decode it lazily
	var res struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	r := c.newRequest(resty.MethodPost, DefaultBaseURL, V3SaleMflqGetPath, nil, map[string]interface{}{"pid": productID}, &res)
	if _, err := do(r); err != nil {
		var bad ErrGeekTimeAPIBadCode
		if errors.As(err, &bad) {
			if msg := apiErrorMessage(bad.ResponseString); msg != "" {
				return errors.New(msg)
			}
		}
		return err
	}
	var data struct {
		IsSuccess bool `json:"is_success"`
	}
	if err := json.Unmarshal(res.Data, &data); err != nil || !data.IsSuccess {
		return errors.New("领取未成功")
	}
	return nil
}

// apiErrorMessage extracts error.msg from a geektime error response body
func apiErrorMessage(body string) string {
	var e struct {
		Error struct {
			Msg string `json:"msg"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &e) == nil {
		return e.Error.Msg
	}
	return ""
}
