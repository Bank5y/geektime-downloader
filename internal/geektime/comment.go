package geektime

import (
	"context"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
)

const (
	// V4CommentListPath get article comments, paged by score
	V4CommentListPath = "/serv/v4/comment/list"

	// maxCommentPages guards against endless paging if the API misbehaves
	maxCommentPages = 200
	commentPageGap  = 300 * time.Millisecond
)

// CommentReply is a reply (usually from the author) to a comment.
type CommentReply struct {
	UserName string
	Content  string
}

// Comment is one article comment.
type Comment struct {
	UserName  string
	Content   string
	Ctime     time.Time
	LikeCount int
	IsTop     bool
	Replies   []CommentReply
}

// V4CommentList fetches one page of article comments. prev is the score of
// the last comment on the previous page, 0 for the first page.
func (c *Client) V4CommentList(aid int, prev int64) (response.V4CommentListResponse, error) {
	var res response.V4CommentListResponse
	r := c.newRequest(
		resty.MethodPost,
		DefaultBaseURL,
		V4CommentListPath,
		nil,
		map[string]interface{}{
			"aid":  aid,
			"prev": prev,
			"sort": 0,
		},
		&res,
	)
	if _, err := do(r); err != nil {
		return response.V4CommentListResponse{}, err
	}
	return res, nil
}

// ArticleComments fetches article comments. With allPages false only the
// first page is returned, matching what the article page shows initially.
func (c *Client) ArticleComments(ctx context.Context, aid int, allPages bool) ([]Comment, error) {
	var comments []Comment
	var prev int64
	for page := 0; page < maxCommentPages; page++ {
		if page > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(commentPageGap):
			}
		}
		res, err := c.V4CommentList(aid, prev)
		if err != nil {
			return nil, err
		}
		list := res.Data.List
		for _, v := range list {
			comments = append(comments, toComment(v))
		}
		if !allPages || !res.Data.Page.More || len(list) == 0 {
			break
		}
		next := list[len(list)-1].Score
		if next == prev {
			break
		}
		prev = next
	}
	return comments, nil
}

func toComment(v response.V4Comment) Comment {
	cm := Comment{
		UserName:  v.UserName,
		Content:   v.CommentContent,
		LikeCount: v.LikeCount,
		IsTop:     v.CommentIsTop,
	}
	if v.CommentCtime > 0 {
		cm.Ctime = time.Unix(v.CommentCtime, 0)
	}
	for _, r := range v.Replies {
		cm.Replies = append(cm.Replies, CommentReply{UserName: r.UserName, Content: r.Content})
	}
	return cm
}
