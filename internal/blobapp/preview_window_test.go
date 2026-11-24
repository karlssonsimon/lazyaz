package blobapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// fakeBlob is a blob held in memory, read through the same narrow
// interface the preview uses for a real one.
type fakeBlob struct {
	data       []byte
	lineStarts []int
	err        error
}

func newFakeBlob(data []byte) *fakeBlob {
	starts := []int{0}
	for i, b := range data {
		if b == '\n' && i+1 < len(data) {
			starts = append(starts, i+1)
		}
	}
	return &fakeBlob{data: data, lineStarts: starts}
}

func (b *fakeBlob) ReadBlobRange(_ context.Context, offset, count int64) ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	if offset < 0 || offset >= int64(len(b.data)) {
		return nil, nil
	}
	end := offset + count
	if end > int64(len(b.data)) {
		end = int64(len(b.data))
	}
	return b.data[offset:end], nil
}

func (b *fakeBlob) Properties(context.Context) (blob.BlobProperties, error) {
	if b.err != nil {
		return blob.BlobProperties{}, b.err
	}
	return blob.BlobProperties{Size: int64(len(b.data)), ContentType: "text/plain"}, nil
}

// lineOf is the index of the line holding a blob offset.
func (b *fakeBlob) lineOf(offset int64) int {
	idx := sort.Search(len(b.lineStarts), func(i int) bool {
		return int64(b.lineStarts[i]) > offset
	})
	return idx - 1
}

