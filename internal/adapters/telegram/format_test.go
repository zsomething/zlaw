package telegram

import (
	"testing"
)

func TestMdToHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "bold",
			input: "**bold text**",
			want:  "<b>bold text</b>",
		},
		{
			name:  "italic star",
			input: "*italic text*",
			want:  "<i>italic text</i>",
		},
		{
			name:  "italic underscore",
			input: "_italic text_",
			want:  "<i>italic text</i>",
		},
		{
			name:  "inline code",
			input: "`some code`",
			want:  "<code>some code</code>",
		},
		{
			name:  "inline code escapes HTML",
			input: "`a < b && b > c`",
			want:  "<code>a &lt; b &amp;&amp; b &gt; c</code>",
		},
		{
			name:  "fenced code block",
			input: "```\nfoo bar\n```",
			want:  "<pre>foo bar\n</pre>",
		},
		{
			name:  "fenced code block with language",
			input: "```go\nfmt.Println()\n```",
			want:  "<pre><code class=\"language-go\">fmt.Println()\n</code></pre>",
		},
		{
			name:  "link",
			input: "[click here](https://example.com)",
			want:  `<a href="https://example.com">click here</a>`,
		},
		{
			name:  "header",
			input: "## Section Title",
			want:  "<b>Section Title</b>",
		},
		{
			name:  "html special chars in plain text",
			input: "a < b & c > d",
			want:  "a &lt; b &amp; c &gt; d",
		},
		{
			name:  "no formatting inside code",
			input: "`**not bold**`",
			want:  "<code>**not bold**</code>",
		},
		{
			name:  "bold before italic",
			input: "**bold** and *italic*",
			want:  "<b>bold</b> and <i>italic</i>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mdToHTML(tt.input)
			if got != tt.want {
				t.Errorf("mdToHTML(%q)\n got:  %q\n want: %q", tt.input, got, tt.want)
			}
		})
	}
}
