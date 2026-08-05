package blobapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/vim"
)

const minifiedJSON = `{"alpha":1,"beta":{"gamma":[1,2,3]},"delta":"end"}`

// = formats the loaded document in place: the window becomes the whole
// formatted document, and = again restores the raw view exactly.
func TestPreviewFormatToggleRestoresExactly(t *testing.T) {
	m := searchModel(t, minifiedJSON)
	rawSize := m.preview.blobSize
	rawData := string(m.preview.windowData)

	m = typeKeys(m, "=")
	if !m.preview.formatted || m.preview.formatKind != "JSON" {
		t.Fatalf("formatted = %v kind = %q, want true JSON", m.preview.formatted, m.preview.formatKind)
	}
	if !strings.Contains(string(m.preview.windowData), "\n") {
		t.Error("window still holds the minified document")
	}
	if m.preview.windowStart != 0 || m.preview.blobSize != int64(len(m.preview.windowData)) {
		t.Errorf("window is not the whole document: start %d size %d len %d",
			m.preview.windowStart, m.preview.blobSize, len(m.preview.windowData))
	}
	if m.preview.cursor != 0 {
		t.Errorf("cursor = %d, want 0 after formatting", m.preview.cursor)
	}
	if len(m.preview.lineStarts) < 2 {
		t.Error("line index not rebuilt for the formatted document")
	}

	m = typeKeys(m, "=")
	if m.preview.formatted {
		t.Fatal("second = did not restore the raw view")
	}
	if string(m.preview.windowData) != rawData || m.preview.blobSize != rawSize {
		t.Error("raw view not restored exactly")
	}
	if m.preview.rawStash != nil {
		t.Error("stash not dropped on restore")
	}
}

// = must work from inside the vim capture too, and the capture survives
// the toggle.
func TestPreviewFormatFromVimCapture(t *testing.T) {
	m := searchModel(t, minifiedJSON)
	m = typeKeys(m, "v", "=")
	if !m.preview.formatted {
		t.Fatal("= inside the capture did not format")
	}
	if !m.preview.vimMode {
		t.Error("formatting dropped the vim capture")
	}
	if m.preview.vcur != (vim.Cursor{}) {
		t.Errorf("vim cursor = %+v, want origin", m.preview.vcur)
	}
}

// Search while formatted runs against the in-memory document — matches
// that only exist in the pretty-printed text must be found without a
// service.
func TestPreviewFormatSearchInMemory(t *testing.T) {
	m := searchModel(t, minifiedJSON)
	m = typeKeys(m, "=")

	m = typeKeys(m, "/")
	for _, r := range "gamma" {
		m = typeKeys(m, string(r))
	}
	m = typeKeys(m, "enter")

	want := strings.Index(string(m.preview.windowData), "gamma")
	if m.preview.cursor != int64(want) {
		t.Errorf("cursor = %d, want %d (offset of gamma in the formatted text)", m.preview.cursor, want)
	}
	if m.preview.search.scanning {
		t.Error("formatted search kicked off a streamed scan")
	}
}

// Yanking in formatted mode yanks formatted text.
func TestPreviewFormatYankSelectsFormattedText(t *testing.T) {
	m := searchModel(t, minifiedJSON)
	m = typeKeys(m, "=", "v", "y", "y")

	// yy on the first line of the formatted document is "{\n".
	// The clipboard write is async; instead verify via the region the
	// grammar produced: the first line of the window must be "{".
	if got := m.previewBuf().Line(0); got != "{" {
		t.Errorf("first formatted line = %q, want {", got)
	}
}

// Non-parseable content refuses and changes nothing.
func TestPreviewFormatRefusesNonDocument(t *testing.T) {
	m := searchModel(t, "just some log lines\nanother line\n")
	before := string(m.preview.windowData)

	m = typeKeys(m, "=")
	if m.preview.formatted {
		t.Fatal("plain text was formatted")
	}
	if string(m.preview.windowData) != before {
		t.Error("refusal still changed the window")
	}
}

// Over the format budget the toggle refuses outright.
func TestPreviewFormatRefusesOverBudget(t *testing.T) {
	m := searchModel(t, minifiedJSON)
	m.formatBudget = 4

	m = typeKeys(m, "=")
	if m.preview.formatted {
		t.Fatal("formatted despite the budget")
	}
}

// Opening another blob drops the formatted view.
func TestPreviewFormatDropsOnNewBlob(t *testing.T) {
	m := searchModel(t, minifiedJSON)
	m = typeKeys(m, "=")
	if !m.preview.formatted {
		t.Fatal("setup: format failed")
	}

	m2, _ := m.openPreview(blob.BlobEntry{Name: "other.json", Size: 10})
	if m2.preview.formatted || m2.preview.rawStash != nil {
		t.Error("format state survived onto a different blob")
	}
}
