package ui

import "testing"

func TestTextInputInsertAtCursor(t *testing.T) {
	ti := TextInput{}
	ti.Insert("hello")
	if ti.Value != "hello" || ti.Cursor != 5 {
		t.Fatalf("after Insert(hello): %q cursor=%d, want hello cursor=5", ti.Value, ti.Cursor)
	}
	ti.Cursor = 2
	ti.Insert("XX")
	if ti.Value != "heXXllo" || ti.Cursor != 4 {
		t.Fatalf("mid-cursor Insert: %q cursor=%d, want heXXllo cursor=4", ti.Value, ti.Cursor)
	}
}

func TestTextInputInsertRuneAware(t *testing.T) {
	ti := TextInput{}
	ti.Insert("ä")
	ti.Insert("ö")
	if ti.Value != "äö" || ti.Cursor != 2 {
		t.Fatalf("multi-byte insert: %q cursor=%d, want äö cursor=2", ti.Value, ti.Cursor)
	}
}

func TestTextInputArrowKeys(t *testing.T) {
	ti := TextInput{Value: "abcd", Cursor: 4}
	ti.HandleKey("left")
	if ti.Cursor != 3 {
		t.Fatalf("after left: cursor=%d, want 3", ti.Cursor)
	}
	ti.HandleKey("left")
	ti.HandleKey("left")
	ti.HandleKey("left")
	ti.HandleKey("left") // clamp at 0
	if ti.Cursor != 0 {
		t.Fatalf("clamped left: cursor=%d, want 0", ti.Cursor)
	}
	ti.HandleKey("right")
	if ti.Cursor != 1 {
		t.Fatalf("after right: cursor=%d, want 1", ti.Cursor)
	}
}

func TestTextInputHomeEnd(t *testing.T) {
	ti := TextInput{Value: "abcd", Cursor: 2}
	ti.HandleKey("home")
	if ti.Cursor != 0 {
		t.Fatalf("after home: cursor=%d, want 0", ti.Cursor)
	}
	ti.HandleKey("end")
	if ti.Cursor != 4 {
		t.Fatalf("after end: cursor=%d, want 4", ti.Cursor)
	}
	ti.Cursor = 2
	ti.HandleKey("ctrl+a")
	if ti.Cursor != 0 {
		t.Fatalf("after ctrl+a: cursor=%d, want 0", ti.Cursor)
	}
	ti.HandleKey("ctrl+e")
	if ti.Cursor != 4 {
		t.Fatalf("after ctrl+e: cursor=%d, want 4", ti.Cursor)
	}
}

func TestTextInputBackspace(t *testing.T) {
	ti := TextInput{Value: "abcd", Cursor: 4}
	ti.HandleKey("backspace")
	if ti.Value != "abc" || ti.Cursor != 3 {
		t.Fatalf("backspace at end: %q cursor=%d, want abc cursor=3", ti.Value, ti.Cursor)
	}
	ti.Cursor = 1
	ti.HandleKey("backspace")
	if ti.Value != "bc" || ti.Cursor != 0 {
		t.Fatalf("backspace mid: %q cursor=%d, want bc cursor=0", ti.Value, ti.Cursor)
	}
	ti.HandleKey("backspace") // no-op at start
	if ti.Value != "bc" || ti.Cursor != 0 {
		t.Fatalf("backspace at start: %q cursor=%d, want unchanged", ti.Value, ti.Cursor)
	}
}

func TestTextInputDelete(t *testing.T) {
	ti := TextInput{Value: "abcd", Cursor: 0}
	ti.HandleKey("delete")
	if ti.Value != "bcd" || ti.Cursor != 0 {
		t.Fatalf("delete at start: %q cursor=%d, want bcd cursor=0", ti.Value, ti.Cursor)
	}
	ti.Cursor = 3
	ti.HandleKey("delete") // no-op at end
	if ti.Value != "bcd" || ti.Cursor != 3 {
		t.Fatalf("delete at end: %q cursor=%d, want unchanged", ti.Value, ti.Cursor)
	}
}

func TestTextInputDeleteWord(t *testing.T) {
	ti := TextInput{Value: "hello world foo", Cursor: 15}
	ti.HandleKey("ctrl+w")
	if ti.Value != "hello world " || ti.Cursor != 12 {
		t.Fatalf("ctrl+w end: %q cursor=%d, want 'hello world ' cursor=12", ti.Value, ti.Cursor)
	}
	ti.HandleKey("ctrl+w")
	if ti.Value != "hello " || ti.Cursor != 6 {
		t.Fatalf("ctrl+w again: %q cursor=%d, want 'hello ' cursor=6", ti.Value, ti.Cursor)
	}
	ti.HandleKey("ctrl+w")
	if ti.Value != "" || ti.Cursor != 0 {
		t.Fatalf("ctrl+w last word: %q cursor=%d, want empty cursor=0", ti.Value, ti.Cursor)
	}
}

