package nodes

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Markdown converts Markdown text to HTML.
// Zero dependencies — implements the most common subset:
// headings, bold, italic, code, links, lists, blockquotes, horizontal rules.
//
// For production use with full CommonMark compliance, swap Execute's
// body to use github.com/yuin/goldmark — the interface stays identical.
//
// Example:
//
//	nodes.NewMarkdown()
type Markdown struct{}

func NewMarkdown() *Markdown { return &Markdown{} }

func (m *Markdown) Name() string { return "markdown" }

func (m *Markdown) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Converts Markdown text to HTML. Supports headings, bold, italic, code, links, lists, blockquotes.",
		Params: map[string]any{
			"text": map[string]any{"type": "string", "desc": "Markdown text to convert."},
		},
	}
}

func (m *Markdown) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	text, ok := in["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("markdown: 'text' is required")
	}

	html := convertMarkdown(text)
	return orchkit.Output{
		"html":  html,
		"chars": len(html),
	}, nil
}

func convertMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var out strings.Builder
	inList := false
	inCode := false

	for _, line := range lines {
		// Fenced code blocks
		if strings.HasPrefix(line, "```") {
			if inCode {
				out.WriteString("</code></pre>\n")
				inCode = false
			} else {
				if inList {
					out.WriteString("</ul>\n")
					inList = false
				}
				out.WriteString("<pre><code>")
				inCode = true
			}
			continue
		}
		if inCode {
			out.WriteString(escapeHTML(line) + "\n")
			continue
		}

		// Headings
		if strings.HasPrefix(line, "######") {
			out.WriteString("<h6>" + inline(line[7:]) + "</h6>\n")
			continue
		} else if strings.HasPrefix(line, "#####") {
			out.WriteString("<h5>" + inline(line[6:]) + "</h5>\n")
			continue
		} else if strings.HasPrefix(line, "####") {
			out.WriteString("<h4>" + inline(line[5:]) + "</h4>\n")
			continue
		} else if strings.HasPrefix(line, "###") {
			out.WriteString("<h3>" + inline(line[4:]) + "</h3>\n")
			continue
		} else if strings.HasPrefix(line, "##") {
			out.WriteString("<h2>" + inline(line[3:]) + "</h2>\n")
			continue
		} else if strings.HasPrefix(line, "#") {
			out.WriteString("<h1>" + inline(line[2:]) + "</h1>\n")
			continue
		}

		// Horizontal rule
		if line == "---" || line == "***" || line == "___" {
			out.WriteString("<hr>\n")
			continue
		}

		// Blockquote
		if strings.HasPrefix(line, "> ") {
			out.WriteString("<blockquote>" + inline(line[2:]) + "</blockquote>\n")
			continue
		}

		// Unordered list
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			if !inList {
				out.WriteString("<ul>\n")
				inList = true
			}
			out.WriteString("<li>" + inline(line[2:]) + "</li>\n")
			continue
		}

		// Close list if needed
		if inList && line == "" {
			out.WriteString("</ul>\n")
			inList = false
		}

		// Paragraph
		if line == "" {
			out.WriteString("\n")
		} else {
			out.WriteString("<p>" + inline(line) + "</p>\n")
		}
	}

	if inList {
		out.WriteString("</ul>\n")
	}
	if inCode {
		out.WriteString("</code></pre>\n")
	}

	return out.String()
}

var (
	reBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic = regexp.MustCompile(`\*(.+?)\*`)
	reCode   = regexp.MustCompile("`(.+?)`")
	reLink   = regexp.MustCompile(`\[(.+?)\]\((.+?)\)`)
)

func inline(s string) string {
	s = reLink.ReplaceAllString(s, `<a href="$2">$1</a>`)
	s = reBold.ReplaceAllString(s, `<strong>$1</strong>`)
	s = reItalic.ReplaceAllString(s, `<em>$1</em>`)
	s = reCode.ReplaceAllString(s, `<code>$1</code>`)
	return s
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
