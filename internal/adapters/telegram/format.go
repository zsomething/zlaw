package telegram

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	reFencedCode  = regexp.MustCompile("(?s)```(\\w*)\\n?(.*?)```")
	reInlineCode  = regexp.MustCompile("`([^`\n]+)`")
	reHeader      = regexp.MustCompile(`(?m)^#{1,6} +(.+)$`)
	reBold        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalicStar  = regexp.MustCompile(`\*([^*\n]+)\*`)
	reItalicUnder = regexp.MustCompile(`_([^_\n]+)_`)
	reLink        = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

// mdToHTML converts a subset of Markdown to Telegram-compatible HTML.
// Supported: fenced code blocks, inline code, **bold**, *italic*, _italic_,
// # headings (rendered as bold), and [links](url).
//
// Strategy: extract code spans first (to avoid formatting inside them),
// HTML-escape the remaining text, apply Markdown rules, then restore code.
func mdToHTML(md string) string {
	// placeholders[i] holds the final HTML for extracted code spans/blocks.
	var placeholders []string

	// reserve replaces match with a unique null-byte-delimited token.
	reserve := func(htmlBlock string) string {
		token := fmt.Sprintf("\x00%d\x00", len(placeholders))
		placeholders = append(placeholders, htmlBlock)
		return token
	}

	// 1. Extract fenced code blocks.
	result := reFencedCode.ReplaceAllStringFunc(md, func(m string) string {
		sub := reFencedCode.FindStringSubmatch(m)
		lang, code := strings.TrimSpace(sub[1]), html.EscapeString(sub[2])
		if lang != "" {
			return reserve(fmt.Sprintf(`<pre><code class="language-%s">%s</code></pre>`, lang, code))
		}
		return reserve("<pre>" + code + "</pre>")
	})

	// 2. Extract inline code.
	result = reInlineCode.ReplaceAllStringFunc(result, func(m string) string {
		sub := reInlineCode.FindStringSubmatch(m)
		return reserve("<code>" + html.EscapeString(sub[1]) + "</code>")
	})

	// 3. HTML-escape remaining text, preserving placeholder tokens (which only
	//    contain \x00 and ASCII digits — none of &<>" are present in them).
	result = escapeNonTokens(result)

	// 4. Apply Markdown formatting rules.
	result = reHeader.ReplaceAllString(result, "<b>$1</b>")
	result = reBold.ReplaceAllString(result, "<b>$1</b>")
	result = reItalicStar.ReplaceAllString(result, "<i>$1</i>")
	result = reItalicUnder.ReplaceAllString(result, "<i>$1</i>")
	result = reLink.ReplaceAllString(result, `<a href="$2">$1</a>`)

	// 5. Restore code placeholders.
	for i, ph := range placeholders {
		result = strings.Replace(result, fmt.Sprintf("\x00%d\x00", i), ph, 1)
	}

	return result
}

// escapeNonTokens HTML-escapes &, <, > in s while leaving \x00-delimited
// placeholder tokens untouched.
func escapeNonTokens(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inToken := false
	for _, r := range s {
		if r == '\x00' {
			inToken = !inToken
			b.WriteRune(r)
			continue
		}
		if inToken {
			b.WriteRune(r)
			continue
		}
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
