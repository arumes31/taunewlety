package sanitize

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// allowedTags is the set of HTML tags that are permitted in LLM-generated
// newsletter content. Tags outside this set are unwrapped: the element is
// dropped but its children are kept.
var allowedTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "a": true, "img": true, "ul": true, "ol": true, "li": true,
	"br": true, "hr": true, "strong": true, "em": true, "b": true, "i": true,
	"div": true, "span": true, "table": true, "tr": true, "td": true, "th": true,
	"thead": true, "tbody": true, "blockquote": true, "pre": true, "code": true,
}

// deniedTags are tags removed together with their entire subtree.
var deniedTags = map[string]bool{
	"script": true, "iframe": true, "object": true, "embed": true,
	"form": true, "style": true, "noscript": true, "template": true,
	"base": true, "link": true, "meta": true,
}

// allowedAttrs lists the attributes kept per tag. Anything not listed —
// including every on* event handler and inline style — is dropped.
var allowedAttrs = map[string]map[string]bool{
	"a":   {"href": true, "title": true},
	"img": {"src": true, "alt": true, "title": true, "width": true, "height": true},
	"td":  {"colspan": true, "rowspan": true},
	"th":  {"colspan": true, "rowspan": true},
}

// urlAttrs are the attributes whose value is a URL and therefore must be
// scheme-checked.
var urlAttrs = map[string]bool{"href": true, "src": true}

// allowedSchemes are the URL schemes permitted in href/src. javascript:,
// data:, vbscript: and anything else are rejected. Scheme-relative ("//host")
// and path-relative URLs carry no scheme and are allowed.
var allowedSchemes = map[string]bool{
	"http": true, "https": true, "mailto": true,
}

// HTML sanitizes LLM-generated HTML by parsing it and rebuilding it from an
// allowlist. Denied tags are removed with their contents, tags outside the
// allowlist are unwrapped, and only allowlisted attributes survive. URL
// attributes are decoded and canonicalized before their scheme is checked,
// so obfuscations such as "java&#115;cript:" or "  JaVaScRiPt :" cannot slip
// through.
func HTML(input string) string {
	if input == "" {
		return ""
	}

	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(input), context)
	if err != nil {
		// Parsing an HTML fragment effectively cannot fail, but if it ever
		// does, emit the input as escaped text rather than raw markup.
		return html.EscapeString(input)
	}

	var out strings.Builder
	for _, n := range nodes {
		renderSanitized(&out, n)
	}
	return out.String()
}

// renderSanitized writes n and its descendants to out, applying the tag and
// attribute allowlists.
func renderSanitized(out *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		out.WriteString(html.EscapeString(n.Data))
		return
	case html.ElementNode:
		// fall through
	default:
		// Comments, doctypes and anything else carry no useful content.
		return
	}

	tag := strings.ToLower(n.Data)
	if deniedTags[tag] {
		return // drop the element and everything inside it
	}

	if !allowedTags[tag] {
		renderChildren(out, n) // unwrap: keep the contents, lose the tag
		return
	}

	out.WriteString("<" + tag)
	for _, attr := range n.Attr {
		name, ok := sanitizeAttr(tag, attr)
		if !ok {
			continue
		}
		out.WriteString(" " + name + `="` + html.EscapeString(attr.Val) + `"`)
	}
	out.WriteString(">")

	if isVoidElement(tag) {
		return
	}

	renderChildren(out, n)
	out.WriteString("</" + tag + ">")
}

func renderChildren(out *strings.Builder, n *html.Node) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		renderSanitized(out, child)
	}
}

// sanitizeAttr reports whether attr may be kept on tag, returning its
// normalized name.
func sanitizeAttr(tag string, attr html.Attribute) (string, bool) {
	// Namespaced attributes (xlink:href and friends) are never needed here
	// and are a known vector for smuggling scripts.
	if attr.Namespace != "" {
		return "", false
	}

	name := strings.ToLower(attr.Key)
	if !allowedAttrs[tag][name] {
		return "", false
	}
	if urlAttrs[name] && !isSafeURL(attr.Val) {
		return "", false
	}
	return name, true
}

// isSafeURL reports whether raw carries no scheme, or a scheme on the
// allowlist. The value is canonicalized first: the parser has already decoded
// HTML entities, so this strips the control characters and whitespace that
// browsers ignore when resolving a URL.
func isSafeURL(raw string) bool {
	canonical := strings.Map(func(r rune) rune {
		// Browsers strip tabs, newlines and C0 controls before parsing a URL.
		if r <= 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, raw)

	colon := strings.IndexByte(canonical, ':')
	if colon < 0 {
		return true // relative URL or fragment — no scheme to validate
	}

	// A colon that appears after the first path separator is part of the
	// path, not a scheme (e.g. "/a/b:c" or "//host/a:b").
	if slash := strings.IndexAny(canonical, "/?#"); slash >= 0 && slash < colon {
		return true
	}

	return allowedSchemes[strings.ToLower(canonical[:colon])]
}

// isVoidElement reports whether tag is an HTML void element, which has no
// closing tag.
func isVoidElement(tag string) bool {
	switch tag {
	case "br", "hr", "img":
		return true
	}
	return false
}
