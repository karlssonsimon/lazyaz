package blobapp

import (
	"fmt"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	tea "charm.land/bubbletea/v2"
)

// BenchmarkVisualCursorMove200kFiltered exercises the visual-mode
// cursor-move hot path at the scale that made it lag: 200k blobs with a
// list filter applied. Every j press must stay O(1)-ish — no full-list
// scans, no per-keypress materialization of the selection.
func BenchmarkVisualCursorMove200kFiltered(b *testing.B) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 40
	m.hasAccount = true
	m.hasContainer = true
	m.focus = blobsPane

	m.blobs = make([]blob.BlobEntry, 200_000)
	for i := range m.blobs {
		m.blobs[i] = blob.BlobEntry{Name: fmt.Sprintf("data/blob-%06d.json", i)}
	}
	m.refreshItems()
	m.blobsList.SetFilterText("blob")
	m.blobsList.Select(0)

	m.visual.Start(m.blobs[0].Name)

	down := tea.KeyPressMsg{Code: 'j', Text: "j"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, _ = m.handleVisualLineKey(down, "j")
	}
}
