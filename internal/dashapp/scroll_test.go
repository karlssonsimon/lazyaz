package dashapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

// makeModel builds a Model wired with the minimum state the scroll
// helpers need: registered widgets, row heights, focus index, and
// data backing for whatever widget RowCount reads.
func makeModel(focusedIdx, topH, botH, totalRows int) Model {
	m := Model{
		widgets:    dashboardWidgets(),
		focusedIdx: focusedIdx,
		// rowHeights are PANE heights including border. innerHeight is
		// height - 2 inside the renderer; visible data rows are
		// inner - 2 (title + header). So pane height H gives visible
		// = H - 4 data rows, matching the previous focusedWidgetDims
		// behaviour.
		rowHeights: []int{topH, botH},
	}
	m.offsets = make([]int, len(m.widgets))

	if focusedIdx == 0 {
		// Top widget = namespaceCounts. Total = len(m.namespaces).
		m.namespaces = make([]servicebus.Namespace, totalRows)
		for i := range m.namespaces {
			m.namespaces[i] = servicebus.Namespace{Name: "ns"}
		}
		return m
	}
	// Bottom widget = dlqAlerts. Total comes from m.dlqAlerts(): one
	// queue with DLQ count > 0 per row.
	m.namespaces = []servicebus.Namespace{{Name: "ns0"}}
	ents := make([]servicebus.Entity, totalRows)
	for i := range ents {
		ents[i] = servicebus.Entity{Name: "q", Kind: servicebus.EntityQueue, DeadLetterCount: 1}
	}
	m.entitiesByNS = map[string][]servicebus.Entity{"ns0": ents}
	return m
}

func TestFocusedWidgetDimsTopWidget(t *testing.T) {
	m := makeModel(0, 10, 12, 7)
	total, visible := m.focusedWidgetDims()
	if total != 7 {
		t.Errorf("total = %d, want 7", total)
	}
	// pane H = 10; visible data = 10 - 4 = 6.
	if visible != 6 {
		t.Errorf("visible = %d, want 6", visible)
	}
}

func TestFocusedWidgetDimsBottomWidget(t *testing.T) {
	m := makeModel(1, 10, 12, 5)
	total, visible := m.focusedWidgetDims()
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	// pane H = 12; visible data = 12 - 4 = 8.
	if visible != 8 {
		t.Errorf("visible = %d, want 8", visible)
	}
}

func TestFocusedWidgetDimsTinyHeightFlooredToOne(t *testing.T) {
	m := makeModel(0, 4, 4, 3)
	_, visible := m.focusedWidgetDims()
	if visible != 1 {
		t.Errorf("visible = %d, want 1 (floor)", visible)
	}
}

func TestMaxFocusedOffsetWhenAllRowsFit(t *testing.T) {
	m := makeModel(0, 20, 20, 5) // visible = 16, total = 5 → no overflow
	if got := m.maxFocusedOffset(); got != 0 {
		t.Errorf("maxFocusedOffset = %d, want 0 (no overflow)", got)
	}
}

func TestMaxFocusedOffsetWhenOverflowing(t *testing.T) {
	// pane H = 10 → visible = 6; total = 100 → maxOffset = 100 - 5 = 95
	m := makeModel(0, 10, 10, 100)
	if got := m.maxFocusedOffset(); got != 95 {
		t.Errorf("maxFocusedOffset = %d, want 95", got)
	}
}

func TestScrollFocusedClampsAtZero(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	m.scrollFocused(-5)
	if m.offsets[0] != 0 {
		t.Errorf("offsets[0] = %d, want 0", m.offsets[0])
	}
}

func TestScrollFocusedClampsAtMax(t *testing.T) {
	m := makeModel(0, 10, 10, 100) // maxOffset = 95
	m.scrollFocused(1000)
	if m.offsets[0] != 95 {
		t.Errorf("offsets[0] = %d, want 95", m.offsets[0])
	}
}

func TestScrollFocusedRoutesByFocus(t *testing.T) {
	m := makeModel(1, 10, 10, 100)
	m.scrollFocused(3)
	if m.offsets[1] != 3 {
		t.Errorf("offsets[1] = %d, want 3", m.offsets[1])
	}
	if m.offsets[0] != 0 {
		t.Errorf("offsets[0] = %d, want 0 (not focused)", m.offsets[0])
	}
}

func TestScrollFocusedToTopAndBottom(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	m.offsets[0] = 50
	m.scrollFocusedToTop()
	if m.offsets[0] != 0 {
		t.Errorf("after scrollFocusedToTop, offsets[0] = %d, want 0", m.offsets[0])
	}
	m.scrollFocusedToBottom()
	if m.offsets[0] != 95 {
		t.Errorf("after scrollFocusedToBottom, offsets[0] = %d, want 95", m.offsets[0])
	}
}

func TestHalfPageStepIsHalfOfVisible(t *testing.T) {
	m := makeModel(0, 10, 10, 100) // visible = 6
	if got := m.halfPageStep(); got != 3 {
		t.Errorf("halfPageStep = %d, want 3", got)
	}
}

func TestHalfPageStepFloorsAtOne(t *testing.T) {
	m := makeModel(0, 5, 5, 100) // visible = 1, step = 0 → floored to 1
	if got := m.halfPageStep(); got != 1 {
		t.Errorf("halfPageStep = %d, want 1", got)
	}
}

func TestMoveFocusGoesDown(t *testing.T) {
	widgets := dashboardWidgets()
	if got := moveFocus(widgets, 0, 1, 0); got != 1 {
		t.Errorf("moveFocus down from 0 = %d, want 1", got)
	}
}

func TestMoveFocusClampsAtEdge(t *testing.T) {
	widgets := dashboardWidgets()
	if got := moveFocus(widgets, 0, -1, 0); got != 0 {
		t.Errorf("moveFocus up from top = %d, want 0 (no movement)", got)
	}
	if got := moveFocus(widgets, 1, 1, 0); got != 1 {
		t.Errorf("moveFocus down from bottom = %d, want 1 (no movement)", got)
	}
}

func TestComputeRowHeightsEvenSplit(t *testing.T) {
	got := computeRowHeights(20, 2)
	want := []int{10, 10}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("computeRowHeights(20, 2) = %v, want %v", got, want)
	}
}

func TestComputeRowHeightsRemainderToBottom(t *testing.T) {
	// 23/2 = 11 base, rem 1. Remainder goes to the last row so the
	// layout matches the previous "topH = body/2, botH = body - topH"
	// behaviour where the bottom pane absorbed the extra row.
	got := computeRowHeights(23, 2)
	want := []int{11, 12}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("computeRowHeights(23, 2) = %v, want %v", got, want)
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},
		{-3, 0, 10, 0},
		{15, 0, 10, 10},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, tc := range tests {
		if got := clampInt(tc.v, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}