// numberedLines builds a blob of n lines, each starting with its own
// index ("L000042 ") and padded to a length between minLen and maxLen,
// so any line on screen says which line of the blob it is.
func numberedLines(n, minLen, maxLen int) []byte {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		length := minLen
		if maxLen > minLen {
			length += (i * 37) % (maxLen - minLen)
		}
		head := fmt.Sprintf("L%06d ", i)
		sb.WriteString(head)
		for j := len(head); j < length; j++ {
			sb.WriteByte(byte('a' + (i+j)%26))
		}
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

// previewDriver runs the preview against a fakeBlob through the real
// Update loop. Commands are queued instead of run, so a test decides
// when a load lands: settle runs everything outstanding, and anything
// not settled is a request still in flight.
type previewDriver struct {
	t       *testing.T
	m       Model
	blob    *fakeBlob
	pending []tea.Cmd
	// windows and counts tally the window loads and line counts that
	// have landed.
	windows int
	counts  int
}

func openTestPreview(t *testing.T, name string, data []byte) *previewDriver {
	t.Helper()

	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 160, 50
	m.hasAccount = true
	m.currentAccount.Name = "acct"
	m.hasContainer = true
	m.containerName = "cont"
	m.focus = blobsPane
	m.resize()

	d := &previewDriver{t: t, blob: newFakeBlob(data)}
	entry := blob.BlobEntry{Name: name, Size: int64(len(data)), ContentType: "text/plain"}
	opened, cmd := m.openPreviewFrom(entry, d.blob)
	d.m = opened
	d.queue(cmd)
	d.settle()
	return d
}

func (d *previewDriver) queue(cmd tea.Cmd) {
	if cmd != nil {
		d.pending = append(d.pending, cmd)
	}
}

func (d *previewDriver) send(msg tea.Msg) {
	updated, cmd := d.m.Update(msg)
	d.m = updated.(Model)
	d.queue(cmd)
}

// settle runs queued commands, and the ones they lead to, until nothing
// is outstanding.
func (d *previewDriver) settle() {
	d.t.Helper()
	for steps := 0; len(d.pending) > 0; steps++ {
		if steps > 200 {
			d.t.Fatal("commands never settle: the preview keeps issuing requests")
		}
		cmd := d.pending[0]
		d.pending = d.pending[1:]

		switch msg := cmd().(type) {
		case nil:
		case tea.BatchMsg:
			for _, c := range msg {
				d.queue(c)
			}
		case spinner.TickMsg:
			// The spinner reschedules itself on a timer; it has no part
			// in what is being tested.
		case previewWindowLoadedMsg:
			d.windows++
			d.send(msg)
		case previewLinesCountedMsg:
			d.counts++
			d.send(msg)
		default:
			d.send(msg)
		}
	}
}

func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(rest[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

// key sends keypresses without running the commands they return.
func (d *previewDriver) key(keys ...string) {
	for _, k := range keys {
		d.send(keyPress(k))
	}
}

// press sends keypresses, letting everything each one starts finish.
func (d *previewDriver) press(keys ...string) {
	d.t.Helper()
	for _, k := range keys {
		d.send(keyPress(k))
		d.settle()
	}
}

// topLine is the blob line shown on the first row of the view.
func (d *previewDriver) topLine() int {
	p := d.m.preview
	return d.blob.lineOf(p.windowStart + int64(p.lineStarts[p.viewport.YOffset()]))
}

// bottomLine is the blob line shown on the last row of the view that
// has one.
func (d *previewDriver) bottomLine() int {
	p := d.m.preview
	last := min(p.viewport.YOffset()+p.viewport.Height(), len(p.lineStarts)) - 1
	return d.blob.lineOf(p.windowStart + int64(p.lineStarts[last]))
}

func (d *previewDriver) cursorLine() int {
	return d.blob.lineOf(d.m.preview.cursor)
}

// gutterTopNumber is the line number the gutter shows on the first row,
// 0 when the gutter is unnumbered.
func (d *previewDriver) gutterTopNumber() int {
	first := d.m.previewFirstLineNumber()
	if first == 0 {
		return 0
	}
	return first + d.m.preview.viewport.YOffset()
}

// screen is the rendered frame as plain text.
func (d *previewDriver) screen() string {
	return ansi.Strip(d.m.View().Content)
}

// checkGutter fails unless the gutter numbers the top row with the line
// the blob really has there.
func (d *previewDriver) checkGutter(when string) {
	d.t.Helper()
	if got, want := d.gutterTopNumber(), d.topLine()+1; got != want {
		d.t.Errorf("%s: gutter numbers the top row %d, the blob has line %d there", when, got, want)
	}
}

// A blob far larger than the window: 100k lines, about 17 MB.
func largeLog() []byte {
	return numberedLines(100_000, 120, 220)
}

// Scrolling line by line must move the view exactly one line per key,
// across window slides, and must not fetch a window per keypress.
func TestPreviewScrollMovesOneLinePerKey(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())
	d.windows = 0

	const presses = 3000
	const maxWindows = presses / 100
	top := d.topLine()
	for i := 0; i < presses; i++ {
		d.press("j")
		if got := d.topLine(); got != top+1 {
			t.Fatalf("press %d: top line went from %d to %d", i, top, got)
		}
		top++
		if d.windows > maxWindows {
			t.Fatalf("%d window loads after %d keypresses", d.windows, i+1)
		}
	}
	if d.windows == 0 {
		t.Fatal("the scroll never left the first window; the test covers no slide")
	}
	d.checkGutter("after scrolling down")

	for i := 0; i < presses; i++ {
		d.press("k")
		if got := d.topLine(); got != top-1 {
			t.Fatalf("press %d up: top line went from %d to %d", i, top, got)
		}
		top--
	}
	if top != 0 {
		t.Errorf("scrolling back up ended on line %d, want 0", top)
	}
	d.checkGutter("after scrolling back up")
}

// The same in the vim capture: the cursor moves one line per key and the
// view never jumps.
func TestPreviewVimCursorMovesOneLinePerKey(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())
	d.press("v")
	d.windows = 0

	const presses = 3000
	const maxWindows = presses / 100
	cursor, top := d.cursorLine(), d.topLine()
	for i := 0; i < presses; i++ {
		d.press("j")
		if got := d.cursorLine(); got != cursor+1 {
			t.Fatalf("press %d: cursor went from line %d to %d", i, cursor, got)
		}
		cursor++
		if got := d.topLine(); got != top && got != top+1 {
			t.Fatalf("press %d: view jumped from line %d to %d", i, top, got)
		}
		top = d.topLine()
		if d.windows > maxWindows {
			t.Fatalf("%d window loads after %d keypresses", d.windows, i+1)
		}
	}
	if d.windows == 0 {
		t.Fatal("the cursor never left the first window; the test covers no slide")
	}
	d.checkGutter("after moving down")
}

// While a window is on its way the view keeps scrolling through what is
// loaded, further keys do not restart the request, and the swap leaves
// the view on the line it was on.
func TestPreviewScrollsWhileWindowIsInFlight(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())

	for presses := 0; len(d.pending) == 0; presses++ {
		if presses > 5000 {
			t.Fatal("scrolling never asked for a new window")
		}
		d.key("ctrl+d")
	}
	request := d.m.preview.requestID

	top := d.topLine()
	for i := 0; i < 200; i++ {
		d.key("j")
		if got := d.topLine(); got != top+1 {
			t.Fatalf("press %d with a window in flight: top line went from %d to %d", i, top, got)
		}
		top++
	}
	if d.m.preview.requestID != request {
		t.Errorf("keypresses restarted the window request %d times", d.m.preview.requestID-request)
	}

	d.settle()
	if got := d.topLine(); got != top {
		t.Errorf("the window swap moved the view from line %d to %d", top, got)
	}
	d.checkGutter("after the swap")
	d.press("j")
	if got := d.topLine(); got != top+1 {
		t.Errorf("after the swap j moved the view from line %d to %d", top, got)
	}
}

