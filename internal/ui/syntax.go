package ui

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

type SyntaxPalette struct {
	Key         string
	String      string
	Number      string
	Bool        string
	Null        string
	Punctuation string
	XMLTag      string
	XMLAttr     string
	CSVCellA    string
	CSVCellB    string
}

type SyntaxStyles struct {
	key      lipgloss.Style
	str      lipgloss.Style
	number   lipgloss.Style
	boolean  lipgloss.Style
	null     lipgloss.Style
	punct    lipgloss.Style
	xmlTag   lipgloss.Style
	xmlAttr  lipgloss.Style
	csvCellA lipgloss.Style
	csvCellB lipgloss.Style
}

var (
	xmlTagPattern  = regexp.MustCompile(`<(?:"[^"]*"|'[^']*'|[^'">])*>`)
	xmlAttrPattern = regexp.MustCompile(`\s([A-Za-z_:][A-Za-z0-9_.:-]*)=`)
)

func NewSyntaxStyles(p SyntaxPalette) SyntaxStyles {
	return SyntaxStyles{
		key:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.Key)),
		str:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.String)),
		number:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Number)),
		boolean:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.Bool)),
		null:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.Null)),
		punct:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.Punctuation)),
		xmlTag:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.XMLTag)),
		xmlAttr:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.XMLAttr)),
		csvCellA: lipgloss.NewStyle().Foreground(lipgloss.Color(p.CSVCellA)),
		csvCellB: lipgloss.NewStyle().Foreground(lipgloss.Color(p.CSVCellB)),
	}
}

func (s SyntaxStyles) Highlight(language, input string) string {
	switch language {
	case "json":
		return s.HighlightJSON(input)
	case "xml":
		return s.HighlightXML(input)
	case "csv":
		return s.HighlightCSV(input)
	default:
		return input
	}
}

func (s SyntaxStyles) HighlightJSON(body string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(body), "", "  "); err != nil {
		return s.colorizeJSONLines(body)
	}
	return s.colorizeJSONLines(buf.String())
}

func (s SyntaxStyles) colorizeJSONLines(input string) string {
	var out strings.Builder
	lines := strings.Split(input, "\n")

	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(s.colorizeJSONLine(line))
	}

	return out.String()
}

func (s SyntaxStyles) colorizeJSONLine(line string) string {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	indent := line[:len(line)-len(trimmed)]

	if trimmed == "" {
		return line
	}

	var out strings.Builder
	out.WriteString(indent)

	if trimmed[0] == '"' {
		colonIdx := strings.Index(trimmed, "\":")
		if colonIdx > 0 {
			key := trimmed[:colonIdx+1]
			rest := trimmed[colonIdx+1:]
			out.WriteString(s.key.Render(key))
			out.WriteString(s.punct.Render(":"))
			val := strings.TrimSpace(rest[1:])
			if val != "" {
				out.WriteString(" ")
				out.WriteString(s.colorizeJSONValue(val))
			}
			return out.String()
		}
		out.WriteString(s.colorizeJSONValue(trimmed))
		return out.String()
	}

	out.WriteString(s.colorizeJSONValue(trimmed))
	return out.String()
}

func (s SyntaxStyles) colorizeJSONValue(val string) string {
	if val == "" {
		return val
	}

	trailing := ""
	clean := val
	for strings.HasSuffix(clean, ",") {
		trailing = "," + trailing
		clean = clean[:len(clean)-1]
	}

	var styled string
	switch {
	case clean == "{" || clean == "}" || clean == "[" || clean == "]" ||
		clean == "{}" || clean == "[]":
		styled = s.punct.Render(clean)
	case clean == "null":
		styled = s.null.Render(clean)
	case clean == "true" || clean == "false":
		styled = s.boolean.Render(clean)
	case len(clean) > 0 && clean[0] == '"':
		styled = s.str.Render(clean)
	default:
		styled = s.number.Render(clean)
	}

	if trailing != "" {
		return styled + s.punct.Render(trailing)
	}
	return styled
}

func (s SyntaxStyles) HighlightXML(input string) string {
	return xmlTagPattern.ReplaceAllStringFunc(input, func(tag string) string {
		colored := s.xmlTag.Render(tag)
		colored = xmlAttrPattern.ReplaceAllString(colored, " "+s.xmlAttr.Render("$1")+s.punct.Render("="))
		return colored
	})
}

func (s SyntaxStyles) HighlightCSV(input string) string {
	lines := strings.Split(input, "\n")
	var out strings.Builder
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		for idx, part := range parts {
			if idx > 0 {
				out.WriteString(s.punct.Render(","))
			}
			if idx%2 == 0 {
				out.WriteString(s.csvCellA.Render(part))
			} else {
				out.WriteString(s.csvCellB.Render(part))
			}
		}
	}
	return out.String()
}
