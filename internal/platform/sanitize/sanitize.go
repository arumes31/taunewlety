package sanitize

import (
	"regexp"
	"strings"
)

// allowedTags is the set of HTML tags that are permitted in LLM-generated
// newsletter content. All other tags are stripped.
var allowedTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "a": true, "img": true, "ul": true, "ol": true, "li": true,
	"br": true, "hr": true, "strong": true, "em": true, "b": true, "i": true,
	"div": true, "span": true, "table": true, "tr": true, "td": true, "th": true,
	"thead": true, "tbody": true, "blockquote": true, "pre": true, "code": true,
}

// deniedTags are tags whose content (including nested content) is removed entirely.
var deniedTags = []string{"script", "iframe", "object", "embed", "form", "style"}

// reOnAttributes matches event-handler attributes like onclick, onload, onerror, etc.
var reOnAttributes = regexp.MustCompile(`(?i)\bon\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)

// reJavascriptURL matches javascript: in href/src attributes.
var reJavascriptURL = regexp.MustCompile(`(?i)(href|src)\s*=\s*["']?\s*javascript\s*:`)

// reOpenTag matches an opening HTML tag, capturing the tag name.
var reOpenTag = regexp.MustCompile(`(?i)<\s*/?([a-zA-Z][a-zA-Z0-9]*)\b[^>]*>`)

// reDeniedTagBlock matches an entire denied tag block including its content,
// e.g. <script>...</script> including nested content.
func buildDeniedTagRegex(tag string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)<\s*` + tag + `\b[^>]*>.*?<\s*/\s*` + tag + `\s*>`)
}

// reSelfClosingDenied matches self-closing or unclosed denied tags.
func buildDeniedSelfClosingRegex(tag string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)<\s*` + tag + `\b[^>]*/?\s*>`)
}

// HTML sanitizes LLM-generated HTML by removing dangerous tags and attributes.
// It strips <script>, <iframe>, <object>, <embed>, <form>, <style> tags and their
// contents, removes on* event handler attributes, and removes javascript: URLs.
// Tags not in the allowlist are stripped (opening and closing tags removed, but
// their inner content is preserved).
func HTML(input string) string {
	s := input

	// 1. Remove denied tag blocks (tag + all content inside)
	for _, tag := range deniedTags {
		re := buildDeniedTagRegex(tag)
		s = re.ReplaceAllString(s, "")
		// Also remove self-closing or unclosed variants
		reSC := buildDeniedSelfClosingRegex(tag)
		s = reSC.ReplaceAllString(s, "")
	}

	// 2. Remove on* event attributes
	s = reOnAttributes.ReplaceAllString(s, "")

	// 3. Remove javascript: URLs
	s = reJavascriptURL.ReplaceAllString(s, "$1=\"\"")

	// 4. Strip tags that are not in the allowlist (keep inner text)
	s = reOpenTag.ReplaceAllStringFunc(s, func(match string) string {
		submatch := reOpenTag.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return "" // remove malformed tag
		}
		tagName := strings.ToLower(submatch[1])
		if allowedTags[tagName] {
			// For allowed tags, also strip any remaining dangerous attributes
			// (on* attributes already removed above, but double-check for style with expressions)
			cleaned := reOnAttributes.ReplaceAllString(match, "")
			return cleaned
		}
		return "" // strip disallowed tag, inner content preserved by not matching it
	})

	return s
}
