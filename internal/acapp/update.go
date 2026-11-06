package acapp

import (
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		text := paste.String()
		switch {
		case m.SubOverlay.Active:
			m.SubOverlay.PasteText(text, m.Subscriptions)
			return m, nil
		case m.ThemeOverlay.Active:
			m.ThemeOverlay.PasteText(text, m.Schemes)
			return m, nil
		case m.HelpOverlay.Active:
			m.HelpOverlay.PasteText(text)
			return m, nil
		case m.copyOverlay.Active:
			m.copyOverlay.TypeText(text)
			return m, nil
		default:
			return m.updateFocusedList(msg)
		}
	}

	if cursorModel, cursorCmd := m.Cursor.Update(msg); cursorCmd != nil {
		m.Cursor = cursorModel
		m2, listCmd := m.updateFocusedList(msg)
		return m2, tea.Batch(cursorCmd, listCmd)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.Loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case appshell.SubscriptionsLoadedMsg:
		return m.handleSubscriptionsLoaded(msg)

	case storesLoadedMsg:
		return m.handleStoresLoaded(msg)

	case settingsLoadedMsg:
		return m.handleSettingsLoaded(msg)

	case clipboardMsg:
		if msg.err != nil {
			m.Notify(appshell.LevelError, fmt.Sprintf("Clipboard: %s", msg.err.Error()))
		} else {
			m.Notify(appshell.LevelSuccess, fmt.Sprintf("Copied to clipboard: %s", ui.TrimToWidth(msg.text, 60)))
		}
		return m, nil

	case list.FilterMatchesMsg:
		// Dropped: filtering runs synchronously on each keystroke.
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		switch m.inputMode() {
		case ModeNormal, ModeListFilter:
		default:
			return m, nil
		}
		if consumed, double := m.handleMouseClick(msg); consumed {
			if double {
				return m.handleEnter()
			}
			return m, nil
		}
	}

	return m.updateFocusedList(msg)
}

func (m Model) handleSubscriptionsLoaded(msg appshell.SubscriptionsLoadedMsg) (Model, tea.Cmd) {
	matched, status, selectPreferred, cmd := m.Model.HandleSubscriptionsLoaded(msg, m.cache.subscriptions)
	if !selectPreferred {
		return m, cmd
	}
	next, selectCmd := m.selectSubscription(matched)
	next.ClearLoading()
	next.ResolveSpinner(next.LoadingSpinnerID, appshell.LevelSuccess, status)
	return next, selectCmd
}

func (m Model) handleStoresLoaded(msg storesLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load App Configuration stores in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}

	m.stores = msg.stores
	m.storesList.Title = fmt.Sprintf("Stores (%d)", len(m.stores))
	ui.SetItemsPreserveKey(&m.storesList, storesToItems(m.stores), storeItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d stores in %s", len(m.stores), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		if m.pendingNav.hasTarget() {
			return m.advancePendingNav()
		}
		return m, nil
	}
	return m, msg.next
}

func (m Model) handleSettingsLoaded(msg settingsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasStore || m.currentStore.Name != msg.store.Name || m.currentStore.SubscriptionID != msg.store.SubscriptionID {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load settings in %s: %s", m.currentStore.Name, msg.err.Error()))
		return m, nil
	}

	m.settings = msg.settings
	m.settingsList.Title = fmt.Sprintf("Settings (%d)", len(m.settings))
	ui.SetItemsPreserveKey(&m.settingsList, settingsToItems(m.settings), settingItemKey)
	m.refreshOpenValue()

	if msg.done {
		status := fmt.Sprintf("Loaded %d settings in %s", len(m.settings), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		if m.pendingNav.hasTarget() {
			return m.advancePendingNav()
		}
		return m, nil
	}
	return m, msg.next
}

// refreshOpenValue re-reads the open setting from a freshly loaded
// list so a changed value shows without reopening it.
func (m *Model) refreshOpenValue() {
	if !m.viewingValue {
		return
	}
	want := m.currentSetting
	for _, s := range m.settings {
		if s.Key == want.Key && s.Label == want.Label {
			if s.Value != want.Value || s.ContentType != want.ContentType {
				m.currentSetting = s
				m.setValueContent()
			} else {
				m.currentSetting = s
			}
			return
		}
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch m.inputMode() {
	case ModeCopyPalette:
		if target, picked := m.copyOverlay.HandleKey(key, m.Keymap); picked {
			return m.copyToClipboard(target.Value)
		}
		return m, nil

	case ModeOverlay:
		if result := m.HandleOverlayKeys(key); result.Handled {
			if result.SelectSub != nil {
				return m.selectSubscription(*result.SelectSub)
			}
			if result.ThemeSelected {
				m.applyScheme(m.Schemes[m.ThemeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.Schemes[m.ThemeOverlay.ActiveThemeIdx].Name)
			}
		}
		return m, nil

	case ModeListFilter:
		return m.handleListFilterKey(msg, key)

	case ModeNormal:
		return m.handleNormalKey(msg, key)
	}
	return m, nil
}

func (m Model) handleListFilterKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, true):
		return m, tea.Quit
	case m.Keymap.OpenFocused.Matches(key):
		m.commitFocusedFilter()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Filter applied for %s", paneName(m.focus)))
		return m, nil
	}
	return m.updateFocusedList(msg)
}

