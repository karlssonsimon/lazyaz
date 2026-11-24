package ui

import (
	"testing"

	"github.com/alecthomas/chroma/v2/lexers"
)

func TestIsProbablyBinary(t *testing.T) {
	if IsProbablyBinary([]byte("hello\nworld")) {
		t.Fatal("expected plain text to be non-binary")
	}

	if !IsProbablyBinary([]byte{0x00, 0x01, 0x02}) {
		t.Fatal("expected null/control bytes to be binary")
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
