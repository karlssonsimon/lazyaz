package kvapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// Azure Key Vault secret name: 1-127 chars, [0-9a-zA-Z-]. The portal
// and SDK enforce the same; reject up front so we don't get a confusing
// 400 mid-paste.
var secretNamePattern = regexp.MustCompile(`^[0-9a-zA-Z-]{1,127}$`)

// pasteRow is one secret about to be created in the target vault.
type pasteRow struct {
	name          string
	value         string
	existsInVault bool
	skip          bool
}

// pasteModalState is the state of the paste-secrets modal. It owns the
// row list, cursor position, and visual-line selection state. Renders
// and consumes keys when active; produces a Plan() that the model
// dispatches to Azure when the user submits.
type pasteModalState struct {
	active       bool
	rows         []pasteRow
	cursor       int
	visual       bool
	visualAnchor int
	vaultName    string
}

// pastePlan is what falls out of the modal when the user submits.
// Apply lists every non-skipped row in stable order; the counts mirror
// what gets displayed in the status bar after execution.
type pastePlan struct {
	Apply      []pasteRow
	NewVersion int
	Create     int
	Skip       int
}

// parsePasteBundle is the validation pass that decides whether `p`
// opens the modal or quietly notifies. Returns the parsed name→value
// map on success, or a descriptive error the caller can show.
func parsePasteBundle(clipboard string) (map[string]string, error) {
	trimmed := strings.TrimSpace(clipboard)
	if trimmed == "" {
		return nil, errors.New("clipboard is empty")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, errors.New("clipboard doesn't look like a secrets bundle")
	}
	if len(raw) == 0 {
		return nil, errors.New("clipboard bundle has no secrets")
	}

	out := make(map[string]string, len(raw))
	for name, rawValue := range raw {
		if !secretNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid secret name %q", name)
		}
		// Reject null up front — json.Unmarshal of `null` into a string
		// silently sets "" without erroring, which would otherwise let
		// `{"x": null}` through as an empty-value secret.
		if string(rawValue) == "null" {
			return nil, fmt.Errorf("secret %q: value must be a string", name)
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil, fmt.Errorf("secret %q: value must be a string", name)
		}
		out[name] = value
	}
	return out, nil
}

// newPasteModalState validates the clipboard payload and builds the
// modal state. existing is the current vault's secret list so we can
// pre-classify each row as new-version vs create.
func newPasteModalState(clipboard string, existing []keyvault.Secret) (pasteModalState, error) {
	bundle, err := parsePasteBundle(clipboard)
	if err != nil {
		return pasteModalState{}, err
	}

	existsByName := make(map[string]bool, len(existing))
	for _, s := range existing {
		existsByName[s.Name] = true
	}

	names := make([]string, 0, len(bundle))
	for n := range bundle {
		names = append(names, n)
	}
	sort.Strings(names)

	rows := make([]pasteRow, 0, len(names))
	for _, n := range names {
		rows = append(rows, pasteRow{
			name:          n,
			value:         bundle[n],
			existsInVault: existsByName[n],
		})
	}

	return pasteModalState{active: true, rows: rows}, nil
}

// withVaultName labels the modal header. Optional convenience, no
// behavior change.
func (s pasteModalState) withVaultName(name string) pasteModalState {
	s.vaultName = name
	return s
}

// close returns a zeroed modal. The caller stops routing keys to it.
func (s pasteModalState) close() pasteModalState {
	return pasteModalState{}
}

// moveCursor clamps to the row range.
func (s pasteModalState) moveCursor(delta int) pasteModalState {
	if len(s.rows) == 0 {
		return s
	}
	next := s.cursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(s.rows) {
		next = len(s.rows) - 1
	}
	s.cursor = next
	return s
}

// toggleCurrentSkip flips skip on the row under the cursor, or — if
// visual mode is on — across the anchor→cursor range.
func (s pasteModalState) toggleCurrentSkip(_ keymap.Keymap) pasteModalState {
	if s.visual {
		return s.toggleCurrentSkipVisual()
	}
	if len(s.rows) == 0 {
		return s
	}
	s.rows[s.cursor].skip = !s.rows[s.cursor].skip
	return s
}

// toggleCurrentSkipVisual flips skip across the visual range. If the
// range is mixed, force-skip everything (matches secrets-pane mark
// behavior — pulling a mixed range to the "skipped" side is more
// predictable than toggling each row independently).
func (s pasteModalState) toggleCurrentSkipVisual() pasteModalState {
	if !s.visual || len(s.rows) == 0 {
		return s
	}
	start, end := s.visualAnchor, s.cursor
	if start > end {
		start, end = end, start
	}
	allSkipped := true
	for i := start; i <= end; i++ {
		if !s.rows[i].skip {
			allSkipped = false
			break
		}
	}
	target := true
	if allSkipped {
		target = false
	}
	for i := start; i <= end; i++ {
		s.rows[i].skip = target
	}
	s.visual = false
	s.visualAnchor = 0
	return s
}