// Scrolling that reaches the end of the loaded window before the next
// one has arrived holds the view there, keeps count of the lines asked
// for, and catches up when the window lands.
func TestPreviewScrollOutrunsWindowLoad(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())

	rowsBelowView := func() int {
		p := d.m.preview
		return len(p.lineStarts) - p.viewport.YOffset() - p.viewport.Height()
	}
	for rowsBelowView() > d.m.preview.viewport.Height() {
		d.key("ctrl+d")
	}
	for rowsBelowView() > 0 {
		d.key("j")
	}
	if len(d.pending) == 0 {
		t.Fatal("no window was requested on the way to the end of the first one")
	}

	top, position := d.topLine(), d.cursorLine()
	runAhead := func(presses int) {
		t.Helper()
		for i := 0; i < presses; i++ {
			d.key("j")
			if got := d.topLine(); got != top {
				t.Fatalf("press %d at the end of the window: the view moved from line %d to %d", i, top, got)
			}
			if got := d.cursorLine(); got != position+1 {
				t.Fatalf("press %d at the end of the window: the position went from line %d to %d", i, position, got)
			}
			position++
		}
	}
	runAhead(15)

	// Going back up goes by what is on screen, not by the position that
	// ran ahead of it.
	d.key("k")
	if got := d.topLine(); got != top-1 {
		t.Fatalf("k at the end of the window moved the view from line %d to %d", top, got)
	}
	d.key("j")
	if got := d.topLine(); got != top {
		t.Fatalf("j moved the view from line %d to %d", top-1, got)
	}
	position = top
	runAhead(15)

	d.settle()
	if got := d.topLine(); got != position {
		t.Errorf("with the window landed the view is on line %d, the position on line %d", got, position)
	}
	d.checkGutter("after catching up")
}

// G lands on the last line of the blob and shows it, numbered as the
// line it is; gg comes back to the first.
func TestPreviewJumpToBottomAndTop(t *testing.T) {
	for _, vimMode := range []bool{false, true} {
		name := "browse"
		if vimMode {
			name = "vim"
		}
		t.Run(name, func(t *testing.T) {
			d := openTestPreview(t, "app.log", largeLog())
			if vimMode {
				d.press("v")
			}

			d.press("G")
			if got := d.cursorLine(); got != 99_999 {
				t.Fatalf("G put the cursor on line %d, want 99999", got)
			}
			screen := d.screen()
			if !strings.Contains(screen, "L099999") {
				t.Error("the last line of the blob is not on screen after G")
			}
			if !strings.Contains(screen, "100000 │") {
				t.Error("the gutter does not number the last line 100000")
			}
			d.checkGutter("after G")

			d.press("k", "k", "k")
			d.checkGutter("after G and scrolling up")

			d.press("g", "g")
			if got := d.cursorLine(); got != 0 {
				t.Errorf("gg put the cursor on line %d, want 0", got)
			}
			if got := d.topLine(); got != 0 {
				t.Errorf("gg left the view on line %d, want 0", got)
			}
			d.checkGutter("after gg")
		})
	}
}

