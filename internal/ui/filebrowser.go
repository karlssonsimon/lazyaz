package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// DirReader abstracts filesystem access so the file browser can be
// unit-tested without touching real disk.
type DirReader interface {
	ReadDir(path string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
}

// OSDirReader is the production DirReader backed by os.ReadDir / os.Stat.
type OSDirReader struct{}

func (OSDirReader) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (OSDirReader) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }

// FileBrowserAction is the result action from HandleKey.
type FileBrowserAction int

const (
	FBActionNone FileBrowserAction = iota
	FBActionConfirm
	FBActionCancel
)

// FileBrowserResult is the outcome of handling a key press.
type FileBrowserResult struct {
	Action   FileBrowserAction
	Selected []string // populated only when Action == FBActionConfirm
}

// FileBrowserState is the mutable state of the file browser overlay.
// Callers drive it via Open(), HandleKey(), and render with RenderFileBrowser.
type FileBrowserState struct {
	reader  DirReader
	cwd     string
	entries []os.DirEntry
	cursor  int // index into the currently-visible slice (filtered if query set)
	marked  map[string]bool
	visual  bool
	anchor  int // index into the currently-visible slice at time of v

	filterQuery     string
	filterInputOpen bool
}

// Cwd returns the current working directory the browser is showing.
func (s *FileBrowserState) Cwd() string { return s.cwd }

// Entries returns the unfiltered directory listing. Use VisibleEntries
// for what the user is actually seeing.
func (s *FileBrowserState) Entries() []os.DirEntry { return s.entries }

// VisibleEntries returns entries filtered by the current query. When
// no query is set it returns the full listing.
func (s *FileBrowserState) VisibleEntries() []os.DirEntry {
	if s.filterQuery == "" {
		return s.entries
	}
	q := strings.ToLower(s.filterQuery)
	out := make([]os.DirEntry, 0, len(s.entries))
	for _, e := range s.entries {
		if strings.Contains(strings.ToLower(e.Name()), q) {
			out = append(out, e)
		}
	}
	return out
}

// Cursor returns the cursor index into the visible list.
func (s *FileBrowserState) Cursor() int { return s.cursor }

// FilterQuery returns the active filter string.
func (s *FileBrowserState) FilterQuery() string { return s.filterQuery }

// FilterInputOpen reports whether the user is currently typing a filter.
func (s *FileBrowserState) FilterInputOpen() bool { return s.filterInputOpen }

