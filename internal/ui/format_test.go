package ui

import (
	"strings"
	"testing"
)

func TestFormatDocumentJSON(t *testing.T) {
	// Key order must survive: json.Indent works on the raw bytes, so
	// "z" staying before "a" proves nothing was re-marshalled.
	out, kind, ok := FormatDocument([]byte(`{"z":1,"a":{"b":[1,2]}}`))
	if !ok {
		t.Fatal("valid JSON refused")
	}
	if kind != "JSON" {
		t.Fatalf("kind = %q, want JSON", kind)
	}
	text := string(out)
	if !strings.Contains(text, "\n") {
		t.Error("formatted JSON has no line breaks")
	}
	if strings.Index(text, `"z"`) > strings.Index(text, `"a"`) {
		t.Error("key order changed — the document was re-marshalled")
	}
}

func TestFormatDocumentXML(t *testing.T) {
	out, kind, ok := FormatDocument([]byte(`<root><item id="1">alpha</item><item id="2">beta</item></root>`))
	if !ok {
		t.Fatal("valid XML refused")
	}
	if kind != "XML" {
		t.Fatalf("kind = %q, want XML", kind)
	}
	text := string(out)
	if !strings.Contains(text, "\n") {
		t.Error("formatted XML has no line breaks")
	}
	if !strings.Contains(text, "alpha") || !strings.Contains(text, `id="1"`) {
		t.Errorf("content lost in round-trip: %q", text)
	}
}

func TestFormatDocumentJSONWinsOverXML(t *testing.T) {
	// A JSON document containing angle brackets must not be mistaken
	// for XML.
	_, kind, ok := FormatDocument([]byte(`{"html":"<b>hi</b>"}`))
	if !ok || kind != "JSON" {
		t.Fatalf("kind = %q ok = %v, want JSON true", kind, ok)
	}
}

// Real-world XML declares charsets and uses HTML entities; both must
// format instead of refusing.
func TestFormatDocumentXMLRealWorld(t *testing.T) {
	t.Run("xml declaration", func(t *testing.T) {
		_, kind, ok := FormatDocument([]byte(`<?xml version="1.0" encoding="UTF-8"?><root><a>1</a></root>`))
		if !ok || kind != "XML" {
			t.Fatalf("kind = %q ok = %v, want XML true", kind, ok)
		}
	})

	t.Run("iso-8859-1 charset", func(t *testing.T) {
		// é in latin-1 is the single byte 0xE9 — invalid UTF-8, so this
		// exercises the charset reader, not just the declaration.
		doc := append([]byte(`<?xml version="1.0" encoding="ISO-8859-1"?><root><name>caf`), 0xE9)
		doc = append(doc, []byte(`</name></root>`)...)
		out, kind, ok := FormatDocument(doc)
		if !ok || kind != "XML" {
			t.Fatalf("kind = %q ok = %v, want XML true", kind, ok)
		}
		if !strings.Contains(string(out), "café") {
			t.Errorf("latin-1 content not transcoded: %q", out)
		}
	})

	t.Run("html entity", func(t *testing.T) {
		_, kind, ok := FormatDocument([]byte(`<root><a>x&nbsp;y</a></root>`))
		if !ok || kind != "XML" {
			t.Fatalf("kind = %q ok = %v, want XML true", kind, ok)
		}
	})
}

func TestFormatDocumentRefusals(t *testing.T) {
	cases := map[string]string{
		"garbage":       "not a document at all",
		"empty":         "",
		"whitespace":    "   \n\t  ",
		"broken JSON":   `{"a":`,
		"broken XML":    `<root><unclosed>`,
		"no XMLeleemnt": `plain text with < and >`,
	}
	for name, input := range cases {
		if _, _, ok := FormatDocument([]byte(input)); ok {
			t.Errorf("%s: accepted %q, want refusal", name, input)
		}
	}
}