// A jump is not undone by keys typed before its window arrives.
func TestPreviewJumpSurvivesKeysTypedWhileLoading(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())

	d.key("G", "k", "j", "ctrl+d")
	d.settle()
	if got := d.cursorLine(); got != 99_999 {
		t.Errorf("cursor is on line %d after G, want 99999", got)
	}
	if !strings.Contains(d.screen(), "L099999") {
		t.Error("the last line of the blob is not on screen")
	}
}

// Until the lines before a far window have been counted, the gutter has
// no numbers rather than wrong ones.
func TestPreviewGutterIsBlankUntilLinesAreCounted(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())

	d.key("G")
	// Land the window, but not the line count it starts.
	for d.windows < 2 {
		if len(d.pending) == 0 {
			t.Fatal("G did not load a window")
		}
		cmd := d.pending[0]
		d.pending = d.pending[1:]
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				d.queue(c)
			}
		case previewWindowLoadedMsg:
			d.windows++
			d.send(msg)
		}
	}

	if got := d.gutterTopNumber(); got != 0 {
		t.Errorf("gutter shows line %d before the lines were counted", got)
	}
	if strings.Contains(d.screen(), " 1 │") {
		t.Error("the tail of the blob is numbered from 1")
	}

	d.settle()
	d.checkGutter("after the count")
}

// A blob too large to count within the budget keeps an unnumbered gutter
// and is not read for it.
func TestPreviewLineCountRespectsBudget(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())
	d.m.searchScanBudget = 1 << 20

	d.press("G")
	if d.counts != 0 {
		t.Errorf("%d line counts ran past the budget", d.counts)
	}
	if got := d.gutterTopNumber(); got != 0 {
		t.Errorf("gutter shows line %d without a count", got)
	}
	if got := d.cursorLine(); got != 99_999 {
		t.Errorf("cursor is on line %d after G, want 99999", got)
	}
}

// plantNeedles writes a marker into the given lines of a numbered blob
// and returns the blob.
func plantNeedles(data []byte, lines ...int) []byte {
	b := newFakeBlob(data)
	for _, line := range lines {
		at := b.lineStarts[line] + 30
		copy(data[at:], "NEEDLE_MARK")
	}
	return data
}

// A search hit far from the loaded window is brought on screen with its
// line number, without a second pass over the blob to count lines: the
// scan read every byte up to the hit already.
func TestPreviewSearchShowsHitWithLineNumber(t *testing.T) {
	d := openTestPreview(t, "app.log", plantNeedles(largeLog(), 50_000, 99_990))

	d.key("/")
	for _, r := range "NEEDLE_MARK" {
		d.key(string(r))
	}
	d.press("enter")

	if got := d.cursorLine(); got != 50_000 {
		t.Fatalf("search put the cursor on line %d, want 50000", got)
	}
	screen := d.screen()
	if !strings.Contains(screen, "NEEDLE_MARK") {
		t.Error("the match is not on screen")
	}
	if !strings.Contains(screen, "L050000") {
		t.Error("the start of the matching line is scrolled out of view")
	}
	if !strings.Contains(screen, "50001 │") {
		t.Error("the gutter does not number the matching line 50001")
	}
	d.checkGutter("after the search")
	if d.counts != 0 {
		t.Errorf("%d line counts ran; the scan already knew the line", d.counts)
	}

	d.press("n")
	if got := d.cursorLine(); got != 99_990 {
		t.Fatalf("n put the cursor on line %d, want 99990", got)
	}
	d.checkGutter("after n")

	d.press("N")
	if got := d.cursorLine(); got != 50_000 {
		t.Fatalf("N put the cursor on line %d, want 50000", got)
	}
	d.checkGutter("after N")
	if d.counts != 0 {
		t.Errorf("%d line counts ran; the scans already knew the lines", d.counts)
	}
}

// ^ anchors to a line anywhere in the blob, not to wherever a scan chunk
// happens to begin.
func TestPreviewSearchAnchorsToLineStart(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())

	d.key("/")
	for _, r := range "^L070000" {
		d.key(string(r))
	}
	d.press("enter")

	if got := d.cursorLine(); got != 70_000 {
		t.Fatalf("search put the cursor on line %d, want 70000", got)
	}
	d.checkGutter("after the search")
}

