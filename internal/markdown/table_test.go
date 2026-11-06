package markdown

import (
	"strings"
	"testing"
)

func convert(t *testing.T, h string) string {
	t.Helper()
	out, err := getDefaultConverter().ConvertString(h)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func wantLines(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w+"\n") && !strings.HasSuffix(got, w) {
			t.Errorf("missing line %q in:\n%s", w, got)
		}
	}
}

// structure of the editor output: th header, <p> and lists inside cells
func TestTableWithParagraphsAndLists(t *testing.T) {
	got := convert(t, `<h3>常见的用户输入错误和例子</h3><table><thead><tr><th>大类</th><th>说明</th><th>典型例子</th></tr></thead>
<tbody><tr><td><p>指代问题</p></td><td><p>用户用“这个、那个”等表达代替具体对象</p></td>
<td><ul><li><p>第二个适合生产吗？</p></li><li><p>刚才那个项目怎么包装？</p></li></ul></td></tr>
<tr><td><p>词义问题</p></td><td><p>非标准术语</p><p>同义词</p></td><td><ul><li>RAG / 检索增强 | 知识库</li></ul></td></tr></tbody></table><p>后文</p>`)
	wantLines(t, got,
		"| 大类 | 说明 | 典型例子 |",
		"| --- | --- | --- |",
		"| 指代问题 | 用户用“这个、那个”等表达代替具体对象 | - 第二个适合生产吗？<br>- 刚才那个项目怎么包装？ |",
		`| 词义问题 | 非标准术语<br>同义词 | - RAG / 检索增强 \| 知识库 |`,
	)
	if !strings.Contains(got, "|\n\n后文") {
		t.Errorf("table should be followed by a blank line:\n%s", got)
	}
}

// header written as a bold first row instead of th
func TestTableBoldFirstRowBecomesHeader(t *testing.T) {
	got := convert(t, `<table><tbody><tr><td><strong>能做什么</strong></td><td><strong>不能做什么</strong></td></tr>
<tr><td>指代消解</td><td>不直接回答用户问题</td></tr></tbody></table>`)
	wantLines(t, got,
		"| 能做什么 | 不能做什么 |",
		"| --- | --- |",
		"| 指代消解 | 不直接回答用户问题 |",
	)
	if strings.Contains(got, "**") {
		t.Errorf("header cells should not keep the bold markers:\n%s", got)
	}
}

// the geektime editor writes every cell as td, column titles in the first row
func TestTableFirstRowBecomesHeader(t *testing.T) {
	td := `<td style="border: 1px solid #ddd"><p style="margin: 0">%s</p></td>`
	row := func(cells ...string) string {
		out := "<tr>"
		for _, c := range cells {
			out += strings.Replace(td, "%s", c, 1)
		}
		return out + "</tr>"
	}
	got := convert(t, `<table style="border-collapse: collapse"><tbody>`+row("大类", "说明", "典型例子")+
		`<tr><td><p>指代问题</p></td><td><p>代替具体对象</p></td><td><ul><li><p>第二个适合生产吗？</p></li></ul><ul><li><p>刚才那个项目怎么包装？</p></li></ul></td></tr>`+
		`</tbody></table>`)
	wantLines(t, got,
		"| 大类 | 说明 | 典型例子 |",
		"| --- | --- | --- |",
		"| 指代问题 | 代替具体对象 | - 第二个适合生产吗？<br>- 刚才那个项目怎么包装？ |",
	)
	if strings.Contains(got, "|     |") {
		t.Errorf("no empty header expected:\n%s", got)
	}
}

func TestTableMergedCells(t *testing.T) {
	got := convert(t, `<table><thead><tr><th colspan="2">方案</th><th>说明</th></tr></thead><tbody>
<tr><td rowspan="2">A</td><td>a1</td><td>x</td></tr>
<tr><td>a2</td><td>y</td></tr>
<tr><td>B</td><td colspan="2">全部</td></tr></tbody></table>`)
	wantLines(t, got,
		"| 方案 |  | 说明 |",
		"| --- | --- | --- |",
		"| A | a1 | x |",
		"|  | a2 | y |",
		"| B | 全部 |  |",
	)
}