func TestTextInputSpace(t *testing.T) {
	ti := TextInput{Value: "ab", Cursor: 1}
	ti.HandleKey("space")
	if ti.Value != "a b" || ti.Cursor != 2 {
		t.Fatalf("space at cursor: %q cursor=%d, want 'a b' cursor=2", ti.Value, ti.Cursor)
	}
}

func TestTextInputPrintable(t *testing.T) {
	ti := TextInput{Value: "ab", Cursor: 1}
	ti.HandleKey("X")
	if ti.Value != "aXb" || ti.Cursor != 2 {
		t.Fatalf("printable at cursor: %q cursor=%d, want aXb cursor=2", ti.Value, ti.Cursor)
	}
}

func TestTextInputHandleKeyConsumed(t *testing.T) {
	ti := TextInput{Value: "abc", Cursor: 1}
	cases := map[string]bool{
		"left":      true,
		"right":     true,
		"home":      true,
		"end":       true,
		"ctrl+a":    true,
		"ctrl+e":    true,
		"backspace": true,
		"delete":    true,
		"ctrl+w":    true,
		"space":     true,
		"x":         true,
		"enter":     false,
		"tab":       false,
		"ctrl+r":    false,
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			ti := ti
			got := ti.HandleKey(key)
			if got != want {
				t.Fatalf("HandleKey(%q) = %v, want %v", key, got, want)
			}
		})
	}
}

func TestTextInputReset(t *testing.T) {
	ti := TextInput{Value: "abc", Cursor: 2}
	ti.Reset()
	if ti.Value != "" || ti.Cursor != 0 {
		t.Fatalf("after Reset: %q cursor=%d, want empty cursor=0", ti.Value, ti.Cursor)
	}
}

func TestTextInputSetValue(t *testing.T) {
	ti := TextInput{Value: "abc", Cursor: 2}
	ti.SetValue("xyz")
	if ti.Value != "xyz" || ti.Cursor != 3 {
		t.Fatalf("after SetValue(xyz): %q cursor=%d, want xyz cursor=3", ti.Value, ti.Cursor)
	}
}

func TestTextInputSplitAtCursor(t *testing.T) {
	ti := TextInput{Value: "abcd", Cursor: 2}
	before, after := ti.SplitAtCursor()
	if before != "ab" || after != "cd" {
		t.Fatalf("SplitAtCursor mid: (%q, %q), want (ab, cd)", before, after)
	}
	ti.Cursor = 0
	before, after = ti.SplitAtCursor()
	if before != "" || after != "abcd" {
		t.Fatalf("SplitAtCursor start: (%q, %q), want ('', abcd)", before, after)
	}
	ti.Cursor = 4
	before, after = ti.SplitAtCursor()
	if before != "abcd" || after != "" {
		t.Fatalf("SplitAtCursor end: (%q, %q), want (abcd, '')", before, after)
	}
}

func TestTextInputSplitWithCursor(t *testing.T) {
	cases := []struct {
		name       string
		value      string
		cursor     int
		wantBefore string
		wantAt     string
		wantAfter  string
	}{
		{"mid", "abcd", 2, "ab", "c", "d"},
		{"start", "abcd", 0, "", "a", "bcd"},
		{"end", "abcd", 4, "abcd", " ", ""},
		{"empty", "", 0, "", " ", ""},
		{"overflow", "abc", 99, "abc", " ", ""},
		{"multi-byte mid", "äöü", 1, "ä", "ö", "ü"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ti := TextInput{Value: tc.value, Cursor: tc.cursor}
			b, at, a := ti.SplitWithCursor()
			if b != tc.wantBefore || at != tc.wantAt || a != tc.wantAfter {
				t.Fatalf("SplitWithCursor = (%q, %q, %q), want (%q, %q, %q)",
					b, at, a, tc.wantBefore, tc.wantAt, tc.wantAfter)
			}
		})
	}
}

func TestTextInputClampCursor(t *testing.T) {
	// Cursor past end (e.g. after external Value mutation) should clamp.
	ti := TextInput{Value: "abc", Cursor: 99}
	before, after := ti.SplitAtCursor()
	if before != "abc" || after != "" {
		t.Fatalf("SplitAtCursor with overflow: (%q, %q), want (abc, '')", before, after)
	}
}
