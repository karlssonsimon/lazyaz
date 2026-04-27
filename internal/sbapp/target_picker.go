package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

const (
	pickerPhaseEntities   = 0
	pickerPhaseNamespaces = 1
)

// targetPickerState manages the cascading overlay for choosing a move target.
type targetPickerState struct {
	active    bool
	phase     int // pickerPhaseEntities or pickerPhaseNamespaces
	cursorIdx int
	query     string
	filtered  []int

	moveAction actionID

	entities   []servicebus.Entity
	namespaces []servicebus.Namespace

	// Cross-namespace selection; zero value means current namespace.
	targetNS servicebus.Namespace
	crossNS  bool // true after selecting "Other namespace..."
}

func (s *targetPickerState) close() {
	*s = targetPickerState{}
}

func (s *targetPickerState) refilter() {
	if s.query == "" {
		s.filtered = nil
		s.cursorIdx = 0
		return
	}

	switch s.phase {
	case pickerPhaseEntities:
		// Index 0 is the "Other namespace..." entry; filter over entity entries starting at 1.
		type indexed struct {
			idx   int
			label string
		}
		var items []indexed
		for i, e := range s.entities {
			items = append(items, indexed{idx: i + 1, label: e.Name}) // +1 for the synthetic entry
		}
		matches := fuzzy.Filter(s.query, items, func(it indexed) string { return it.label })
		s.filtered = make([]int, len(matches))
		for i, m := range matches {
			s.filtered[i] = items[m].idx
		}
	case pickerPhaseNamespaces:
		s.filtered = fuzzy.Filter(s.query, s.namespaces, func(ns servicebus.Namespace) string {
			return ns.Name
		})
	}

	if s.cursorIdx >= s.visibleCount() {
		s.cursorIdx = max(0, s.visibleCount()-1)
	}
}

func (s *targetPickerState) visibleCount() int {
	if s.filtered != nil {
		return len(s.filtered)
	}
	switch s.phase {
	case pickerPhaseEntities:
		return 1 + len(s.entities) // "Other namespace..." + entities
	case pickerPhaseNamespaces:
		return len(s.namespaces)
	}
	return 0
}

// openTargetPicker initializes the picker with entities from the current namespace,
// excluding the source entity.
func (m *Model) openTargetPicker(moveAction actionID) {
	var filtered []servicebus.Entity
	for _, e := range m.entities {
		if e.Name != m.currentEntity.Name {
			filtered = append(filtered, e)
		}
	}
	m.targetPicker = targetPickerState{
		active:     true,
		phase:      pickerPhaseEntities,
		moveAction: moveAction,
		entities:   filtered,
		namespaces: m.namespaces,
	}
}

func (s *targetPickerState) handleKey(key string, km keymap.Keymap) (result targetPickerResult) {
	switch {
	case km.ThemeUp.Matches(key):
		if s.cursorIdx > 0 {
			s.cursorIdx--
		}
	case km.ThemeDown.Matches(key):
		if s.cursorIdx < s.visibleCount()-1 {
			s.cursorIdx++
		}
	case km.ThemeApply.Matches(key):
		return s.handleSelect()
	case km.ThemeCancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.filtered = nil
			s.cursorIdx = 0
		} else if s.phase == pickerPhaseNamespaces {
			// Go back to entity list.
			s.phase = pickerPhaseEntities
			s.cursorIdx = 0
			s.query = ""
			s.filtered = nil
			s.crossNS = false
			s.targetNS = servicebus.Namespace{}
		} else {
			s.close()
		}
	case km.BackspaceUp.Matches(key):
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.query += text
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.query += key
			s.refilter()
		}
	}
	return targetPickerResult{}
}

type targetPickerResult struct {
	// switchToNamespaces triggers phase transition to namespace list.
	switchToNamespaces bool
	// fetchNamespaceEntities means a namespace was selected and we need
	// to fetch its entities.
	fetchNamespaceEntities bool
	selectedNamespace      servicebus.Namespace
	// execute means a target entity was selected; execute the move.
	execute      bool
	targetNS     servicebus.Namespace
	targetEntity servicebus.Entity
}

func (s *targetPickerState) handleSelect() targetPickerResult {
	switch s.phase {
	case pickerPhaseEntities:
		idx := s.cursorIdx
		if s.filtered != nil {
			if len(s.filtered) == 0 {
				return targetPickerResult{}
			}
			idx = s.filtered[s.cursorIdx]
		}
		// Index 0 is the "Other namespace..." synthetic entry.
		if idx == 0 {
			s.phase = pickerPhaseNamespaces
			s.cursorIdx = 0
			s.query = ""
			s.filtered = nil
			return targetPickerResult{switchToNamespaces: true}
		}
		// Entity selected — execute the move.
		entity := s.entities[idx-1] // -1 for synthetic entry
		ns := s.targetNS
		return targetPickerResult{
			execute:      true,
			targetNS:     ns,
			targetEntity: entity,
		}

	case pickerPhaseNamespaces:
		idx := s.cursorIdx
		if s.filtered != nil {
			if len(s.filtered) == 0 {
				return targetPickerResult{}
			}
			idx = s.filtered[s.cursorIdx]
		}
		if idx >= len(s.namespaces) {
			return targetPickerResult{}
		}
		ns := s.namespaces[idx]
		s.targetNS = ns
		s.crossNS = true
		return targetPickerResult{
			fetchNamespaceEntities: true,
			selectedNamespace:      ns,
		}
	}
	return targetPickerResult{}
}

