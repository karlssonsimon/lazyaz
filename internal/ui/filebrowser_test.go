package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeDirEntry implements os.DirEntry for test injection.
type fakeDirEntry struct {
	name  string
	isDir bool
}

func (f fakeDirEntry) Name() string { return f.name }
func (f fakeDirEntry) IsDir() bool  { return f.isDir }
func (f fakeDirEntry) Type() os.FileMode {
	if f.isDir {
		return os.ModeDir
	}
	return 0
}
func (f fakeDirEntry) Info() (os.FileInfo, error) { return fakeFileInfo{f}, nil }

type fakeFileInfo struct{ fakeDirEntry }

func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.Type() }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) Sys() any           { return nil }

// mapDirReader is a test DirReader that returns predefined entries per path.
type mapDirReader struct {
	entries map[string][]os.DirEntry
}

func (r *mapDirReader) ReadDir(path string) ([]os.DirEntry, error) {
	if ents, ok := r.entries[path]; ok {
		return ents, nil
	}
	return nil, os.ErrNotExist
}

func (r *mapDirReader) Stat(path string) (os.FileInfo, error) {
	if _, ok := r.entries[path]; ok {
		return fakeFileInfo{fakeDirEntry{name: path, isDir: true}}, nil
	}
	return fakeFileInfo{fakeDirEntry{name: path, isDir: false}}, nil
}

func TestFileBrowserOpenPopulatesEntries(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/start": {
				fakeDirEntry{name: "b-dir", isDir: true},
				fakeDirEntry{name: "a-dir", isDir: true},
				fakeDirEntry{name: "readme.txt", isDir: false},
				fakeDirEntry{name: ".hidden", isDir: false},
			},
		},
	}
	var s FileBrowserState
	s.Open("/start", reader)

	if s.Cwd() != "/start" {
		t.Fatalf("want cwd /start, got %q", s.Cwd())
	}
	if len(s.Entries()) != 4 {
		t.Fatalf("want 4 entries, got %d", len(s.Entries()))
	}
	// Sort order: dirs first (alpha), then files (alpha), hidden last.
	want := []string{"a-dir", "b-dir", "readme.txt", ".hidden"}
	for i, e := range s.Entries() {
		if e.Name() != want[i] {
			t.Fatalf("entry %d: want %q, got %q", i, want[i], e.Name())
		}
	}
}

func TestFileBrowserCursorJAndK(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {
				fakeDirEntry{name: "a", isDir: false},
				fakeDirEntry{name: "b", isDir: false},
				fakeDirEntry{name: "c", isDir: false},
			},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	if s.Cursor() != 0 {
		t.Fatalf("initial cursor: want 0, got %d", s.Cursor())
	}
	s.HandleKey("j")
	if s.Cursor() != 1 {
		t.Fatalf("after j: want 1, got %d", s.Cursor())
	}
	s.HandleKey("j")
	s.HandleKey("j") // should clamp at last index (2)
	if s.Cursor() != 2 {
		t.Fatalf("after 3×j: want 2, got %d", s.Cursor())
	}
	s.HandleKey("k")
	if s.Cursor() != 1 {
		t.Fatalf("after k: want 1, got %d", s.Cursor())
	}
}

func TestFileBrowserEnterDirectoryWithL(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/home":                        {fakeDirEntry{name: "docs", isDir: true}},
			filepath.Join("/home", "docs"): {fakeDirEntry{name: "readme.txt", isDir: false}},
		},
	}
	var s FileBrowserState
	s.Open("/home", reader)
	s.HandleKey("l")
	want := filepath.Join("/home", "docs")
	if s.Cwd() != want {
		t.Fatalf("after l on dir: want cwd %q, got %q", want, s.Cwd())
	}
	if len(s.Entries()) != 1 || s.Entries()[0].Name() != "readme.txt" {
		t.Fatalf("after l on dir: want readme.txt in entries")
	}
}

