package markdown

import (
	"strings"
	"testing"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
)

func TestRenderComments_Empty(t *testing.T) {
	if got := renderComments(nil); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestRenderComments(t *testing.T) {
	got := renderComments([]geektime.Comment{
		{
			UserName:  "张三_",
			Content:   "# 不是标题\r\n- 不是列表\n1. 也不是\n<script>&amp;",
			Ctime:     time.Date(2024, 1, 2, 10, 0, 0, 0, time.Local),
			LikeCount: 12,
			IsTop:     true,
			Replies:   []geektime.CommentReply{{UserName: "作者", Content: "谢谢\n第二行"}},
		},
		{UserName: "李四", Content: "好文"},
	})

	for _, want := range []string{
		"## 精选留言（2）",
		"**张三\\_** · 2024-01-02 · 👍 12 · 置顶",
		"\\# 不是标题  \n\\- 不是列表  \n1\\. 也不是  \n\\<script\\>&",
		"> **作者**: 谢谢  \n> 第二行",
		"**李四**\n\n好文",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