// The search bar takes the pane's footer row, so the viewport has to
// give that row up for as long as the bar is showing.
func TestPreviewSearchBarTakesRowFromViewport(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())
	full := d.m.preview.viewport.Height()

	d.press("/")
	if got := d.m.preview.viewport.Height(); got != full-1 {
		t.Errorf("viewport is %d rows with the prompt open, want %d", got, full-1)
	}
	d.press("esc")
	if got := d.m.preview.viewport.Height(); got != full {
		t.Errorf("viewport is %d rows after closing the prompt, want %d", got, full)
	}

	d.press("/", "L", "0", "0", "0", "0", "6", "0", "enter")
	if got := d.m.preview.viewport.Height(); got != full-1 {
		t.Errorf("viewport is %d rows with a search active, want %d", got, full-1)
	}
	if !strings.Contains(d.screen(), "L000060") {
		t.Error("a match just below the first screen is not on screen")
	}
	lines := strings.Split(d.screen(), "\n")
	if len(lines) > d.m.Height {
		t.Errorf("the frame is %d rows tall in a %d row terminal", len(lines), d.m.Height)
	}

	d.press("esc")
	if got := d.m.preview.viewport.Height(); got != full {
		t.Errorf("viewport is %d rows after clearing the search, want %d", got, full)
	}
}

// A match far along a long line is shown whole, not as the few columns
// that fit before the pane's right edge.
func TestPreviewSearchRevealsWholeMatch(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "L%06d %s\n", i, strings.Repeat("x", 40))
	}
	fmt.Fprintf(&sb, "L000200 %sTHE_LONG_NEEDLE_IN_THE_LINE tail\n", strings.Repeat("y", 300))
	d := openTestPreview(t, "app.log", []byte(sb.String()))

	d.key("/")
	for _, r := range "THE_LONG_NEEDLE_IN_THE_LINE" {
		d.key(string(r))
	}
	d.press("enter")

	if !strings.Contains(d.screen(), "THE_LONG_NEEDLE_IN_THE_LINE") {
		t.Error("the match is cut off by the edge of the pane")
	}
}

// Lines of a few kilobytes — JSON logs — leave only a few screens in a
// window of the starting size. Scrolling must still move a line per key,
// and the window must grow so it is not refetched every few lines.
func TestPreviewScrollsLongLines(t *testing.T) {
	d := openTestPreview(t, "events.log", numberedLines(8000, 5000, 5200))
	d.windows = 0

	const presses = 1000
	const maxWindows = presses / 20
	top := d.topLine()
	for i := 0; i < presses; i++ {
		d.press("j")
		if got := d.topLine(); got != top+1 {
			t.Fatalf("press %d: top line went from %d to %d", i, top, got)
		}
		top++
		if d.windows > maxWindows {
			t.Fatalf("%d window loads after %d keypresses", d.windows, i+1)
		}
	}
	d.checkGutter("after scrolling down")
}

// Lines so long that a window cannot hold a screen of them: there is no
// smooth scrolling to be had, but every line has to stay reachable.
func TestPreviewScrollsPastHugeLines(t *testing.T) {
	const lines = 90
	d := openTestPreview(t, "wide.log", numberedLines(lines, 60_000, 60_100))

	d.windows = 0
	presses := 0
	for ; d.bottomLine() < lines-1; presses++ {
		if presses > lines*4 {
			t.Fatalf("stuck with line %d of %d at the bottom of the view after %d keypresses", d.bottomLine(), lines, presses)
		}
		d.press("j")
	}
	// Even here a window is not fetched for every key: the cursor has to
	// have covered a share of the window first.
	if d.windows > presses/2 {
		t.Errorf("%d window loads for %d keypresses", d.windows, presses)
	}

	for presses := 0; d.topLine() > 0; presses++ {
		if presses > lines*4 {
			t.Fatalf("stuck with line %d at the top of the view after %d keypresses up", d.topLine(), presses)
		}
		d.press("k")
	}
}

