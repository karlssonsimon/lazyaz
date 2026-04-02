package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

type SyntaxPalette struct {
	Key         string
	String      string
	Number      string
	Bool        string
	Punctuation string
	XMLTag      string
	XMLAttr     string
}

// ansiColor holds pre-computed ANSI true-color escape sequences for a single
// foreground color, so we don't depend on lipgloss profile detection.
type ansiColor struct {
	open  string // e.g. "\033[38;2;180;142;173m"
	close string // "\033[0m"
}

func newAnsiColor(hex string) ansiColor {
	r, g, b := hexToRGB(hex)
	return ansiColor{
		open:  fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b),
		close: "\033[0m",
	}
}

func (c ansiColor) render(s string) string {
	return c.open + s + c.close
}

type SyntaxStyles struct {
	tokenColors map[chroma.TokenType]ansiColor
}

func NewSyntaxStyles(p SyntaxPalette) SyntaxStyles {
	s := SyntaxStyles{
		tokenColors: make(map[chroma.TokenType]ansiColor),
	}

	set := func(tt chroma.TokenType, hex string) {
		s.tokenColors[tt] = newAnsiColor(hex)
	}

	// Structural: tags (XML elements, JSON keys)
	set(chroma.NameTag, p.XMLTag)
	// Strings
	set(chroma.LiteralString, p.String)
	set(chroma.LiteralStringDouble, p.String)
	set(chroma.LiteralStringSingle, p.String)
	set(chroma.LiteralStringBacktick, p.String)
	set(chroma.LiteralStringAffix, p.String)
	// Numbers
	set(chroma.LiteralNumber, p.Number)
	set(chroma.LiteralNumberFloat, p.Number)
	set(chroma.LiteralNumberInteger, p.Number)
	// Keywords / constants
	set(chroma.Keyword, p.Key)
	set(chroma.KeywordConstant, p.Bool)
	// Punctuation
	set(chroma.Punctuation, p.Punctuation)
	// XML attributes
	set(chroma.NameAttribute, p.XMLAttr)
	// Comments (XML declaration, etc.)
	set(chroma.Comment, p.Punctuation)
	set(chroma.CommentPreproc, p.Punctuation)

	return s
}

func DetectLexer(filename, contentType string) chroma.Lexer {
	if l := lexers.Match(filename); l != nil {
		return l
	}
	if contentType != "" {
		mime := contentType
		if i := strings.IndexByte(mime, ';'); i >= 0 {
			mime = strings.TrimSpace(mime[:i])
		}
		if l := lexers.MatchMimeType(mime); l != nil {
			return l
		}
	}
	return lexers.Fallback
}

func (s SyntaxStyles) Highlight(filename, contentType, input string) string {
	lexer := DetectLexer(filename, contentType)
	lexer = chroma.Coalesce(lexer)
	return s.renderWithLexer(lexer, input)
}

func (s SyntaxStyles) HighlightJSON(body string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(body), "", "  "); err != nil {
		return s.highlightAsJSON(body)
	}
	return s.highlightAsJSON(buf.String())
}

func (s SyntaxStyles) highlightAsJSON(input string) string {
	lexer := lexers.Get("json")
	if lexer == nil {
		return input
	}
	lexer = chroma.Coalesce(lexer)
	return s.renderWithLexer(lexer, input)
}

func (s SyntaxStyles) renderWithLexer(lexer chroma.Lexer, input string) string {
	iterator, err := lexer.Tokenise(nil, input)
	if err != nil {
		return input
	}

	var buf strings.Builder
	for _, tok := range iterator.Tokens() {
		if c, ok := s.colorForToken(tok.Type); ok {
			buf.WriteString(c.render(tok.Value))
		} else {
			buf.WriteString(tok.Value)
		}
	}

	return buf.String()
}

// colorForToken walks up the chroma token type hierarchy to find a matching
// color. For example, LiteralStringOther inherits from LiteralString.
func (s SyntaxStyles) colorForToken(tt chroma.TokenType) (ansiColor, bool) {
	for t := tt; t > 0; t = t.Parent() {
		if c, ok := s.tokenColors[t]; ok {
			return c, true
		}
	}
	return ansiColor{}, false
}

// hexToRGB parses a hex color string (with or without # prefix) into RGB components.
func hexToRGB(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimLeft(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return uint8(r), uint8(g), uint8(b)
}
