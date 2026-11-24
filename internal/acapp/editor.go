package acapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// Editing in an external editor: the value goes to a private temp
// file (pretty-printed when it is JSON, named .json so the editor
// knows), the TUI suspends while $VISUAL / $EDITOR / vi runs on it,
// and the result comes back through the usual confirmation and
// conditional save. Embedded, the parent runs the process: a tab's
// commands are wrapped for routing, which would hide the framework's
// exec request, so the tab asks with ExternalEditRequestMsg and the
// parent answers with ExternalEditDoneMsg.

// ExternalEditRequestMsg asks the parent to run Argv (editor plus the
// file) with the terminal suspended, then send ExternalEditDoneMsg.
type ExternalEditRequestMsg struct {
	Argv []string
	Path string
}

// ExternalEditDoneMsg reports that the editor exited.
type ExternalEditDoneMsg struct {
	Path string
	Err  error
}

// externalEdit is the state of an edit that is out in an editor.
type externalEdit struct {
	target   appconfig.Setting
	path     string
	original string // what was written to the file
	wantJSON bool
	started  time.Time
	// forCreate means the text is the value of a key-value being
	// created: it goes back into the create form, not to a save.
	forCreate bool
}

// editorCommand resolves the editor: $VISUAL, then $EDITOR, then the
// platform fallback — vi on Unix, Notepad on Windows, which blocks
// until its window closes when started directly. The value may carry
// arguments ("code --wait").
func editorCommand() ([]string, error) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return strings.Fields(v), nil
		}
	}
	fallback := "vi"
	if runtime.GOOS == "windows" {
		fallback = "notepad"
	}
	if _, err := exec.LookPath(fallback); err == nil {
		return []string{fallback}, nil
	}
	return nil, fmt.Errorf("no editor: set EDITOR (for a GUI editor include its wait flag, e.g. \"code --wait\")")
}

// editorText is the text handed to the editor: pretty JSON when the
// value parses as JSON, the raw value otherwise.
func editorText(s appconfig.Setting) (text string, isJSON bool) {
	if json.Valid([]byte(s.Value)) && strings.TrimSpace(s.Value) != "" {
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(s.Value), "", "  "); err == nil {
			return buf.String() + "\n", true
		}
	}
	return s.Value, false
}

// openEditValueInEditor writes the temp file and hands off to the
// editor. Locked settings are refused up front.
func (m Model) openEditValueInEditor() (Model, tea.Cmd) {
	setting, ok := m.actionSetting()
	if !ok || !m.hasStore {
		return m, nil
	}
	if setting.ReadOnly {
		m.Notify(appshell.LevelWarn, fmt.Sprintf("%s is locked (read-only); unlock it in the portal before editing", setting.DisplayKey()))
		return m, nil
	}
	text, isJSON := editorText(setting)
	return m.launchEditor(externalEdit{target: setting, original: text, wantJSON: isJSON}, setting.DisplayKey())
}

// launchEditor writes text to a private temp file and hands it to the
// editor, recording the edit on the model. Embedded, the parent runs
// the process; standalone, the tab does.
func (m Model) launchEditor(ed externalEdit, filenameHint string) (Model, tea.Cmd) {
	argv, err := editorCommand()
	if err != nil {
		m.Notify(appshell.LevelError, err.Error())
		return m, nil
	}
	ext := ".txt"
	if ed.wantJSON {
		ext = ".json"
	}
	f, err := os.CreateTemp("", "lazyaz-"+sanitizeForFilename(filenameHint)+"-*"+ext)
	if err != nil {
		m.Notify(appshell.LevelError, "Could not create a temp file for the editor: "+err.Error())
		return m, nil
	}
	path := f.Name()
	if _, err := f.WriteString(ed.original); err != nil {
		f.Close()
		os.Remove(path)
		m.Notify(appshell.LevelError, "Could not write the temp file for the editor: "+err.Error())
		return m, nil
	}
	f.Close()

	ed.path = path
	ed.started = time.Now()
	m.externalEdit = &ed
	argv = append(argv, path)
	if m.EmbeddedMode {
		return m, func() tea.Msg { return ExternalEditRequestMsg{Argv: argv, Path: path} }
	}
	c := exec.Command(argv[0], argv[1:]...)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return ExternalEditDoneMsg{Path: path, Err: err}
	})
}

