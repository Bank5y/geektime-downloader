package markdown

import (
	"regexp"
	"strconv"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	cellNewlines = regexp.MustCompile(`\s*(\r?\n)+\s*`)
	// a pipe not already escaped
	cellPipe = regexp.MustCompile(`(^|[^\\])\|`)
)

// tablePlugin converts html tables to GFM tables. On top of plugin.Table it
// expands colspan / rowspan (GFM has no merged cells) and keeps every cell on one line with
// pipes escaped, so paragraphs and lists inside cells do not break the table.
// Tables without a header use their first row as the header.
func tablePlugin() md.Plugin {
	return func(c *md.Converter) []md.Rule {
		c.Before(func(selec *goquery.Selection) {
			selec.Find("table").Each(func(_ int, t *goquery.Selection) {
				expandSpans(t)
				promoteFirstRowHeader(t)
			})
		})
		rules := plugin.Table()(c)
		return append(rules, md.Rule{
			Filter: []string{"th", "td"},
			Replacement: func(content string, selec *goquery.Selection, opt *md.Options) *string {
				return md.String(tableCell(content, selec))
			},
		})
	}
}

// tableCell renders one cell of a GFM table row
func tableCell(content string, s *goquery.Selection) string {
	content = strings.TrimSpace(content)
	if s.Find("table").Length() == 0 {
		content = cellNewlines.ReplaceAllString(content, "<br>")
		// twice: the regexp consumes the char before a pipe, "||" needs a second pass
		content = cellPipe.ReplaceAllString(content, `$1\|`)
		content = cellPipe.ReplaceAllString(content, `$1\|`)
	}
	prefix := " "
	if s.Prev().Length() == 0 {
		prefix = "| "
	}
	return prefix + content + " |"
}

// tableRows returns the rows of t, not those of nested tables
func tableRows(t *goquery.Selection) *goquery.Selection {
	return t.Find("tr").FilterFunction(func(_ int, tr *goquery.Selection) bool {
		return tr.Closest("table").IsSelection(t)
	})
}

func spanOf(s *goquery.Selection, attr string) int {
	v, ok := s.Attr(attr)
	if !ok {
		return 1
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return 1
	}
	if n > 100 {
		return 100 // guard against bogus values
	}
	return n
}

// expandSpans replaces merged cells with the merged cell plus empty cells, so
// every row has a cell for every column
func expandSpans(t *goquery.Selection) {
	pending := map[int]int{} // column -> rows still covered by a rowspan above
	tableRows(t).Each(func(_ int, tr *goquery.Selection) {
		col := 0
		fill := func(at *goquery.Selection, before bool, tag string) {
			for pending[col] > 0 {
				pending[col]--
				empty := "<" + tag + "></" + tag + ">"
				if at == nil {
					tr.AppendHtml(empty)
				} else if before {
					at.BeforeHtml(empty)
				} else {
					at.AfterHtml(empty)
				}
				col++
			}
		}
		tr.ChildrenFiltered("td, th").Each(func(_ int, cell *goquery.Selection) {
			tag := goquery.NodeName(cell)
			fill(cell, true, tag)
			cs, rs := spanOf(cell, "colspan"), spanOf(cell, "rowspan")
			cell.RemoveAttr("colspan")
			cell.RemoveAttr("rowspan")
			for i := 0; i < cs; i++ {
				if rs > 1 {
					pending[col+i] = rs - 1
				}
			}
			for i := 1; i < cs; i++ {
				cell.AfterHtml("<" + tag + "></" + tag + ">")
			}
			col += cs
		})
		// rowspans from above that reach past the last cell of this row
		for c := range pending {
			if c >= col && pending[c] > 0 {
				for ; col < c; col++ {
					tr.AppendHtml("<td></td>")
				}
				fill(nil, false, "td")
			}
		}
	})
}

// promoteFirstRowHeader turns the first row into the header when the table
// has none. GFM tables need a header row, and the geektime editor writes every
// cell as td with the column titles in the first row, so an empty header plus
// the titles as a normal row would not match the article.
func promoteFirstRowHeader(t *goquery.Selection) {
	if t.Find("thead").Length() > 0 || t.Find("th").Length() > 0 {
		return
	}
	first := tableRows(t).First()
	cells := first.ChildrenFiltered("td")
	if cells.Length() == 0 {
		return
	}
	cells.Each(func(_ int, cell *goquery.Selection) {
		// a header cell is bold already
		cell.Find("strong, b").Each(func(_ int, b *goquery.Selection) {
			b.ReplaceWithSelection(b.Contents())
		})
		for _, n := range cell.Nodes {
			if n.Type == html.ElementNode {
				n.Data = "th"
				n.DataAtom = atom.Th
			}
		}
	})
}
