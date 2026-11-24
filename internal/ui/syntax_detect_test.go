package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/lexers"
)

func TestIsProbablyBinary(t *testing.T) {
	text := strings.Repeat("an ordinary line of log output\n", 100)

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, false},
		{"plain text", []byte("hello\nworld"), false},
		{"null and control bytes", []byte{0x00, 0x01, 0x02}, true},
		{"crlf with tabs", []byte("a\tb\r\nc\td\r\n"), false},
		{"latin-1 text", []byte("r\xe4ksm\xf6rg\xe5s\n"), false},
		{"text with one stray NUL", []byte(text + "\x00" + text), false},
		{"terminal color codes", []byte(strings.Repeat("\x1b[32mINFO\x1b[0m ok\n", 50)), false},
		{"utf-16 without decoding", []byte("h\x00e\x00l\x00l\x00o\x00\n\x00"), true},
		{"png header", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x01\x00\x00\x00\x01\x00\x08\x06\x00\x00\x00"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsProbablyBinary(tt.data); got != tt.want {
				t.Errorf("IsProbablyBinary = %v, want %v", got, tt.want)
			}
		})
	}
}

// Evenly spread bytes are what compressed data looks like; it must stay
// on the binary side of the threshold.
func TestIsProbablyBinaryUniformBytes(t *testing.T) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i * 131)
	}
	if !IsProbablyBinary(data) {
		t.Error("uniformly distributed bytes were taken for text")
	}
}

func TestDecodeUTF16(t *testing.T) {
	// "hi\n" then "åä" in both byte orders.
	le := []byte{0xFF, 0xFE, 'h', 0, 'i', 0, '\n', 0, 0xE5, 0, 0xE4, 0}
	be := []byte{0xFE, 0xFF, 0, 'h', 0, 'i', 0, '\n', 0, 0xE5, 0, 0xE4}

	for name, data := range map[string][]byte{"little endian": le, "big endian": be} {
		if !HasUTF16BOM(data) {
			t.Errorf("%s: byte order mark not recognised", name)
		}
		got, err := DecodeUTF16(data)
		if err != nil {
			t.Fatalf("%s: DecodeUTF16 failed: %v", name, err)
		}
		if want := []byte("hi\nåä"); !bytes.Equal(got, want) {
			t.Errorf("%s: decoded %q, want %q", name, got, want)
		}
	}

	if HasUTF16BOM([]byte("plain")) {
		t.Error("plain text reported a UTF-16 byte order mark")
	}
}

// The content types storage puts on untyped uploads say nothing about
// the language, so they must not pick a lexer.
func TestDetectLexerIgnoresGenericContentTypes(t *testing.T) {
	for _, tc := range []struct{ name, contentType string }{
		{"app.log", "text/plain"},
		{"export", "application/octet-stream"},
		{"notes", "text/plain; charset=utf-8"},
	} {
		if got := DetectLexer(tc.name, tc.contentType); got != lexers.Fallback {
			t.Errorf("DetectLexer(%q, %q) = %s, want the plain-text fallback", tc.name, tc.contentType, got.Config().Name)
		}
	}

	if got := DetectLexer("data", "application/json"); got == lexers.Fallback {
		t.Error("a specific content type no longer selects its lexer")
	}
	if got := DetectLexer("data.json", "application/octet-stream"); got == lexers.Fallback {
		t.Error("the file name no longer selects a lexer when the content type is generic")
	}
}
