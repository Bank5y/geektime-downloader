package response

// V4CommentListResponse is the response of /serv/v4/comment/list
type V4CommentListResponse struct {
	Code int `json:"code"`
	Data struct {
		List []V4Comment `json:"list"`
		Page struct {
			More  bool `json:"more"`
			Count int  `json:"count"`
		} `json:"page"`
	} `json:"data"`
}

// V4Comment is one article comment
type V4Comment struct {
	ID             int64  `json:"id"`
	UserName       string `json:"user_name"`
	CommentContent string `json:"comment_content"`
	CommentCtime   int64  `json:"comment_ctime"`
	CommentIsTop   bool   `json:"comment_is_top"`
	LikeCount      int    `json:"like_count"`
	Score          int64  `json:"score"`
	IPAddress      string `json:"ip_address"`
	Replies        []struct {
		UserName string `json:"user_name"`
		Content  string `json:"content"`
		Ctime    int64  `json:"ctime"`
	} `json:"replies"`
}