func TestFileBrowserGoUpWithH(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/":     {fakeDirEntry{name: "home", isDir: true}},
			"/home": {fakeDirEntry{name: "readme.txt", isDir: false}},
		},
	}
	var s FileBrowserState
	s.Open("/home", reader)
	s.HandleKey("h")
	if s.Cwd() != "/" {
		t.Fatalf("after h: want cwd /, got %q", s.Cwd())
	}
}

func TestFileBrowserSpaceTogglesMark(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {
				fakeDirEntry{name: "a", isDir: false},
				fakeDirEntry{name: "b", isDir: false},
			},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	s.HandleKey(" ")
	marks := s.Marked()
	if len(marks) != 1 || filepath.Base(marks[0]) != "a" {
		t.Fatalf("after space on a: want [a], got %v", marks)
	}
	s.HandleKey(" ")
	if len(s.Marked()) != 0 {
		t.Fatalf("second space: want unmarked, got %v", s.Marked())
	}
}

func TestFileBrowserVisualLineMarksRange(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {
				fakeDirEntry{name: "a", isDir: false},
				fakeDirEntry{name: "b", isDir: false},
				fakeDirEntry{name: "c", isDir: false},
				fakeDirEntry{name: "d", isDir: false},
			},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	// Anchor at index 1 (b), move down to index 3 (d), commit.
	s.HandleKey("j") // cursor = 1 (b)
	s.HandleKey("v")
	s.HandleKey("j") // cursor = 2
	s.HandleKey("j") // cursor = 3
	s.HandleKey("v") // commit
	marks := s.Marked()
	if len(marks) != 3 {
		t.Fatalf("want 3 marks (b,c,d), got %d: %v", len(marks), marks)
	}
	want := []string{"b", "c", "d"}
	for i, m := range marks {
		if filepath.Base(m) != want[i] {
			t.Fatalf("mark %d: want %q, got %q", i, want[i], filepath.Base(m))
		}
	}
}

func TestFileBrowserConfirmWithMarksReturnsSelected(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {
				fakeDirEntry{name: "a", isDir: false},
				fakeDirEntry{name: "b", isDir: false},
			},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	s.HandleKey(" ") // mark a
	res := s.HandleKey("enter")
	if res.Action != FBActionConfirm {
		t.Fatalf("want Confirm, got %v", res.Action)
	}
	if len(res.Selected) != 1 || filepath.Base(res.Selected[0]) != "a" {
		t.Fatalf("want [a], got %v", res.Selected)
	}
}

func TestFileBrowserConfirmWithNoMarksNoOp(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {fakeDirEntry{name: "a", isDir: false}},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	res := s.HandleKey("enter")
	if res.Action != FBActionNone {
		t.Fatalf("want None (file, no marks → no confirm), got %v", res.Action)
	}
}

func TestFileBrowserEscCancels(t *testing.T) {
	var s FileBrowserState
	s.Open("/", &mapDirReader{entries: map[string][]os.DirEntry{"/": nil}})
	res := s.HandleKey("esc")
	if res.Action != FBActionCancel {
		t.Fatalf("want Cancel, got %v", res.Action)
	}
}

func TestFileBrowserSlashOpensFilterAndTypingNarrowsList(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {
				fakeDirEntry{name: "alpha.txt", isDir: false},
				fakeDirEntry{name: "beta.txt", isDir: false},
				fakeDirEntry{name: "gamma.log", isDir: false},
			},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	s.HandleKey("/")
	if !s.FilterInputOpen() {
		t.Fatalf("after /: want filter input open")
	}
	s.HandleKey("b")
	s.HandleKey("e")
	s.HandleKey("t")
	if q := s.FilterQuery(); q != "bet" {
		t.Fatalf("want query %q, got %q", "bet", q)
	}
	visible := s.VisibleEntries()
	if len(visible) != 1 || visible[0].Name() != "beta.txt" {
		t.Fatalf("want filtered to [beta.txt], got %v", namesOf(visible))
	}
}

