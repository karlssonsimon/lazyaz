package ui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/vim"
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

// StartDir picks the directory a file browser opens in: remembered when
// it still names a directory, otherwise the process working directory.
func StartDir(remembered string) string {
	if remembered != "" {
		if info, err := os.Stat(remembered); err == nil && info.IsDir() {
			return remembered
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return string(filepath.Separator)
}

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
	filterCaret     int // rune index of the edit caret within filterQuery
	filterInputOpen bool

	// notice replaces the hint line until the next key press; it reports
	// a pasted path that could not be opened.
	notice string

	km     keymap.Keymap
	chords vim.Resolver
}

// Cwd returns the current working directory the browser is showing.
func (s *FileBrowserState) Cwd() string { return s.cwd }

// Entries returns the unfiltered directory listing. Use VisibleEntries
// for what the user is actually seeing.
func (s *FileBrowserState) Entries() []os.DirEntry { return s.entries }

// VisibleEntries returns entries filtered by the current query (fzf
// matching and syntax, best match first). When no query is set it
// returns the full listing.
func (s *FileBrowserState) VisibleEntries() []os.DirEntry {
	if s.filterQuery == "" {
		return s.entries
	}
	idx := fuzzy.Filter(s.filterQuery, s.entries, func(e os.DirEntry) string { return e.Name() })
	out := make([]os.DirEntry, len(idx))
	for i, j := range idx {
		out[i] = s.entries[j]
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
// Marks, cursor, visual state, and filter are all reset. The keymap
// is captured for HandleKey to consult; callers pass m.Keymap so the
// user's keymap config applies.
func (s *FileBrowserState) Open(startDir string, reader DirReader, km keymap.Keymap) {
	s.reader = reader
	s.cwd = startDir
	s.cursor = 0
	s.marked = make(map[string]bool)
	s.visual = false
	s.anchor = 0
	s.filterQuery = ""
	s.filterCaret = 0
	s.filterInputOpen = false
	s.notice = ""
	s.km = km
	s.chords.Clear()
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

// halfPageStep matches the overlay's MaxVisible (18) / 2.
const halfPageStep = 9

// HandleKey processes a single key press. Returns the action plus any
// selected paths (only populated on Confirm). Keys are matched against
// the keymap captured at Open(); unbound keys are silently ignored.
func (s *FileBrowserState) HandleKey(key string) FileBrowserResult {
	s.notice = ""
	if s.filterInputOpen {
		return s.handleFilterInputKey(key)
	}

	km := s.km

	// The gg chord resolves before the switch. Every JumpTopPrefix key
	// arms here — Home included — matching the browser's original
	// primed-flag behavior rather than the viewers' immediate Home.
	if s.chords.GG(km.JumpTopPrefix, key, false) == vim.ChordFired {
		s.cursor = 0
		return FileBrowserResult{Action: FBActionNone}
	}

	switch {
	case km.FilterInput.Matches(key):
		s.filterInputOpen = true
	case km.CursorDown.Matches(key):
		s.moveCursor(1)
	case km.CursorUp.Matches(key):
		s.moveCursor(-1)
	case km.HalfPageDown.Matches(key):
		s.moveCursor(halfPageStep)
	case km.HalfPageUp.Matches(key):
		s.moveCursor(-halfPageStep)
	case km.JumpBottom.Matches(key):
		if n := len(s.VisibleEntries()); n > 0 {
			s.cursor = n - 1
		}
	case km.OpenFocusedAlt.Matches(key):
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
	case km.OpenFocused.Matches(key):
		// Marks win: if anything is marked, Enter submits the marks.
		// Otherwise: a directory under the cursor navigates in (use l/right
		// for explicit nav with marks held); a file under the cursor
		// submits just that file.
		if len(s.marked) > 0 {
			return FileBrowserResult{Action: FBActionConfirm, Selected: s.Marked()}
		}
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
			return FileBrowserResult{Action: FBActionNone}
		}
		return FileBrowserResult{Action: FBActionConfirm, Selected: []string{filepath.Join(s.cwd, cur.Name())}}
	case km.Cancel.Matches(key):
		// Peel filter first (mirrors how the blobs pane handles esc).
		if s.filterQuery != "" {
			s.filterQuery = ""
			s.clampCursor()
			return FileBrowserResult{Action: FBActionNone}
		}
		return FileBrowserResult{Action: FBActionCancel}
	case km.NavigateLeft.Matches(key):
		parent := filepath.Dir(s.cwd)
		if parent != s.cwd {
			s.cwd = parent
			s.cursor = 0
			s.filterQuery = ""
			s.loadEntries()
		}
	case km.ToggleMark.Matches(key):
		s.toggleCurrentMark()
	case km.ToggleVisualLine.Matches(key):
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
	case s.visual && km.VisualSwapAnchor.Matches(key):
		s.anchor, s.cursor = s.cursor, s.anchor
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
		s.filterCaret = 0
		s.filterInputOpen = false
		s.clampCursor()
		return FileBrowserResult{Action: FBActionNone}
	}
	ti := TextInput{Value: s.filterQuery, Cursor: s.filterCaret}
	if ti.HandleKey(key) {
		changed := ti.Value != s.filterQuery
		s.filterQuery = ti.Value
		s.filterCaret = ti.Cursor
		if changed {
			s.cursor = 0
		}
	}
	return FileBrowserResult{Action: FBActionNone}
}

// Paste handles text pasted into the browser. While the filter input is
// open the text is inserted into the filter. Otherwise it is read as a
// path: a directory becomes the current directory, a file moves the
// browser to its directory with the cursor on the file. Marks are kept.
// A path that cannot be opened leaves the browser where it is and puts
// a notice in the hint line.
func (s *FileBrowserState) Paste(text string) {
	s.notice = ""
	if s.filterInputOpen {
		ti := TextInput{Value: s.filterQuery, Cursor: s.filterCaret}
		ti.Insert(strings.TrimSpace(firstLine(text)))
		s.filterQuery = ti.Value
		s.filterCaret = ti.Cursor
		s.cursor = 0
		return
	}
	if s.reader == nil {
		return
	}
	candidates := pastedPathCandidates(text, s.cwd)
	if len(candidates) == 0 {
		return
	}
	for _, p := range candidates {
		info, err := s.reader.Stat(p)
		if err != nil {
			continue
		}
		dir, focus := p, ""
		if !info.IsDir() {
			dir, focus = filepath.Dir(p), filepath.Base(p)
		}
		s.cwd = dir
		s.cursor = 0
		s.visual = false
		s.filterQuery = ""
		s.filterCaret = 0
		s.loadEntries()
		for i, e := range s.entries {
			if e.Name() == focus {
				s.cursor = i
				break
			}
		}
		return
	}
	s.notice = "no such path: " + truncateMiddle(candidates[0], 40)
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// pastedPathCandidates turns pasted text into the absolute paths it may
// name, most literal first. It accepts what terminals and file managers
// put on the clipboard: surrounding quotes, file:// URLs, a leading ~,
// and backslash-escaped characters from drag-and-drop. Relative paths
// resolve against cwd.
func pastedPathCandidates(text, cwd string) []string {
	p := firstLine(text)
	if p == "" {
		return nil
	}
	quoted := false
	if n := len(p); n >= 2 && (p[0] == '\'' || p[0] == '"') && p[n-1] == p[0] {
		p = p[1 : n-1]
		quoted = true
	}
	if strings.HasPrefix(p, "file://") {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			return []string{filepath.Clean(filepath.FromSlash(u.Path))}
		}
	}
	resolve := func(p string) string {
		if p == "~" || strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[1:])
			}
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		return filepath.Clean(p)
	}
	out := []string{resolve(p)}
	// Backslash is the separator on Windows, an escape everywhere else.
	if !quoted && runtime.GOOS != "windows" && strings.Contains(p, "\\") {
		if alt := resolve(unescapeBackslashes(p)); alt != out[0] {
			out = append(out, alt)
		}
	}
	return out
}

func unescapeBackslashes(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' && i+1 < len(p) {
			i++
		}
		b.WriteByte(p[i])
	}
	return b.String()
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
	hint := "/ filter · g/G top/bot · ctrl+d/u half-page · space mark · v range · o swap · enter"
	if state.filterInputOpen {
		hint = "type to filter · enter apply · esc clear"
	} else if state.filterQuery != "" {
		hint = "esc clear filter · / edit · enter confirm · space mark"
	}
	if state.notice != "" {
		hint = state.notice
	}

	cfg := OverlayListConfig{
		Title:       fmt.Sprintf("Select files — %s", title),
		Query:       state.filterQuery,
		QueryCursor: state.filterCaret,
		CloseHint:   hint,
		MaxVisible:  18,
		HideSearch:  !state.filterInputOpen && state.filterQuery == "",
		Center:      true,
	}
	return RenderOverlayList(cfg, items, state.cursor, styles, width, height, base)
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
