package acapp

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
)

func TestEditorCommandResolution(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code --wait")
	argv, err := editorCommand()
	if err != nil || len(argv) != 2 || argv[0] != "code" || argv[1] != "--wait" {
		t.Fatalf("EDITOR with args: argv=%v err=%v", argv, err)
	}
	t.Setenv("VISUAL", "hx")
	argv, _ = editorCommand()
	if len(argv) != 1 || argv[0] != "hx" {
		t.Fatalf("VISUAL should win: %v", argv)
	}
}

// JSON goes to the editor pretty-printed as a .json file; other values
// go as they are.
func TestEditorTextAndFile(t *testing.T) {
	text, isJSON := editorText(appconfig.Setting{Value: `{"a":1,"b":[1,2]}`})
	if !isJSON || !strings.Contains(text, "\n  \"a\": 1,") || !strings.HasSuffix(text, "\n") {
		t.Fatalf("JSON not pretty-printed: %q", text)
	}
	if text, isJSON := editorText(appconfig.Setting{Value: "plain 30"}); isJSON || text != "plain 30" {
		t.Fatalf("plain value altered: %q %v", text, isJSON)
	}

	m := storeModel(t)
	m.EmbeddedMode = true
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")
	m, _ = m.handleEnter()
	m.settingsList.Select(2) // Features/Checkout, JSON
	_, cmd := m.openEditValueInEditor()
	if cmd == nil {
		t.Fatal("no command")
	}
	req, ok := cmd().(ExternalEditRequestMsg)
	if !ok {
		t.Fatalf("embedded edit should ask the parent, got %T", cmd())
	}
	defer os.Remove(req.Path)
	if req.Argv[0] != "true" || req.Argv[len(req.Argv)-1] != req.Path || !strings.HasSuffix(req.Path, ".json") {
		t.Fatalf("request = %+v", req)
	}
	data, err := os.ReadFile(req.Path)
	if err != nil || !strings.Contains(string(data), "\"retries\": 3") {
		t.Fatalf("temp file content: %q err=%v", data, err)
	}
	info, _ := os.Stat(req.Path)
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("temp file is readable by others: %v", info.Mode())
	}
}

// editDone drives a round trip: open in the editor, rewrite the file,
// report the editor as finished.
func editDone(t *testing.T, m Model, newContent string, elapsed time.Duration) Model {
	t.Helper()
	m.EmbeddedMode = true
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")
	m, cmd := m.openEditValueInEditor()
	req := cmd().(ExternalEditRequestMsg)
	if newContent != "" {
		if err := os.WriteFile(req.Path, []byte(newContent), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m.externalEdit.started = time.Now().Add(-elapsed)
	m, _ = m.handleExternalEditDone(ExternalEditDoneMsg{Path: req.Path})
	if _, err := os.Stat(req.Path); !os.IsNotExist(err) {
		t.Error("temp file not removed")
	}
	return m
}

// An edited JSON value reaches the confirmation with a line summary and
// without the editor's trailing newline.
func TestEditorRoundTripConfirms(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(2)
	m = editDone(t, m, "{\n  \"enabled\": false,\n  \"retries\": 3\n}\n", 5*time.Second)
	if !m.confirmModal.Active {
		t.Fatalf("expected the save confirmation, status=%q", m.Status)
	}
	// The stored value is one-line JSON; the diff is against the
	// pretty-printed text the editor showed, so only the edited line
	// is reported.
	msg := m.confirmModal.Message
	if !strings.Contains(msg, "4 → 4 lines, 2 changed") || !strings.Contains(msg, `-   2    "enabled": true,`) || !strings.Contains(msg, `+   2    "enabled": false,`) {
		t.Fatalf("summary = %q", msg)
	}
}

// Invalid JSON is refused; an instant, unchanged return blames a GUI
// editor without a wait flag; a slow unchanged return is just unchanged.
func TestEditorRoundTripRefusals(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(2)

	bad := editDone(t, m, "{not json\n", 5*time.Second)
	if bad.confirmModal.Active || !strings.Contains(bad.Status, "not valid JSON") {
		t.Fatalf("invalid JSON: confirm=%v status=%q", bad.confirmModal.Active, bad.Status)
	}
	fast := editDone(t, m, "", 10*time.Millisecond)
	if fast.confirmModal.Active || !strings.Contains(fast.Status, "wait flag") {
		t.Fatalf("instant return: confirm=%v status=%q", fast.confirmModal.Active, fast.Status)
	}
	slow := editDone(t, m, "", 5*time.Second)
	if slow.confirmModal.Active || !strings.Contains(slow.Status, "unchanged") {
		t.Fatalf("slow unchanged: confirm=%v status=%q", slow.confirmModal.Active, slow.Status)
	}
}

func TestChangeSummary(t *testing.T) {
	if got := changeSummary("60", "90"); got != "old: 60\nnew: 90" {
		t.Fatalf("short summary = %q", got)
	}
	// Edits in two places both appear, not only the first.
	got := changeSummary("a\nb\nc\nd\ne", "a\nB\nc\nd\nE\nf")
	for _, want := range []string{"5 → 6 lines, 5 changed", "-   2  b", "+   2  B", "-   5  e", "+   5  E", "+   6  f"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "a\n") || strings.Contains(got, "  c") {
		t.Errorf("unchanged lines should not be listed:\n%s", got)
	}
}

func TestLineDiffMovesAndCap(t *testing.T) {
	d := lineDiff([]string{"x", "y", "z"}, []string{"y", "z", "w"})
	if len(d) != 2 || d[0].sign != "-" || d[0].text != "x" || d[1].sign != "+" || d[1].text != "w" || d[1].line != 3 {
		t.Fatalf("diff = %+v", d)
	}
	big := make([]string, maxDiffLines+10)
	for i := range big {
		big[i] = fmt.Sprintf("line %d", i)
	}
	// Every old line is removed and the single empty new line added.
	got := changeSummary(strings.Join(big, "\n"), "")
	if !strings.Contains(got, fmt.Sprintf("… and %d more", len(big)+1-maxDiffLines)) {
		t.Fatalf("long diff not capped:\n%s", got)
	}
}

func TestSanitizeForFilename(t *testing.T) {
	if got := sanitizeForFilename("Db/Password:prod ../x"); strings.ContainsAny(got, "/:. ") && !strings.Contains(got, ".") {
		t.Fatalf("unsafe characters survived: %q", got)
	}
	if got := sanitizeForFilename("../../etc/passwd"); strings.Contains(got, "/") {
		t.Fatalf("path separators survived: %q", got)
	}
}
