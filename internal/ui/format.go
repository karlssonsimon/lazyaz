package ui

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
)

// FormatDocument pretty-prints a document for the in-memory formatted
// view. JSON first — json.Indent works on the raw bytes, so key order
// and number representations survive untouched. Then XML through the
// stdlib tokenizer round-trip, which may normalize minor syntax
// (self-closing tags, attribute quoting); acceptable for a view that
// never writes back. Anything else refuses.
func FormatDocument(data []byte) ([]byte, string, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, "", false
	}

	if json.Valid(trimmed) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, trimmed, "", "  "); err == nil {
			return buf.Bytes(), "JSON", true
		}
	}

	if out, ok := formatXML(trimmed); ok {
		return out, "XML", true
	}
	return nil, "", false
}

func formatXML(data []byte) ([]byte, bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	sawElement := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false
		}
		// Whitespace-only text between elements is the old formatting;
		// dropping it lets the encoder's indentation win.
		if cd, ok := tok.(xml.CharData); ok && len(bytes.TrimSpace(cd)) == 0 {
			continue
		}
		if _, ok := tok.(xml.StartElement); ok {
			sawElement = true
		}
		if err := enc.EncodeToken(tok); err != nil {
			return nil, false
		}
	}
	if !sawElement {
		return nil, false
	}
	if err := enc.Flush(); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}