// A blob that is a single line longer than any window can still be
// walked along in the vim capture.
func TestPreviewWalksSingleHugeLine(t *testing.T) {
	data := []byte(strings.Repeat("abcdefghij ", 280_000))
	d := openTestPreview(t, "one-line.txt", data)
	d.press("v")

	if got := len(d.m.preview.windowData); got > previewLongLineBytes {
		t.Errorf("%d bytes of a single line are loaded, the cap is %d", got, previewLongLineBytes)
	}

	last := int64(-1)
	for presses := 0; d.m.preview.cursor < int64(len(data))-1; presses++ {
		if presses > 200 {
			t.Fatalf("stuck at byte %d of %d", d.m.preview.cursor, len(data))
		}
		if d.m.preview.cursor == last {
			t.Fatalf("$ no longer moves the cursor from byte %d of %d", last, len(data))
		}
		last = d.m.preview.cursor
		d.press("$")
	}
}

// longLineBlob is short lines around two lines that cannot be loaded
// whole: one longer than a window reaches past the cursor, one longer
// than any window.
func longLineBlob() []byte {
	var sb strings.Builder
	line := 0
	short := func(count int) {
		for i := 0; i < count; i++ {
			fmt.Fprintf(&sb, "L%06d a short line of text\n", line)
			line++
		}
	}
	long := func(size int) {
		fmt.Fprintf(&sb, "L%06d ", line)
		sb.WriteString(strings.Repeat("abcdefghij ", size/11))
		sb.WriteString("\n")
		line++
	}
	short(150)
	long(400_000)
	short(150)
	long(2_200_000)
	short(150)
	return []byte(sb.String())
}

// A line too long to load whole is crossed a piece at a time: j and k
// keep moving the position, in both modes, all the way down the blob
// and back up.
func TestPreviewScrollsPastLineTooLongToLoad(t *testing.T) {
	data := longLineBlob()
	const maxPresses = 1500

	for _, mode := range []string{"browse", "vim"} {
		t.Run(mode, func(t *testing.T) {
			d := openTestPreview(t, "mixed.log", data)
			lastLine := d.blob.lineOf(int64(len(data)) - 1)
			atBottom := func() bool {
				if mode == "vim" {
					return d.cursorLine() == lastLine
				}
				return d.bottomLine() == lastLine
			}
			if mode == "vim" {
				d.press("v")
			}

			for presses := 0; !atBottom(); presses++ {
				if presses > maxPresses {
					t.Fatalf("not at the bottom after %d keypresses, at line %d of %d", presses, d.cursorLine(), lastLine)
				}
				before := d.m.preview.cursor
				d.press("j")
				if d.m.preview.cursor <= before {
					t.Fatalf("j left the position at byte %d, line %d of %d", before, d.cursorLine(), lastLine)
				}
			}

			for presses := 0; d.cursorLine() > 0; presses++ {
				if presses > maxPresses {
					t.Fatalf("not at the top after %d keypresses, at line %d", presses, d.cursorLine())
				}
				before := d.m.preview.cursor
				d.press("k")
				if d.m.preview.cursor >= before {
					t.Fatalf("k left the position at byte %d, line %d", before, d.cursorLine())
				}
			}
		})
	}
}

// G on a blob whose last line is too long to load whole shows the piece
// the blob ends with.
func TestPreviewJumpToBottomOfLongLastLine(t *testing.T) {
	data := append(numberedLines(200, 40, 80), strings.Repeat("abcdefghij ", 100_000)...)
	d := openTestPreview(t, "tail.log", data)
	d.press("G")

	p := d.m.preview
	if end := p.windowStart + int64(len(p.windowData)); end != int64(len(data)) {
		t.Errorf("the window ends at byte %d, the blob at %d", end, len(data))
	}
	if got, want := d.cursorLine(), 200; got != want {
		t.Errorf("cursor on line %d, want the last line %d", got, want)
	}
	d.checkGutter("after G")
}

// A blob small enough to fit in one window is loaded once and numbered
// from 1, with no line after the final newline.
func TestPreviewSmallBlobLoadsOnce(t *testing.T) {
	d := openTestPreview(t, "small.log", numberedLines(500, 40, 80))

	d.press("G", "g", "g", "ctrl+d", "ctrl+d", "ctrl+u", "G")
	if d.windows != 1 {
		t.Errorf("%d window loads for a blob that fits in one", d.windows)
	}
	if got := d.cursorLine(); got != 499 {
		t.Errorf("G put the cursor on line %d, want 499", got)
	}
	if got := d.m.preview.viewport.TotalLineCount(); got != 500 {
		t.Errorf("the view holds %d lines for a 500 line blob", got)
	}
	d.checkGutter("at the bottom")
}