// toggleVisual flips visual-line mode. Re-entering the anchor uses the
// current cursor row.
func (s pasteModalState) toggleVisual() pasteModalState {
	if s.visual {
		s.visual = false
		s.visualAnchor = 0
		return s
	}
	s.visual = true
	s.visualAnchor = s.cursor
	return s
}

// Plan collects the apply set and per-category counts.
func (s pasteModalState) Plan() pastePlan {
	plan := pastePlan{}
	for _, r := range s.rows {
		if r.skip {
			plan.Skip++
			continue
		}
		plan.Apply = append(plan.Apply, r)
		if r.existsInVault {
			plan.NewVersion++
		} else {
			plan.Create++
		}
	}
	return plan
}

// HandleKey dispatches a key event. Returns the next state and whether
// the modal should be considered "consumed" the key (always true while
// active — even unrecognized keys shouldn't fall through to the pane).
func (s pasteModalState) HandleKey(key string, km keymap.Keymap) (pasteModalState, bool) {
	if !s.active {
		return s, false
	}
	switch {
	case km.Cancel.Matches(key):
		return s.close(), true
	case km.CursorDown.Matches(key):
		return s.moveCursor(1), true
	case km.CursorUp.Matches(key):
		return s.moveCursor(-1), true
	case km.ToggleMark.Matches(key):
		return s.toggleCurrentSkip(km), true
	case km.ToggleVisualLine.Matches(key):
		return s.toggleVisual(), true
	}
	return s, true
}

// rowLabel returns the human-readable action label for a row, matching
// what the modal shows in the right column.
func (r pasteRow) actionLabel() string {
	switch {
	case r.skip:
		return "skip"
	case r.existsInVault:
		return "new version"
	default:
		return "create"
	}
}

// pasteResultMsg is emitted when the paste command finishes. Carries
// per-category counts and any per-row errors that occurred mid-loop.
type pasteResultMsg struct {
	created    int
	newVersion int
	errors     []string
}

// pasteSecretsCmd loops SetSecret per applied row. Errors are
// accumulated rather than aborting the loop — partial success is fine
// and matches how Azure handles per-secret writes (each is independent).
func pasteSecretsCmd(svc *keyvault.Service, vault keyvault.Vault, plan pastePlan) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		var msg pasteResultMsg
		for _, r := range plan.Apply {
			if err := svc.SetSecret(ctx, vault, r.name, r.value); err != nil {
				msg.errors = append(msg.errors, fmt.Sprintf("%s: %s", r.name, err.Error()))
				continue
			}
			if r.existsInVault {
				msg.newVersion++
			} else {
				msg.created++
			}
		}
		return msg
	}
}

// View renders the modal body as plain text. Coloring and box chrome
// are layered on top by renderPasteModal.
func (s pasteModalState) View() string {
	if !s.active {
		return ""
	}
	var b strings.Builder
	plan := s.Plan()
	header := fmt.Sprintf("Paste %d secrets", len(s.rows))
	if s.vaultName != "" {
		header += fmt.Sprintf(" into %s", s.vaultName)
	}
	b.WriteString(header)
	b.WriteString(":\n\n")

	nameWidth := 0
	for _, r := range s.rows {
		if w := len(r.name); w > nameWidth {
			nameWidth = w
		}
	}

	for i, r := range s.rows {
		mark := "[ ]"
		if !r.skip {
			mark = "[v]"
		}
		cursor := "  "
		if i == s.cursor {
			cursor = "> "
		}
		visualMarker := " "
		if s.visual {
			start, end := s.visualAnchor, s.cursor
			if start > end {
				start, end = end, start
			}
			if i >= start && i <= end {
				visualMarker = "*"
			}
		}
		fmt.Fprintf(&b, "%s%s%s %-*s %s\n", cursor, visualMarker, mark, nameWidth, r.name, r.actionLabel())
	}

	fmt.Fprintf(&b, "\nplan: %d create · %d new version · %d skip\n",
		plan.Create, plan.NewVersion, plan.Skip)
	b.WriteString("\nj/k move · space toggle · V visual · enter paste · esc cancel")
	return b.String()
}

// renderPasteModal layers the modal box over base, centered, using the
// same overlay chrome as the other modals.
func (m Model) renderPasteModal(base string) string {
	content := m.pasteModal.View()
	box := m.Styles.Overlay.Box.Render(content)
	return ui.PlaceOverlay(m.Width, m.Height, box, base)
}