// updateTargetPicker handles key input and side effects for the picker.
func (m Model) updateTargetPicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	result := m.targetPicker.handleKey(msg.String(), m.Keymap)

	if result.fetchNamespaceEntities {
		m.startLoading(-1, fmt.Sprintf("Loading entities from %s...", result.selectedNamespace.Name))
		return m, tea.Batch(m.Spinner.Tick,
			fetchTargetEntitiesCmd(m.service, result.selectedNamespace))
	}

	if result.execute {
		moveAction := m.targetPicker.moveAction
		m.targetPicker.close()
		return m.executeMoveAction(moveAction, result)
	}

	return m, nil
}

// executeMoveAction dispatches the actual move command after a target is selected.
func (m Model) executeMoveAction(moveAction actionID, result targetPickerResult) (Model, tea.Cmd) {
	targetNS := result.targetNS
	if targetNS.Name == "" {
		targetNS = m.currentNS
	}

	switch moveAction {
	case actionMoveAll:
		active, dead := m.currentMessageCounts()
		label := "active"
		count := int(active)
		if m.deadLetter {
			label = "DLQ"
			count = int(dead)
		}
		m.startLoading(m.focus, fmt.Sprintf("Moving all %s messages to %s/%s...", label, targetNS.Name, result.targetEntity.Name))
		return m, tea.Batch(m.Spinner.Tick,
			moveAllCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentSubName, m.deadLetter, targetNS, result.targetEntity.Name, count))

	case actionMoveCurrent:
		targets := m.lockedMessageTargets()
		m.startLoading(m.focus, fmt.Sprintf("Moving %d message(s) to %s/%s...", len(targets), targetNS.Name, result.targetEntity.Name))
		return m, tea.Batch(m.Spinner.Tick,
			moveMarkedCmd(m.service, targetNS, result.targetEntity.Name, m.lockedMessages, targets))
	}

	return m, nil
}

// handleTargetEntitiesLoaded populates the picker with entities from the selected namespace.
func (m Model) handleTargetEntitiesLoaded(msg targetEntitiesLoadedMsg) (Model, tea.Cmd) {
	m.ClearLoading()

	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError,
			fmt.Sprintf("Failed to load entities from %s: %s", msg.namespace.Name, msg.err))
		m.targetPicker.close()
		return m, nil
	}

	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess,
		fmt.Sprintf("Loaded %d entities from %s", len(msg.entities), msg.namespace.Name))

	if !m.targetPicker.active {
		return m, nil
	}

	// Filter out the source entity if same namespace.
	var filtered []servicebus.Entity
	for _, e := range msg.entities {
		if msg.namespace.Name == m.currentNS.Name && e.Name == m.currentEntity.Name {
			continue
		}
		filtered = append(filtered, e)
	}
	m.targetPicker.entities = filtered
	m.targetPicker.phase = pickerPhaseEntities
	m.targetPicker.cursorIdx = 0
	m.targetPicker.query = ""
	m.targetPicker.filtered = nil

	return m, nil
}

func (m Model) renderTargetPicker(base string) string {
	s := &m.targetPicker

	var title string
	switch s.phase {
	case pickerPhaseEntities:
		if s.crossNS {
			title = fmt.Sprintf("Move to (%s)", s.targetNS.Name)
		} else {
			title = fmt.Sprintf("Move to (%s)", m.currentNS.Name)
		}
	case pickerPhaseNamespaces:
		title = "Select namespace"
	}

	indices := s.filtered
	var items []ui.OverlayItem

	switch s.phase {
	case pickerPhaseEntities:
		if indices == nil {
			indices = make([]int, 1+len(s.entities))
			for i := range indices {
				indices[i] = i
			}
		}
		items = make([]ui.OverlayItem, len(indices))
		for ci, si := range indices {
			if si == 0 {
				items[ci] = ui.OverlayItem{
					Label: "Other namespace...",
					Hint:  "cross-ns",
				}
			} else {
				e := s.entities[si-1]
				kind := "queue"
				if e.Kind == servicebus.EntityTopic {
					kind = "topic"
				}
				items[ci] = ui.OverlayItem{
					Label: e.Name,
					Hint:  kind,
				}
			}
		}
	case pickerPhaseNamespaces:
		if indices == nil {
			indices = make([]int, len(s.namespaces))
			for i := range indices {
				indices[i] = i
			}
		}
		items = make([]ui.OverlayItem, len(indices))
		for ci, si := range indices {
			items[ci] = ui.OverlayItem{
				Label: s.namespaces[si].Name,
			}
		}
	}

	cfg := ui.OverlayListConfig{
		Title:      title,
		Query:      s.query,
		CursorView: m.Cursor.View(),
		CloseHint:  m.Keymap.Cancel.Short(),
		Bindings: &ui.OverlayBindings{

			MoveUp:   m.Keymap.ThemeUp,

			MoveDown: m.Keymap.ThemeDown,

			Apply:    m.Keymap.ThemeApply,

			Cancel:   m.Keymap.ThemeCancel,

			Erase:    m.Keymap.BackspaceUp,

		},
		MaxVisible: 10,
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.cursorIdx, m.Styles, m.Width, m.Height, base)
}
