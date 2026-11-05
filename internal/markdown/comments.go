package markdown

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
)

var (
	// characters that can start inline markdown / html constructs
	mdInlineEscaper = strings.NewReplacer(
		`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`,
		`<`, `\<`, `>`, `\>`, `#`, `\#`, `|`, `\|`, `~`, `\~`,
	)
	// line prefixes that turn a line into a list item
	mdListPrefix = regexp.MustCompile(`^(\s*)([-+]|\d+\.)(\s)`)
)

// renderComments renders comments as a markdown section; empty if none.
func renderComments(comments []geektime.Comment) string {
	if len(comments) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n## 精选留言（%d）\n", len(comments))
	for _, c := range comments {
		b.WriteString("\n")
		meta := []string{"**" + escapeText(c.UserName) + "**"}
		if !c.Ctime.IsZero() {
			meta = append(meta, c.Ctime.Format("2006-01-02"))
		}
		if c.LikeCount > 0 {
			meta = append(meta, fmt.Sprintf("👍 %d", c.LikeCount))
		}
		if c.IsTop {
			meta = append(meta, "置顶")
		}
		b.WriteString(strings.Join(meta, " · "))
		b.WriteString("\n\n")
		b.WriteString(escapeBlock(c.Content, ""))
		b.WriteString("\n")
		for _, r := range c.Replies {
			name := escapeText(r.UserName)
			if name == "" {
				name = "作者回复"
			}
			b.WriteString("\n> **" + name + "**: ")
			b.WriteString(strings.TrimPrefix(escapeBlock(r.Content, "> "), "> "))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// escapeText escapes a single-line plain text value.
func escapeText(s string) string {
	s = strings.Join(strings.Fields(html.UnescapeString(s)), " ")
	return mdInlineEscaper.Replace(s)
}

// escapeBlock escapes multi-line plain text, keeping line breaks as hard
// breaks and prefixing every line with prefix.
func escapeBlock(s, prefix string) string {
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			continue
		}
		line = mdInlineEscaper.Replace(line)
		// "- x" -> "\- x", "1. x" -> "1\. x"
		line = mdListPrefix.ReplaceAllStringFunc(line, func(m string) string {
			if i := strings.LastIndex(m, "."); i >= 0 {
				return m[:i] + `\` + m[i:]
			}
			i := len(m) - len(strings.TrimLeft(m, " \t"))
			return m[:i] + `\` + m[i:]
		})
		lines = append(lines, prefix+line)
	}
	return strings.Join(lines, "  \n")
}
