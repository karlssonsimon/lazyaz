package blobapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// schemeWithStringColor returns a scheme whose syntax colors differ from
// the fallback, so a re-highlight is detectable in the rendered output.
func schemeWithStringColor(name, base0B string) ui.Scheme {
	s := ui.FallbackScheme()
	s.Name = name
	s.Base0B = base0B
	return s
}

// Syntax highlighting bakes the scheme's colors into the string at
// render time. Switching theme has to re-highlight the open preview,
// otherwise the pane keeps painting the previous theme's colors until
// the blob happens to be reloaded.
func TestApplySchemeRehighlightsOpenPreview(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 40
	m.hasAccount = true
	m.hasContainer = true
	m.focus = blobsPane
	m.resize()

	body := []byte(`{"name": "hello world"}`)
	m.preview.open = true
	m.preview.blobName = "data.json"
	m.preview.contentType = "application/json"
	m.preview.blobSize = int64(len(body))
	m.preview.windowData = body
	m.preview.lineStarts = computeLineStarts(body)

	m.ApplyScheme(schemeWithStringColor("before", "22C55E"))
	m.preview.rendered = renderPreviewContent(body, m.preview.blobName, m.preview.contentType, false, m.Styles)
	m.preview.viewport.SetContent(m.preview.rendered)
	before := m.preview.rendered

	m.ApplyScheme(schemeWithStringColor("after", "FF00AA"))
	after := m.preview.rendered

	if before == after {
		t.Fatal("preview still carries the previous theme's colors after a scheme change")
	}
	if !strings.Contains(after, "hello world") {
		t.Errorf("re-highlighted preview lost its content:\n%s", after)
	}
}

// A closed preview has nothing to re-render, and the scheme change must
// not resurrect stale content into the viewport.
func TestApplySchemeLeavesClosedPreviewAlone(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 40
	m.resize()

	m.ApplyScheme(schemeWithStringColor("after", "FF00AA"))

	if m.preview.rendered != "" {
		t.Errorf("closed preview gained content on scheme change: %q", m.preview.rendered)
	}
}
