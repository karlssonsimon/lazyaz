package dashapp

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// makeModel builds a Model wired with the minimum state the cursor +
// scroll helpers need: registered widgets, row heights, focus index,
// and data backing for whatever widget RowCount reads.
func makeModel(focusedIdx, topH, botH, totalRows int) Model {
	m := Model{
		widgets:    dashboardWidgets(),
		focusedIdx: focusedIdx,
		// rowHeights are Miller column heights including title/footer.
		// Column H gives body height H - 2; the table header and overflow
		// hint live inside that body.
		rowHeights: []int{topH, botH},
	}
	m.offsets = make([]int, len(m.widgets))
	m.cursors = make([]int, len(m.widgets))
	m.viewStates = make([]widgetViewState, len(m.widgets))

	if focusedIdx == 0 {
		m.namespaces = make([]servicebus.Namespace, totalRows)
		for i := range m.namespaces {
			m.namespaces[i] = servicebus.Namespace{Name: "ns"}
		}
		return m
	}
	// Bottom widget (DLQ alerts) — synthesise queue entries with DLQ > 0.
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
	if visible != 7 {
		t.Errorf("visible = %d, want 7", visible)
	}
}

func TestFocusedWidgetDimsBottomWidget(t *testing.T) {
	m := makeModel(1, 10, 12, 5)
	total, visible := m.focusedWidgetDims()
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if visible != 9 {
		t.Errorf("visible = %d, want 9", visible)
	}
}

func TestInnerHeightToVisibleDataMatchesMillerBodySpace(t *testing.T) {
	innerHeight := 8
	visible := innerHeightToVisibleData(innerHeight)
	if visible != 7 {
		t.Fatalf("visible = %d, want 7", visible)
	}

	cells := [][]string{{"name"}}
	for i := 0; i < 20; i++ {
		cells = append(cells, []string{"row"})
	}
	out := renderScrollableTable(cells, []lipgloss.Position{lipgloss.Left}, ui.Styles{}, 0, visible, -1)
	if got := strings.Count(out, "\n") + 1; got != innerHeight {
		t.Fatalf("rendered lines = %d, want %d", got, innerHeight)
	}
}

func TestFocusedWidgetDimsTinyHeightFlooredToOne(t *testing.T) {
	m := makeModel(0, 4, 4, 3)
	_, visible := m.focusedWidgetDims()
	if visible != 1 {
		t.Errorf("visible = %d, want 1 (floor)", visible)
	}
}

func TestMoveCursorClampsAtZero(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	m.moveCursorFocused(-5)
	if m.cursors[0] != 0 {
		t.Errorf("cursors[0] = %d, want 0", m.cursors[0])
	}
}

func TestMoveCursorClampsAtMax(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	m.moveCursorFocused(1000)
	// Cursor max = total - 1 = 99
	if m.cursors[0] != 99 {
		t.Errorf("cursors[0] = %d, want 99", m.cursors[0])
	}
}

func TestMoveCursorRoutesByFocus(t *testing.T) {
	m := makeModel(1, 10, 10, 100)
	m.moveCursorFocused(3)
	if m.cursors[1] != 3 {
		t.Errorf("cursors[1] = %d, want 3", m.cursors[1])
	}
	if m.cursors[0] != 0 {
		t.Errorf("cursors[0] = %d, want 0 (not focused)", m.cursors[0])
	}
}

func TestCursorToTopAndBottom(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	m.cursors[0] = 50
	m.cursorToTop()
	if m.cursors[0] != 0 {
		t.Errorf("cursorToTop: cursors[0] = %d, want 0", m.cursors[0])
	}
	m.cursorToBottom()
	if m.cursors[0] != 99 {
		t.Errorf("cursorToBottom: cursors[0] = %d, want 99", m.cursors[0])
	}
}

func TestScrollFollowsCursorDownward(t *testing.T) {
	// visible = 7; visibleData (hint reserved) = 6.
	// Cursor at 0..5 fits in window [0, 6). Cursor at 6 forces a scroll.
	m := makeModel(0, 10, 10, 100)
	for i := 0; i < 5; i++ {
		m.moveCursorFocused(1)
	}
	if m.cursors[0] != 5 {
		t.Errorf("cursor = %d, want 5", m.cursors[0])
	}
	if m.offsets[0] != 0 {
		t.Errorf("offset at cursor=5 = %d, want 0 (still in window)", m.offsets[0])
	}
	m.moveCursorFocused(1)
	if m.cursors[0] != 6 {
		t.Errorf("cursor = %d, want 6", m.cursors[0])
	}
	if m.offsets[0] != 1 {
		t.Errorf("offset at cursor=6 = %d, want 1 (cursor pushed past window)", m.offsets[0])
	}
}

func TestScrollFollowsCursorUpward(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	m.cursors[0] = 50
	m.scrollToKeepCursorVisible() // brings offset up to 45
	if m.offsets[0] != 45 {
		t.Errorf("after seeking: offset = %d, want 45", m.offsets[0])
	}
	// Now cursor up — should drop offset to keep cursor in view.
	m.moveCursorFocused(-10)
	if m.cursors[0] != 40 {
		t.Errorf("cursor = %d, want 40", m.cursors[0])
	}
	if m.offsets[0] != 40 {
		t.Errorf("offset = %d, want 40 (cursor at top of window)", m.offsets[0])
	}
}

func TestClampCursorsToDataShrinkage(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	m.cursors[0] = 80
	// Data shrinks to 30 rows.
	m.namespaces = m.namespaces[:30]
	m.clampCursorsToData()
	if m.cursors[0] != 29 {
		t.Errorf("cursor = %d, want 29 (clamped to total-1)", m.cursors[0])
	}
}

func TestHalfPageStepIsHalfOfVisible(t *testing.T) {
	m := makeModel(0, 10, 10, 100)
	if got := m.halfPageStep(); got != 3 {
		t.Errorf("halfPageStep = %d, want 3", got)
	}
}

func TestHalfPageStepFloorsAtOne(t *testing.T) {
	m := makeModel(0, 5, 5, 100)
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
		t.Errorf("moveFocus up from top = %d, want 0", got)
	}
	if got := moveFocus(widgets, 1, 1, 0); got != 1 {
		t.Errorf("moveFocus down from bottom = %d, want 1", got)
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
	got := computeRowHeights(23, 2)
	want := []int{11, 12}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("computeRowHeights(23, 2) = %v, want %v", got, want)
	}
}

func TestRecomputeWidgetHeightsMatchesViewTinyHeightMinimum(t *testing.T) {
	m := makeModel(0, 10, 10, 1)
	m.Width = 80
	m.Height = 3
	m.recomputeWidgetHeights()
	rows, _ := gridDims(m.widgets)
	want := computeRowHeights(2, rows)
	if len(m.rowHeights) != len(want) {
		t.Fatalf("rowHeights = %v, want %v", m.rowHeights, want)
	}
	for i := range want {
		if m.rowHeights[i] != want[i] {
			t.Fatalf("rowHeights = %v, want %v", m.rowHeights, want)
		}
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
