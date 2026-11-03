package gui

import (
	"io"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// allowedTags are rendered as is (with filtered attributes); other tags are dropped but their text is kept.
var allowedTags = map[string]bool{
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"p": true, "br": true, "hr": true, "ul": true, "ol": true, "li": true,
	"strong": true, "b": true, "em": true, "i": true, "u": true, "s": true, "del": true, "sup": true, "sub": true,
	"blockquote": true, "code": true, "pre": true, "span": true, "div": true,
	"a": true, "img": true, "figure": true, "figcaption": true,
	"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
}

// droppedWithContent are removed together with everything inside them.
var droppedWithContent = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"noscript": true, "template": true, "svg": true, "math": true, "form": true,
	"textarea": true, "select": true, "button": true, "head": true, "title": true,
}

var voidTags = map[string]bool{"br": true, "hr": true, "img": true}

// sanitizeHTML keeps a safe subset of html for displaying remote rich text.
func sanitizeHTML(in string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(in))
	skip := 0 // depth inside droppedWithContent elements
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() == io.EOF {
				return b.String()
			}
			return b.String()
		}
		tok := z.Token()
		name := strings.ToLower(tok.Data)
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			if droppedWithContent[name] {
				if tt == html.StartTagToken && !voidTags[name] {
					skip++
				}
				continue
			}
			if skip > 0 || !allowedTags[name] {
				continue
			}
			attrs := safeAttrs(name, tok.Attr)
			if name == "img" && attrs == "" {
				continue // image without a safe src
			}
			b.WriteString("<" + name + attrs)
			if voidTags[name] {
				b.WriteString(" />")
			} else {
				b.WriteString(">")
			}
		case html.EndTagToken:
			if droppedWithContent[name] {
				if skip > 0 {
					skip--
				}
				continue
			}
			if skip > 0 || !allowedTags[name] || voidTags[name] {
				continue
			}
			b.WriteString("</" + name + ">")
		case html.TextToken:
			if skip == 0 {
				b.WriteString(html.EscapeString(tok.Data))
			}
		}
	}
}

func safeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

func safeAttrs(tag string, attrs []html.Attribute) string {
	var b strings.Builder
	add := func(k, v string) { b.WriteString(" " + k + `="` + html.EscapeString(v) + `"`) }
	switch tag {
	case "a":
		for _, a := range attrs {
			if strings.ToLower(a.Key) == "href" {
				if u, ok := safeURL(a.Val); ok {
					add("href", u)
					add("target", "_blank")
					add("rel", "noopener noreferrer")
				}
			}
		}
	case "img":
		var src, alt string
		for _, a := range attrs {
			switch strings.ToLower(a.Key) {
			case "src":
				if u, ok := safeURL(a.Val); ok {
					src = u
				}
			case "alt":
				alt = a.Val
			}
		}
		if src == "" {
			return ""
		}
		add("src", src)
		add("alt", alt)
		add("loading", "lazy")
		add("referrerpolicy", "no-referrer")
	case "td", "th":
		for _, a := range attrs {
			k := strings.ToLower(a.Key)
			if k == "colspan" || k == "rowspan" {
				if n, err := strconv.Atoi(a.Val); err == nil && n > 0 && n < 100 {
					add(k, strconv.Itoa(n))
				}
			}
		}
	}
	return b.String()
}
