package geektime

import (
	"github.com/go-resty/resty/v2"

	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
)

const (
	// V3LearnProductPath get purchased products of current user ("我的课程")
	V3LearnProductPath = "/serv/v3/learn/product"

	learnProductPageSize = 100
	learnProductMaxPages = 50
)

// MyProducts returns all products purchased by current user.
func (c *Client) MyProducts() ([]response.LearnProduct, error) {
	var all []response.LearnProduct
	seen := make(map[int]bool)

	// "prev" is a 1-based page number
	for page := 1; page <= learnProductMaxPages; page++ {
		var res response.V3LearnProductResponse
		r := c.newRequest(
			resty.MethodPost,
			DefaultBaseURL,
			V3LearnProductPath,
			nil,
			map[string]interface{}{
				"desc":             true,
				"expire":           1,
				"last_learn":       0,
				"learn_status":     0,
				"prev":             page,
				"size":             learnProductPageSize,
				"sort":             1,
				"type":             "",
				"with_learn_count": 1,
			},
			&res,
		)
		if _, err := do(r); err != nil {
			return nil, err
		}

		added := 0
		for _, p := range res.Data.Products {
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			all = append(all, p)
			added++
		}

		if !res.Data.Page.More || added == 0 {
			break
		}
	}
	return all, nil
}