// Marked returns a copy of the marked absolute paths.
func (s *FileBrowserState) Marked() []string {
	paths := make([]string, 0, len(s.marked))
	for p := range s.marked {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// Open initializes the browser at startDir using reader for fs access.
// Marks, cursor, visual state, and filter are all reset.
func (s *FileBrowserState) Open(startDir string, reader DirReader) {
	s.reader = reader
	s.cwd = startDir
	s.cursor = 0
	s.marked = make(map[string]bool)
	s.visual = false
	s.anchor = 0
	s.filterQuery = ""
	s.filterInputOpen = false
	s.loadEntries()
}

func (s *FileBrowserState) loadEntries() {
	if s.reader == nil {
		s.entries = nil
		return
	}
	ents, err := s.reader.ReadDir(s.cwd)
	if err != nil {
		s.entries = nil
		return
	}
	sort.Slice(ents, func(i, j int) bool {
		ai, aj := ents[i], ents[j]
		if ai.IsDir() != aj.IsDir() {
			return ai.IsDir()
		}
		ahi := strings.HasPrefix(ai.Name(), ".")
		ahj := strings.HasPrefix(aj.Name(), ".")
		if ahi != ahj {
			return !ahi
		}
		return strings.ToLower(ai.Name()) < strings.ToLower(aj.Name())
	})
	s.entries = ents
	s.clampCursor()
}

func (s *FileBrowserState) clampCursor() {
	n := len(s.VisibleEntries())
	if n == 0 {
		s.cursor = 0
		return
	}
	if s.cursor >= n {
		s.cursor = n - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}

// HandleKey processes a single key press. Returns the action plus any
// selected paths (only populated on Confirm).
func (s *FileBrowserState) HandleKey(key string) FileBrowserResult {
	if s.filterInputOpen {
		return s.handleFilterInputKey(key)
	}

	switch key {
	case "/":
		s.filterInputOpen = true
		return FileBrowserResult{Action: FBActionNone}
	case "j", "down":
		s.moveCursor(1)
	case "k", "up":
		s.moveCursor(-1)
	case "l", "right":
		visible := s.VisibleEntries()
		if len(visible) == 0 {
			return FileBrowserResult{Action: FBActionNone}
		}
		cur := visible[s.cursor]
		if cur.IsDir() {
			s.cwd = filepath.Join(s.cwd, cur.Name())
			s.cursor = 0
			s.filterQuery = ""
			s.loadEntries()
		}
	case "enter":
		visible := s.VisibleEntries()
		if len(visible) > 0 {
			cur := visible[s.cursor]
			if cur.IsDir() {
				s.cwd = filepath.Join(s.cwd, cur.Name())
				s.cursor = 0
				s.filterQuery = ""
				s.loadEntries()
				return FileBrowserResult{Action: FBActionNone}
			}
		}
		if len(s.marked) == 0 {
			return FileBrowserResult{Action: FBActionNone}
		}
		return FileBrowserResult{Action: FBActionConfirm, Selected: s.Marked()}
	case "esc":
		// Peel filter first (mirrors how the blobs pane handles esc).
		if s.filterQuery != "" {
			s.filterQuery = ""
			s.clampCursor()
			return FileBrowserResult{Action: FBActionNone}
		}
		return FileBrowserResult{Action: FBActionCancel}
	case "h", "left":
		parent := filepath.Dir(s.cwd)
		if parent != s.cwd {
			s.cwd = parent
			s.cursor = 0
			s.filterQuery = ""
			s.loadEntries()
		}
	case " ", "space":
		s.toggleCurrentMark()
	case "v", "V":
		if s.visual {
			visible := s.VisibleEntries()
			lo, hi := s.anchor, s.cursor
			if lo > hi {
				lo, hi = hi, lo
			}
			for i := lo; i <= hi && i < len(visible); i++ {
				s.marked[filepath.Join(s.cwd, visible[i].Name())] = true
			}
			s.visual = false
		} else {
			s.anchor = s.cursor
			s.visual = true
		}
	}
	return FileBrowserResult{Action: FBActionNone}
}

func (s *FileBrowserState) handleFilterInputKey(key string) FileBrowserResult {
	switch key {
	case "enter":
		// Commit: close the input, keep the query applied.
		s.filterInputOpen = false
		s.clampCursor()
		return FileBrowserResult{Action: FBActionNone}
	case "esc":
		// Cancel: clear query and close the input.
		s.filterQuery = ""
		s.filterInputOpen = false
		s.clampCursor()
		return FileBrowserResult{Action: FBActionNone}
	case "backspace":
		if s.filterQuery != "" {
			rs := []rune(s.filterQuery)
			s.filterQuery = string(rs[:len(rs)-1])
			s.cursor = 0
		}
		return FileBrowserResult{Action: FBActionNone}
	case "space":
		s.filterQuery += " "
		s.cursor = 0
		return FileBrowserResult{Action: FBActionNone}
	}
	if isPrintableKey(key) {
		s.filterQuery += key
		s.cursor = 0
	}
	return FileBrowserResult{Action: FBActionNone}
}

func isPrintableKey(key string) bool {
	if key == "" {
		return false
	}
	runes := []rune(key)
	if len(runes) != 1 {
		return false
	}
	return unicode.IsPrint(runes[0])
}

func (s *FileBrowserState) toggleCurrentMark() {
	visible := s.VisibleEntries()
	if len(visible) == 0 {
		return
	}
	path := filepath.Join(s.cwd, visible[s.cursor].Name())
	if s.marked[path] {
		delete(s.marked, path)
	} else {
		s.marked[path] = true
	}
}

func (s *FileBrowserState) moveCursor(delta int) {
	n := len(s.VisibleEntries())
	if n == 0 {
		s.cursor = 0
		return
	}
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= n {
		s.cursor = n - 1
	}
}

// RenderFileBrowser paints the file browser overlay on top of base.
func RenderFileBrowser(state FileBrowserState, styles Styles, width, height int, base string) string {
	visible := state.VisibleEntries()
	items := make([]OverlayItem, len(visible))
	for i, e := range visible {
		prefix := "  "
		if state.marked[filepath.Join(state.cwd, e.Name())] {
			prefix = "● "
		} else if state.visual && insideVisualRange(state.anchor, state.cursor, i) {
			prefix = "· "
		}
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		items[i] = OverlayItem{Label: prefix + e.Name() + suffix}
	}

	title := truncateMiddle(state.cwd, 60)
	hint := "/ filter · esc cancel · space mark · v range · enter confirm"
	if state.filterInputOpen {
		hint = "type to filter · enter apply · esc clear"
	} else if state.filterQuery != "" {
		hint = "esc clear filter · / edit · enter confirm · space mark"
	}

	cfg := OverlayListConfig{
		Title:      fmt.Sprintf("Select files — %s", title),
		Query:      state.filterQuery,
		CloseHint:  hint,
		MaxVisible: 18,
		HideSearch: !state.filterInputOpen && state.filterQuery == "",
		Center:     true,
	}
	return RenderOverlayList(cfg, items, state.cursor, styles.Overlay, width, height, base)
}

func insideVisualRange(anchor, cursor, i int) bool {
	lo, hi := anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return i >= lo && i <= hi
}

// truncateMiddle shortens a path with a "…" in the middle so the ends
// (which carry the most info) stay visible.
func truncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	half := (maxLen - 1) / 2
	return s[:half] + "…" + s[len(s)-(maxLen-1-half):]
}
