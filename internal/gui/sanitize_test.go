package gui

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	in := `<h2>你将获得</h2><ul><li>一 &amp; 二</li></ul>
<p onclick="alert(1)" style="color:red">正文<script>alert('x')</script><b>粗</b></p>
<img src="https://static001.geekbang.org/a.jpg" onerror="alert(1)" alt="图">
<img src="javascript:alert(1)"><img src="//static001.geekbang.org/b.png">
<a href="javascript:alert(1)">bad</a><a href="https://time.geekbang.org/x">good</a>
<iframe src="https://evil"><p>inside</p></iframe><style>body{}</style>
<svg><script>alert(2)</script></svg><custom>kept text</custom><td colspan="2;x">c</td>`
	out := sanitizeHTML(in)
	for _, bad := range []string{"script", "onclick", "onerror", "style", "javascript:", "iframe", "inside", "alert", "<custom", "colspan"} {
		if strings.Contains(out, bad) {
			t.Fatalf("output contains %q:\n%s", bad, out)
		}
	}
	for _, good := range []string{
		"<h2>你将获得</h2>", "一 &amp; 二", "<b>粗</b>", "正文",
		`<img src="https://static001.geekbang.org/a.jpg" alt="图" loading="lazy" referrerpolicy="no-referrer" />`,
		`src="https://static001.geekbang.org/b.png"`,
		`<a href="https://time.geekbang.org/x" target="_blank" rel="noopener noreferrer">good</a>`,
		"<a>bad</a>", "kept text",
	} {
		if !strings.Contains(out, good) {
			t.Fatalf("output missing %q:\n%s", good, out)
		}
	}
}