func TestPreviewEmptyBlob(t *testing.T) {
	d := openTestPreview(t, "empty.log", nil)

	if !strings.Contains(d.screen(), "(empty blob)") {
		t.Error("an empty blob is not reported as empty")
	}
	d.press("j", "G", "g", "g", "v", "j")
	if d.windows != 1 {
		t.Errorf("%d window loads for an empty blob", d.windows)
	}
}

// UTF-16 text is decoded and shown, and searched as the text it is.
func TestPreviewDecodesUTF16(t *testing.T) {
	text := "first line\nsecond line with a needle\nthird line\n"
	data := []byte{0xFF, 0xFE}
	for _, r := range text {
		data = append(data, byte(r), 0)
	}
	d := openTestPreview(t, "export.txt", data)

	if d.m.preview.binary {
		t.Fatal("UTF-16 text was taken for binary")
	}
	if !strings.Contains(d.screen(), "second line with a needle") {
		t.Error("the decoded text is not on screen")
	}

	d.press("/", "n", "e", "e", "d", "l", "e", "enter")
	if got, want := d.m.preview.cursor, int64(strings.Index(text, "needle")); got != want {
		t.Errorf("search put the cursor at %d of the decoded text, want %d", got, want)
	}
}

// Text with terminal color codes or a stray NUL is still text.
func TestPreviewShowsTextWithControlBytes(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "\x1b[32mINFO\x1b[0m \x1b[36mrequest %d\x1b[0m done\n", i)
	}
	sb.WriteString("a line with a stray \x00 in it\n")
	d := openTestPreview(t, "console.log", []byte(sb.String()))

	if d.m.preview.binary {
		t.Fatal("a colored log was taken for binary")
	}
	if !strings.Contains(d.screen(), "request 3") {
		t.Error("the log text is not on screen")
	}
}

func TestPreviewBinaryBlobIsNotRendered(t *testing.T) {
	data := make([]byte, 64*1024)
	for i := range data {
		data[i] = byte(i * 131)
	}
	d := openTestPreview(t, "image.png", data)

	if !d.m.preview.binary {
		t.Fatal("binary data was taken for text")
	}
	if !strings.Contains(d.screen(), "Binary content preview is not supported.") {
		t.Error("the binary notice is not on screen")
	}
}

// A jump whose window fails to load leaves the cursor where the view is,
// so the preview keeps responding.
func TestPreviewFailedJumpKeepsPosition(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())
	d.press("j", "j", "j")

	d.blob.err = errors.New("network down")
	d.press("G")
	if got := d.cursorLine(); got != 3 {
		t.Errorf("a failed G left the cursor on line %d, want 3", got)
	}

	d.blob.err = nil
	d.press("j")
	if got := d.topLine(); got != 4 {
		t.Errorf("after a failed G, j left the view on line %d, want 4", got)
	}
	d.press("G")
	if got := d.cursorLine(); got != 99_999 {
		t.Errorf("G after the failure put the cursor on line %d, want 99999", got)
	}
}

// Opening another blob while the first one's window is still loading
// must not let the stale window land in the new preview.
func TestPreviewIgnoresWindowOfPreviousBlob(t *testing.T) {
	d := openTestPreview(t, "first.log", largeLog())
	d.key("G")
	stale := d.pending
	d.pending = nil

	second := newFakeBlob(numberedLines(100, 40, 80))
	opened, cmd := d.m.openPreviewFrom(blob.BlobEntry{Name: "first.log", Size: int64(len(second.data)), ContentType: "text/plain"}, second)
	d.m = opened
	d.blob = second
	d.queue(cmd)
	d.settle()

	d.pending = stale
	d.settle()
	if got := d.m.preview.blobSize; got != int64(len(second.data)) {
		t.Errorf("the preview holds a blob of %d bytes, want the second one's %d", got, len(second.data))
	}
	if got := d.m.preview.viewport.TotalLineCount(); got != 100 {
		t.Errorf("the view holds %d lines, want the second blob's 100", got)
	}
}

