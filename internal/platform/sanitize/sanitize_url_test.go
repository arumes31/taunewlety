package sanitize

import (
	"strings"
	"testing"
)

// TestHTML_UnsafeURLSchemes covers the obfuscations that the previous
// regex-based filter missed: entity-encoded schemes, embedded control
// characters, and case/whitespace tricks.
func TestHTML_UnsafeURLSchemes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain javascript", `<a href="javascript:alert(1)">x</a>`},
		{"uppercase javascript", `<a href="JavaScript:alert(1)">x</a>`},
		{"mixed case javascript", `<a href="JaVaScRiPt:alert(1)">x</a>`},
		{"leading whitespace", `<a href="   javascript:alert(1)">x</a>`},
		{"embedded tab", `<a href="java&#9;script:alert(1)">x</a>`},
		{"embedded newline", `<a href="java&#10;script:alert(1)">x</a>`},
		{"entity encoded char", `<a href="java&#115;cript:alert(1)">x</a>`},
		{"hex entity encoded", `<a href="&#x6a;avascript:alert(1)">x</a>`},
		{"null byte", "<a href=\"java\x00script:alert(1)\">x</a>"},
		{"data uri", `<a href="data:text/html;base64,PHNjcmlwdD4=">x</a>`},
		{"data uri image src", `<img src="data:text/html,<script>alert(1)</script>">`},
		{"vbscript", `<a href="vbscript:msgbox(1)">x</a>`},
		{"single quoted attribute", `<a href='javascript:alert(1)'>x</a>`},
		{"unquoted attribute", `<a href=javascript:alert(1)>x</a>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strings.ToLower(HTML(tt.input))
			for _, bad := range []string{"javascript", "vbscript", "data:", "alert(1)", "msgbox"} {
				if strings.Contains(result, bad) {
					t.Errorf("unsafe URL survived sanitization (%q present), got: %s", bad, result)
				}
			}
		})
	}
}

func TestHTML_SafeURLsPreserved(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"https", `<a href="https://example.com/x?y=1">L</a>`, `href="https://example.com/x?y=1"`},
		{"http", `<a href="http://example.com">L</a>`, `href="http://example.com"`},
		{"mailto", `<a href="mailto:a@b.com">L</a>`, `href="mailto:a@b.com"`},
		{"relative path", `<a href="/unsubscribe">L</a>`, `href="/unsubscribe"`},
		{"fragment", `<a href="#top">L</a>`, `href="#top"`},
		{"scheme relative", `<a href="//cdn.example.com/a">L</a>`, `href="//cdn.example.com/a"`},
		{"colon in path", `<a href="/a/b:c">L</a>`, `href="/a/b:c"`},
		{"image src", `<img src="https://example.com/p.png" alt="p">`, `src="https://example.com/p.png"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTML(tt.input)
			if !strings.Contains(result, tt.want) {
				t.Errorf("expected %q to survive, got: %s", tt.want, result)
			}
		})
	}
}

func TestHTML_AttributeAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		absent  string
		present string
	}{
		{"style attribute dropped", `<p style="width:expression(alert(1))">T</p>`, "style", "T"},
		{"unknown attribute dropped", `<p data-evil="x">T</p>`, "data-evil", "T"},
		{"srcset dropped", `<img src="https://e.com/a.png" srcset="evil">`, "srcset", "src="},
		{"xlink href dropped", `<a xlink:href="javascript:alert(1)">T</a>`, "xlink", "T"},
		{"formaction dropped", `<a href="/x" formaction="javascript:alert(1)">T</a>`, "formaction", "T"},
		{"alt preserved", `<img src="https://e.com/a.png" alt="hello">`, "", `alt="hello"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTML(tt.input)
			if tt.absent != "" && strings.Contains(strings.ToLower(result), tt.absent) {
				t.Errorf("expected %q to be dropped, got: %s", tt.absent, result)
			}
			if !strings.Contains(result, tt.present) {
				t.Errorf("expected %q to be preserved, got: %s", tt.present, result)
			}
		})
	}
}

func TestHTML_TextIsEscaped(t *testing.T) {
	result := HTML(`<p>5 < 6 & 7 > 4</p>`)
	if strings.Contains(result, "< 6") {
		t.Errorf("bare angle bracket should be escaped, got: %s", result)
	}
	if !strings.Contains(result, "&amp;") {
		t.Errorf("ampersand should be escaped, got: %s", result)
	}
}

func TestHTML_MalformedMarkup(t *testing.T) {
	// Unbalanced and nested-quote markup must not let a script through.
	inputs := []string{
		`<p>unclosed`,
		`<div><p>mismatched</div></p>`,
		`<a href="x" onclick="alert(1)"<script>alert(2)</script>>y</a>`,
		`<<script>script>alert(1)<</script>/script>`,
		`<img src=x onerror=alert(1)>`,
	}
	for _, in := range inputs {
		result := HTML(in)
		if strings.Contains(result, "<script") || strings.Contains(strings.ToLower(result), "onerror") ||
			strings.Contains(strings.ToLower(result), "onclick") {
			t.Errorf("malformed markup %q produced unsafe output: %s", in, result)
		}
	}
}

func TestHTML_CommentsRemoved(t *testing.T) {
	result := HTML(`<p>A</p><!-- a comment --><p>B</p>`)
	if strings.Contains(result, "a comment") || strings.Contains(result, "<!--") {
		t.Errorf("comments should be removed, got: %s", result)
	}
}

func TestHTML_VoidElementsNotClosed(t *testing.T) {
	result := HTML(`<p>a<br>b</p><hr>`)
	if strings.Contains(result, "</br>") || strings.Contains(result, "</hr>") {
		t.Errorf("void elements must not get closing tags, got: %s", result)
	}
}
