package sanitize

import (
	"strings"
	"testing"
)

func TestHTML_ScriptRemoval(t *testing.T) {
	input := `<p>Hello</p><script>alert('xss')</script><p>World</p>`
	result := HTML(input)
	if strings.Contains(result, "<script") {
		t.Errorf("script tag should be removed, got: %s", result)
	}
	if strings.Contains(result, "alert") {
		t.Errorf("script content should be removed, got: %s", result)
	}
	if !strings.Contains(result, "<p>Hello</p>") {
		t.Errorf("safe content should be preserved, got: %s", result)
	}
}

func TestHTML_IframeRemoval(t *testing.T) {
	input := `<iframe src="evil.com"></iframe><p>Safe</p>`
	result := HTML(input)
	if strings.Contains(result, "<iframe") {
		t.Errorf("iframe tag should be removed, got: %s", result)
	}
}

func TestHTML_ObjectEmbedFormRemoval(t *testing.T) {
	input := `<object data="x"></object><embed src="y"><form action="z"><input></form><p>OK</p>`
	result := HTML(input)
	if strings.Contains(result, "<object") || strings.Contains(result, "<embed") || strings.Contains(result, "<form") {
		t.Errorf("object/embed/form tags should be removed, got: %s", result)
	}
}

func TestHTML_OnEventAttributes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		bad   string
	}{
		{"onclick", `<p onclick="alert(1)">Hello</p>`, "onclick"},
		{"onload", `<img onload="alert(1)" src="x.png">`, "onload"},
		{"onerror", `<img onerror="alert(1)" src="x.png">`, "onerror"},
		{"onmouseover", `<div onmouseover="alert(1)">Hi</div>`, "onmouseover"},
		{"onfocus", `<input onfocus="alert(1)">`, "onfocus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTML(tt.input)
			if strings.Contains(result, tt.bad) {
				t.Errorf("event attribute should be removed, got: %s", result)
			}
		})
	}
}

func TestHTML_JavascriptURL(t *testing.T) {
	input := `<a href="javascript:alert(1)">Click</a>`
	result := HTML(input)
	if strings.Contains(result, "javascript:") {
		t.Errorf("javascript: URL should be removed, got: %s", result)
	}
}

func TestHTML_AllowedTags(t *testing.T) {
	input := `<p>Paragraph</p><h1>Heading</h1><a href="https://example.com">Link</a><ul><li>Item</li></ul>`
	result := HTML(input)
	if result != input {
		t.Errorf("allowed tags should be preserved, got: %s", result)
	}
}

func TestHTML_DisallowedTagsStripped(t *testing.T) {
	input := `<marquee>Text</marquee><p>OK</p>`
	result := HTML(input)
	if strings.Contains(result, "<marquee") {
		t.Errorf("disallowed tags should be stripped, got: %s", result)
	}
	if !strings.Contains(result, "Text") {
		t.Errorf("inner text of stripped tags should be preserved, got: %s", result)
	}
}

func TestHTML_StyleTagRemoval(t *testing.T) {
	input := `<style>body{background:url('javascript:alert(1)')}</style><p>OK</p>`
	result := HTML(input)
	if strings.Contains(result, "<style") {
		t.Errorf("style tag should be removed, got: %s", result)
	}
}

func TestHTML_SelfClosingScript(t *testing.T) {
	input := `<script src="evil.js" /><p>Safe</p>`
	result := HTML(input)
	if strings.Contains(result, "<script") {
		t.Errorf("self-closing script should be removed, got: %s", result)
	}
}

func TestHTML_MixedContent(t *testing.T) {
	input := `<h1>Newsletter</h1><script>steal()</script><p onclick="bad()">Good content</p><iframe src="evil"></iframe>`
	result := HTML(input)
	if strings.Contains(result, "<script") {
		t.Errorf("script should be removed, got: %s", result)
	}
	if strings.Contains(result, "onclick") {
		t.Errorf("onclick should be removed, got: %s", result)
	}
	if strings.Contains(result, "<iframe") {
		t.Errorf("iframe should be removed, got: %s", result)
	}
	if !strings.Contains(result, "<h1>Newsletter</h1>") {
		t.Errorf("safe heading should be preserved, got: %s", result)
	}
	if !strings.Contains(result, "Good content") {
		t.Errorf("safe text should be preserved, got: %s", result)
	}
}

func TestHTML_TableTags(t *testing.T) {
	input := `<table><thead><tr><th>H</th></tr></thead><tbody><tr><td>D</td></tr></tbody></table>`
	result := HTML(input)
	if result != input {
		t.Errorf("table tags should be preserved, got: %s", result)
	}
}

func TestHTML_EmptyInput(t *testing.T) {
	result := HTML("")
	if result != "" {
		t.Errorf("empty input should return empty, got: %s", result)
	}
}

func TestHTML_NestedScriptInAllowed(t *testing.T) {
	input := `<div><script>alert(1)</script>Visible text</div>`
	result := HTML(input)
	if strings.Contains(result, "<script") {
		t.Errorf("nested script should be removed, got: %s", result)
	}
	if !strings.Contains(result, "Visible text") {
		t.Errorf("text after script should be preserved, got: %s", result)
	}
}