func (m Model) handleNormalKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	// Esc clears an applied filter on the settings pane first.
	if m.Keymap.Cancel.Matches(key) && m.focus == settingsPane {
		if m.settingsList.FilterState() != list.Unfiltered {
			m.settingsList.ResetFilter()
			m.syncValueToSelection()
			return m, nil
		}
	}

	if ui.ShouldQuit(key, m.Keymap.Quit, false) {
		return m, tea.Quit
	}

	// The value pane is a viewport, not a list: its scrolling, yank and
	// back keys resolve before the list-oriented set below.
	if m.focus == valuePane {
		if m.Keymap.YankSecret.Matches(key) {
			return m.yankValue()
		}
		if updated, cmd, handled := m.handleValueKey(msg, key); handled {
			return updated, cmd
		}
	}

	switch {
	case m.focus != valuePane && m.scrollMotion(key):
		m.syncValueToSelection()
		return m, nil
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		m.syncValueToSelection()
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		m.syncValueToSelection()
		return m, nil
	case m.Keymap.NextFocus.Matches(key):
		m.nextFocus()
		return m, nil
	case m.Keymap.PreviousFocus.Matches(key):
		m.previousFocus()
		return m, nil
	case m.Keymap.RefreshScope.Matches(key):
		return m.refresh()
	case m.Keymap.OpenFocused.Matches(key), m.Keymap.OpenFocusedAlt.Matches(key):
		return m.handleEnter()
	case m.Keymap.NavigateLeft.Matches(key):
		return m.navigateLeft()
	case m.Keymap.YankSecret.Matches(key):
		if m.focus == settingsPane {
			return m.yankValue()
		}
	case m.Keymap.CopyPalette.Matches(key):
		return m.openCopyPalette()
	case !m.EmbeddedMode && m.Keymap.ToggleThemePicker.Matches(key):
		if !m.ThemeOverlay.Active {
			m.ThemeOverlay.Open()
			return m, nil
		}
	case !m.EmbeddedMode && m.Keymap.ToggleHelp.Matches(key):
		if !m.ThemeOverlay.Active {
			if m.HelpOverlay.Active {
				m.HelpOverlay.Close()
			} else {
				m.HelpOverlay.Open("Azure App Configuration Explorer Help", m.HelpSections())
			}
			return m, nil
		}
	case m.Keymap.SubscriptionPicker.Matches(key), m.Keymap.ReloadSubscriptions.Matches(key):
		m.SubOverlay.Open()
		m.StartLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	case m.Keymap.Inspect.Matches(key):
		m.toggleInspect()
		return m, nil
	case m.Keymap.BackspaceUp.Matches(key):
		return m.handleBackspace()
	}

	return m.updateFocusedList(msg)
}

// updateFocusedList forwards a message to the focused list. Cursor
// movement in the settings list drags the open value pane along.
func (m Model) updateFocusedList(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case storesPane:
		cmd = ui.UpdateListSyncFilter(&m.storesList, msg)
	case settingsPane:
		cmd = ui.UpdateListSyncFilter(&m.settingsList, msg)
		m.syncValueToSelection()
	case valuePane:
		m.valueViewport, cmd = m.valueViewport.Update(msg)
	}
	return m, cmd
}