func TestFileBrowserFilterEnterClosesInputKeepsQuery(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {
				fakeDirEntry{name: "alpha.txt", isDir: false},
				fakeDirEntry{name: "beta.txt", isDir: false},
			},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	s.HandleKey("/")
	s.HandleKey("b")
	s.HandleKey("enter")
	if s.FilterInputOpen() {
		t.Fatalf("enter should close filter input")
	}
	if s.FilterQuery() != "b" {
		t.Fatalf("query should persist after enter, got %q", s.FilterQuery())
	}
	if len(s.VisibleEntries()) != 1 {
		t.Fatalf("want filtered view after closing input, got %d", len(s.VisibleEntries()))
	}
}

func TestFileBrowserFilterEscapeClearsAndEscapeAgainCancels(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {fakeDirEntry{name: "alpha", isDir: false}},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	s.HandleKey("/")
	s.HandleKey("a")
	// First esc closes the input AND clears the query (single press).
	res := s.HandleKey("esc")
	if res.Action != FBActionNone {
		t.Fatalf("first esc (in filter input): want None, got %v", res.Action)
	}
	if s.FilterInputOpen() || s.FilterQuery() != "" {
		t.Fatalf("filter should be fully cleared after esc")
	}
	// Next esc cancels the browser.
	res = s.HandleKey("esc")
	if res.Action != FBActionCancel {
		t.Fatalf("second esc: want Cancel, got %v", res.Action)
	}
}

func TestFileBrowserFilterBackspaceRemovesChar(t *testing.T) {
	reader := &mapDirReader{entries: map[string][]os.DirEntry{"/x": {fakeDirEntry{name: "a"}}}}
	var s FileBrowserState
	s.Open("/x", reader)
	s.HandleKey("/")
	s.HandleKey("a")
	s.HandleKey("b")
	s.HandleKey("backspace")
	if s.FilterQuery() != "a" {
		t.Fatalf("after backspace: want 'a', got %q", s.FilterQuery())
	}
}

func TestFileBrowserFilterSurvivesAcrossEnterExitButClearsOnDirChange(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/root": {
				fakeDirEntry{name: "sub", isDir: true},
				fakeDirEntry{name: "other", isDir: false},
			},
			filepath.Join("/root", "sub"): {fakeDirEntry{name: "inner", isDir: false}},
		},
	}
	var s FileBrowserState
	s.Open("/root", reader)
	s.HandleKey("/")
	s.HandleKey("s")
	s.HandleKey("enter") // closes input, filter stays
	if s.FilterQuery() != "s" {
		t.Fatalf("want query 's' after enter, got %q", s.FilterQuery())
	}
	// Move cursor to the only visible (sub) and descend.
	s.HandleKey("l")
	if s.Cwd() != filepath.Join("/root", "sub") {
		t.Fatalf("want cwd %q, got %q", filepath.Join("/root", "sub"), s.Cwd())
	}
	if s.FilterQuery() != "" {
		t.Fatalf("filter should clear on directory change, got %q", s.FilterQuery())
	}
}

func TestFileBrowserMarkOperatesOnFilteredView(t *testing.T) {
	reader := &mapDirReader{
		entries: map[string][]os.DirEntry{
			"/x": {
				fakeDirEntry{name: "alpha.txt"},
				fakeDirEntry{name: "beta.txt"},
				fakeDirEntry{name: "gamma.txt"},
			},
		},
	}
	var s FileBrowserState
	s.Open("/x", reader)
	s.HandleKey("/")
	s.HandleKey("b")
	s.HandleKey("enter") // close input; "beta.txt" is the only visible
	s.HandleKey(" ")     // mark it
	marks := s.Marked()
	if len(marks) != 1 || filepath.Base(marks[0]) != "beta.txt" {
		t.Fatalf("want [beta.txt] marked, got %v", marks)
	}
}

func namesOf(entries []os.DirEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name()
	}
	return out
}
