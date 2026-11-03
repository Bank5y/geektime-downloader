package gui

import (
	"net/http"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
)

func isOpenCourseCode(code string) bool {
	return code == "p29" || code == "p30" || code == "p35"
}

// unlockResult is the outcome of unlocking one course
type unlockResult struct {
	ID      int    `json:"id"`
	Status  string `json:"status"` // claimed | owned | unsupported | error
	Message string `json:"message"`
}

// handleUnlock claims a free open course ("免费领取").
// The product state is checked first so only courses the website itself offers for free are claimed.
func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	var req struct {
		ProductID int `json:"productId"`
	}
	if err := readJSON(w, r, &req); err != nil || req.ProductID <= 0 {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	res := unlockResult{ID: req.ProductID}
	st, err := client.ProductSaleState(req.ProductID)
	switch {
	case err != nil:
		writeAPIError(w, err)
		return
	case st.Owned:
		res.Status, res.Message = "owned", "已拥有该课程"
	case st.SaleType == geektime.SaleTypeGiveOneGetOne:
		res.Status, res.Message = "unsupported", "该课程需「赠一得一」解锁, 无法直接领取"
	case st.SaleType != geektime.SaleTypeFreeClaim:
		res.Status, res.Message = "unsupported", "该课程不是免费领取的公开课"
	default:
		if err := client.ClaimFreeCourse(req.ProductID); err != nil {
			if code := statusOf(err); code == http.StatusUnauthorized || code == http.StatusTooManyRequests {
				writeAPIError(w, err)
				return
			}
			res.Status, res.Message = "error", err.Error()
		} else {
			res.Status, res.Message = "claimed", "领取成功"
		}
	}
	if res.Status == "claimed" {
		// "我的课程" changed
		s.cacheMu.Lock()
		s.products = nil
		s.cacheMu.Unlock()
	}
	writeJSON(w, http.StatusOK, res)
}
