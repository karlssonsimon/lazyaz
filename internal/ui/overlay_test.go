package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderOverlayListNoteRowIsMuted(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	ov := styles.Overlay
	bg := ov.Normal.GetBackground()
	items := []OverlayItem{
		{Label: "row"},
		{Label: "a note", IsNote: true},
	}

	out := RenderOverlayList(OverlayListConfig{NoActiveMarker: true}, items, 0, styles, 100, 40, "")

	if !strings.Contains(out, ov.NoMatch.Background(bg).Render("a note")) {
		t.Errorf("note not rendered with the muted style:\n%q", out)
	}
	if strings.Contains(out, ov.SectionTitle.Background(bg).Render("a note")) {
		t.Errorf("note rendered as a section title:\n%q", out)
	}
}

func TestHighlightMatchFollowsMode(t *testing.T) {
	mark := lipgloss.NewStyle().Transform(func(s string) string { return "[" + s + "]" })
	base := lipgloss.NewStyle()
	label := "Azure Kubernetes Service RBAC Reader"

	if got, want := highlightMatch(label, "reader", true, mark, base), "Azure Kubernetes Service RBAC [Reader]"; got != want {
		t.Errorf("exact highlight = %q, want %q", got, want)
	}
	// A label that only matches with scattered letters is highlighted in
	// fuzzy mode and left plain in exact mode.
	scattered := "Storage Account Contributor shared-data-archive"
	if got := highlightMatch(scattered, "rbac", true, mark, base); got != scattered {
		t.Errorf("exact highlight of a non-match = %q, want it unmarked", got)
	}
	if got := highlightMatch(scattered, "rbac", false, mark, base); !strings.Contains(got, "[") {
		t.Errorf("fuzzy highlight = %q, want scattered letters marked", got)
	}
}

// A row can be filtered on text its label does not show (a tenant name, for
// one). The terms that do occur in the label are still highlighted.
func TestHighlightMatchMarksTermsFoundInLabel(t *testing.T) {
	mark := lipgloss.NewStyle().Transform(func(s string) string { return "[" + s + "]" })
	base := lipgloss.NewStyle()
	label := "Storage Blob Data Reader  sub-a-prod"

	if got, want := highlightMatch(label, "contoso reader", true, mark, base), "Storage Blob Data [Reader]  sub-a-prod"; got != want {
		t.Errorf("highlight = %q, want %q", got, want)
	}
	if got := highlightMatch(label, "contoso !reader", true, mark, base); got != label {
		t.Errorf("highlight = %q, want a negated term left unmarked", got)
	}
}

func TestRenderOverlayListCountsSelectableRows(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	items := []OverlayItem{
		{Label: "a note", IsNote: true},
		{Label: "GROUP", IsHeader: true},
		{Label: "row one"},
		{Label: "row two"},
	}

	out := RenderOverlayList(OverlayListConfig{Query: "row", NoActiveMarker: true}, items, 2, styles, 100, 40, "")

	if !strings.Contains(out, "2 matches") {
		t.Errorf("header does not count the two selectable rows:\n%q", out)
	}
}