func TestTrimWindowToLines(t *testing.T) {
	blobData := []byte("aaaa\nbbbb\ncccc\ndddd\n")
	size := int64(len(blobData))

	tests := []struct {
		name       string
		start, end int64
		cursor     int64
		wantStart  int64
		want       string
	}{
		{name: "whole blob", start: 0, end: size, cursor: 0, wantStart: 0, want: string(blobData)},
		{name: "both ends cut", start: 2, end: 17, cursor: 7, wantStart: 5, want: "bbbb\ncccc\n"},
		{name: "starts at the blob start", start: 0, end: 12, cursor: 1, wantStart: 0, want: "aaaa\nbbbb\n"},
		{name: "ends at the blob end", start: 7, end: size, cursor: 12, wantStart: 10, want: "cccc\ndddd\n"},
		{name: "cursor on the cut first line", start: 2, end: 17, cursor: 3, wantStart: 2, want: "aa\nbbbb\ncccc\n"},
		{name: "cursor on the cut last line", start: 2, end: 17, cursor: 16, wantStart: 5, want: "bbbb\ncccc\ndd"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, gotStart := trimWindowToLines(blobData[tc.start:tc.end], tc.start, size, tc.cursor)
			if string(got) != tc.want || gotStart != tc.wantStart {
				t.Errorf("trimmed to %q at %d, want %q at %d", got, gotStart, tc.want, tc.wantStart)
			}
		})
	}
}

// A piece of a long line is cut between runes, never through one, and
// holds the cursor.
func TestCutWindowToLongLine(t *testing.T) {
	short := []byte("one\ntwo\nthree\n")
	if got, start := cutWindowToLongLine(short, 100, 105); string(got) != string(short) || start != 100 {
		t.Errorf("a window of short lines was cut to %q at %d", got, start)
	}

	long := []byte("head\n" + strings.Repeat("é", 200_000) + "\ntail\n")
	for _, cursor := range []int64{5, 6, 100_001, 200_000, 400_004} {
		const start = 1000
		got, gotStart := cutWindowToLongLine(long, start, start+cursor)
		if len(got) > previewLongLineBytes {
			t.Errorf("cursor %d: the piece is %d bytes, the cap is %d", cursor, len(got), previewLongLineBytes)
		}
		local := start + cursor - gotStart
		if local < 0 || local >= int64(len(got)) {
			t.Errorf("cursor %d: the piece [%d, %d) does not hold the cursor", cursor, gotStart, gotStart+int64(len(got)))
		}
		if !utf8.Valid(got) {
			t.Errorf("cursor %d: the piece is cut through a rune", cursor)
		}
		if want := long[gotStart-start : gotStart-start+int64(len(got))]; string(got) != string(want) {
			t.Errorf("cursor %d: the piece is not the bytes at its offset", cursor)
		}
	}
}

func TestComputeLineStarts(t *testing.T) {
	tests := []struct {
		data string
		want []int
	}{
		{"", []int{0}},
		{"one", []int{0}},
		{"one\n", []int{0}},
		{"one\ntwo", []int{0, 4}},
		{"one\ntwo\n", []int{0, 4}},
		{"one\n\ntwo\n", []int{0, 4, 5}},
	}
	for _, tc := range tests {
		got := computeLineStarts([]byte(tc.data))
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("computeLineStarts(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

// The gutter and the viewport share the pane's width, whatever the line
// numbers grow to.
func TestPreviewGutterAndViewportFillThePane(t *testing.T) {
	d := openTestPreview(t, "app.log", largeLog())
	paneWidth := d.m.previewGutterWidth() + d.m.preview.viewport.Width()

	d.press("G")
	if got := d.m.previewGutterWidth(); got != ui.LineGutterWidth(100_000, previewGutterMinDigits) {
		t.Errorf("gutter is %d wide at line 100000", got)
	}
	if got := d.m.previewGutterWidth() + d.m.preview.viewport.Width(); got != paneWidth {
		t.Errorf("gutter and viewport are %d wide together, the pane is %d", got, paneWidth)
	}
}