// handleExternalEditDone reads the edited file back, drops it, and
// moves to the confirmation (or back into the create form), or
// explains why not.
func (m Model) handleExternalEditDone(msg ExternalEditDoneMsg) (Model, tea.Cmd) {
	ed := m.externalEdit
	if ed == nil || ed.path != msg.Path {
		os.Remove(msg.Path)
		return m, nil
	}
	m.externalEdit = nil
	data, readErr := os.ReadFile(ed.path)
	os.Remove(ed.path)

	if ed.forCreate {
		return m.handleComposedCreateValue(ed, string(data), msg.Err, readErr)
	}

	if msg.Err != nil {
		m.Notify(appshell.LevelError, "Editor exited with an error; nothing saved: "+msg.Err.Error())
		return m, nil
	}
	if readErr != nil {
		m.Notify(appshell.LevelError, "Could not read the edited file; nothing saved: "+readErr.Error())
		return m, nil
	}
	edited := string(data)
	if edited == ed.original {
		if time.Since(ed.started) < time.Second {
			m.Notify(appshell.LevelWarn, "Editor returned at once and the value is unchanged. A GUI editor needs its wait flag, e.g. EDITOR=\"code --wait\"")
		} else {
			m.Notify(appshell.LevelInfo, "Value unchanged")
		}
		return m, nil
	}
	if ed.wantJSON {
		if err := validateJSON(edited); err != nil {
			m.Notify(appshell.LevelError, "Edited value is not valid JSON; nothing saved: "+err.Error())
			return m, nil
		}
	}
	// Editors end a file with a newline; do not store one the value
	// never had.
	if !strings.HasSuffix(ed.target.Value, "\n") {
		edited = strings.TrimSuffix(edited, "\n")
	}
	if edited == ed.target.Value {
		m.Notify(appshell.LevelInfo, "Value unchanged")
		return m, nil
	}
	// Diff against the text the editor showed (pretty-printed JSON),
	// not the stored bytes, so a one-line stored value edited in
	// pretty form does not read as "everything changed".
	return m.confirmSave(ed.target, edited, strings.TrimSuffix(ed.original, "\n"))
}

// confirmSave opens the save confirmation for a new value and arms
// the conditional write. oldShown is the old value as the user saw it
// while editing. Shared by the inline form and the editor.
func (m Model) confirmSave(target appconfig.Setting, newValue, oldShown string) (Model, tea.Cmd) {
	crumbs := []string{m.currentStore.Name, target.DisplayKey()}
	if target.Label != "" {
		crumbs = append(crumbs, target.Label)
	}
	m.confirmModal.OpenWithBreadcrumb(
		"Save value",
		crumbs,
		"Applications reading this store see the change immediately.\n\n"+changeSummary(oldShown, newValue),
		"save", "cancel", false)
	store := m.currentStore
	m.confirmAction = func() tea.Cmd {
		return setValueCmd(m.service, store, target, newValue)
	}
	return m, nil
}

// maxDiffLines bounds the diff shown in the confirmation; the rest is
// summarised as a count so the dialog stays readable.
const maxDiffLines = 14

// changeSummary describes old → new for the confirmation: both values
// inline when they are short single lines, otherwise a line diff with
// every removed (-) and added (+) line, numbered against the old and
// new value respectively.
func changeSummary(oldV, newV string) string {
	if !strings.Contains(oldV, "\n") && !strings.Contains(newV, "\n") && len(oldV) <= 70 && len(newV) <= 70 {
		return fmt.Sprintf("old: %s\nnew: %s", ui.EmptyToDash(oldV), ui.EmptyToDash(newV))
	}
	oldLines := strings.Split(strings.TrimSuffix(oldV, "\n"), "\n")
	newLines := strings.Split(strings.TrimSuffix(newV, "\n"), "\n")
	diff := lineDiff(oldLines, newLines)

	var b strings.Builder
	fmt.Fprintf(&b, "%d → %d lines, %d changed", len(oldLines), len(newLines), len(diff))
	for i, d := range diff {
		if i == maxDiffLines {
			fmt.Fprintf(&b, "\n… and %d more", len(diff)-maxDiffLines)
			break
		}
		fmt.Fprintf(&b, "\n%s%4d  %s", d.sign, d.line, ui.TrimToWidth(d.text, 60))
	}
	return b.String()
}

// diffLine is one removed or added line; line numbers refer to the old
// value for removals and the new value for additions.
type diffLine struct {
	sign string // "-" or "+"
	line int
	text string
}

// lineDiff is a plain longest-common-subsequence diff over lines. The
// values involved are configuration, not source trees, so the quadratic
// table is fine; a cap keeps a pathological value from stalling the UI.
func lineDiff(a, b []string) []diffLine {
	const cap = 2000
	if len(a) > cap || len(b) > cap {
		return []diffLine{{sign: " ", line: 0, text: "(value too large to diff)"}}
	}
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffLine{"-", i + 1, a[i]})
			i++
		default:
			out = append(out, diffLine{"+", j + 1, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, diffLine{"-", i + 1, a[i]})
	}
	for ; j < m; j++ {
		out = append(out, diffLine{"+", j + 1, b[j]})
	}
	return out
}

func validateJSON(s string) error {
	var v any
	return json.Unmarshal([]byte(s), &v)
}

// sanitizeForFilename keeps the key recognisable in the temp file name
// without letting path characters through.
func sanitizeForFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	return filepath.Base(out)
}
