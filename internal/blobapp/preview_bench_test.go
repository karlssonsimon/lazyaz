package blobapp

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkPreviewViewFrame renders a full frame with a realistic
// syntax-highlighted preview window open. Every frame — every keystroke,
// every spinner tick, every mouse-motion event — pays whatever this
// costs.
func BenchmarkPreviewViewFrame(b *testing.B) {
	m := newPreviewBenchModel()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

// BenchmarkPreviewSetContent isolates the viewport.SetContent call that
// View performs on every frame.
func BenchmarkPreviewSetContent(b *testing.B) {
	m := newPreviewBenchModel()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.preview.viewport.SetContent(m.preview.rendered)
	}
}

func newPreviewBenchModel() Model {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 160, 50
	m.hasAccount = true
	m.currentAccount.Name = "bench-account"
	m.hasContainer = true
	m.containerName = "bench-container"
	m.focus = blobsPane
	m.preview.open = true
	m.preview.blobName = "data/records.json"
	m.preview.contentType = "application/json"

	// ~100 KB of JSON, the size of a typical preview window.
	var sb strings.Builder
	sb.WriteString("[\n")
	for i := 0; sb.Len() < 100*1024; i++ {
		fmt.Fprintf(&sb, "  {\"id\": %d, \"name\": \"record-%06d\", \"active\": true, \"score\": %d.5},\n", i, i, i)
	}
	sb.WriteString("]\n")
	raw := sb.String()

	m.preview.blobSize = int64(len(raw))
	m.preview.rendered = renderPreviewContent([]byte(raw), m.preview.blobName, m.preview.contentType, false, m.Styles)
	m.preview.viewport.SetContent(m.preview.rendered)
	m.resize()
	return m
}
