package kvapp

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
	// Handle paste before cursor update (which can swallow PasteMsg).
	if paste, ok := msg.(tea.PasteMsg); ok {
		text := paste.String()
		switch {
		case m.SubOverlay.Active:
			m.SubOverlay.Query += text
			m.SubOverlay.Refilter(m.Subscriptions)
			return m, nil
		case m.ThemeOverlay.Active:
			m.ThemeOverlay.PasteText(text, m.Schemes)
			return m, nil
		case m.HelpOverlay.Active:
			m.HelpOverlay.PasteText(text)
			return m, nil
		case m.actionMenu.active:
			m.actionMenu.query += text
			m.actionMenu.refilter()
			return m, nil
		default:
			var cmd tea.Cmd
			switch m.focus {
			case vaultsPane:
				m.vaultsList, cmd = m.vaultsList.Update(msg)
			case secretsPane:
				m.secretsList, cmd = m.secretsList.Update(msg)
			case versionsPane:
				m.versionsList, cmd = m.versionsList.Update(msg)
			}
			return m, cmd
		}
	}

	if cursorModel, cursorCmd := m.Cursor.Update(msg); cursorCmd != nil {
		m.Cursor = cursorModel
		var listCmd tea.Cmd
		switch m.focus {
		case vaultsPane:
			m.vaultsList, listCmd = m.vaultsList.Update(msg)
		case secretsPane:
			m.secretsList, listCmd = m.secretsList.Update(msg)
		case versionsPane:
			m.versionsList, listCmd = m.versionsList.Update(msg)
		}
		return m, tea.Batch(cursorCmd, listCmd)
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

	case vaultsLoadedMsg:
		return m.handleVaultsLoaded(msg)

	case secretsLoadedMsg:
		return m.handleSecretsLoaded(msg)

	case versionsLoadedMsg:
		return m.handleVersionsLoaded(msg)

	case secretValueYankedMsg:
		return m.handleSecretValueYanked(msg)

	case clipboardMsg:
		if msg.err != nil {
			m.Notify(appshell.LevelError, fmt.Sprintf("Clipboard: %s", msg.err.Error()))
		} else {
			m.Notify(appshell.LevelSuccess, fmt.Sprintf("Copied to clipboard: %s", msg.text))
		}
		return m, nil

	case markedSecretsYankedMsg:
		m.ClearLoading()
		if msg.err != nil {
			m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to yank secrets: %s", msg.err.Error()))
		} else {
			m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Yanked %d secrets as JSON to clipboard", msg.count))
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Fallthrough: propagate to the focused list so filter/selection keys
	// reach the underlying bubbles list.
	var cmd tea.Cmd
	switch m.focus {
	case vaultsPane:
		m.vaultsList, cmd = m.vaultsList.Update(msg)
	case secretsPane:
		m.secretsList, cmd = m.secretsList.Update(msg)
	case versionsPane:
		m.versionsList, cmd = m.versionsList.Update(msg)
	}
	return m, cmd
}

func (m Model) handleSubscriptionsLoaded(msg appshell.SubscriptionsLoadedMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load subscriptions: %s", msg.Err.Error()))
		return m, nil
	}

	m.Subscriptions = msg.Subscriptions
	// Keep the overlay's filtered view in sync with the streaming results
	// so new subscriptions matching the user's query appear immediately.
	if m.SubOverlay.Active {
		m.SubOverlay.Refilter(m.Subscriptions)
	}

	if msg.Done {
		m.cache.subscriptions.Set("", msg.Subscriptions)
		status := fmt.Sprintf("Loaded %d subscriptions in %s", len(msg.Subscriptions), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		if !m.HasSubscription {
			if matched, ok := m.TryApplyPreferredSubscription(); ok {
				// The constructor opened the picker overlay; selectSubscription
				// drives navigation but doesn't dismiss it (the interactive
				// path is dismissed inside the overlay's HandleKey). Close
				// it here so the data loading behind it actually shows.
				m.SubOverlay.Close()
				next, selectCmd := m.selectSubscription(matched)
				next.ClearLoading()
				next.ResolveSpinner(next.loadingSpinnerID, appshell.LevelSuccess, status)
				return next, selectCmd
			}
			m.SubOverlay.Open()
		}
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.Next
}

func (m Model) handleVaultsLoaded(msg vaultsLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load key vaults in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}

	m.vaults = msg.vaults
	m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(m.vaults))
	ui.SetItemsPreserveKey(&m.vaultsList, vaultsToItems(m.vaults), vaultItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d vaults in %s", len(m.vaults), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleSecretsLoaded(msg secretsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasVault || m.currentVault.Name != msg.vault.Name {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load secrets in %s: %s", msg.vault.Name, msg.err.Error()))
		return m, nil
	}

	m.secrets = msg.secrets
	m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(m.secrets))
	ui.SetItemsPreserveKey(&m.secretsList, secretsToItems(m.secrets), secretItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d secrets from %s in %s", len(m.secrets), msg.vault.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleVersionsLoaded(msg versionsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasSecret || m.currentSecret.Name != msg.secretName {
		return m, nil
	}
	if m.currentVault.Name != msg.vault.Name {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load versions for %s: %s", msg.secretName, msg.err.Error()))
		return m, nil
	}

	m.versions = msg.versions
	m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(m.versions))
	ui.SetItemsPreserveKey(&m.versionsList, versionsToItems(m.versions), versionItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d versions for %s in %s", len(m.versions), msg.secretName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleSecretValueYanked(msg secretValueYankedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to yank secret value: %s", msg.err.Error()))
		return m, nil
	}

	label := msg.secretName
	if msg.version != "" {
		v := msg.version
		if len(v) > 12 {
			v = v[:12]
		}
		label = fmt.Sprintf("%s@%s", msg.secretName, v)
	}
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Yanked %s to clipboard", label))
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	if m.actionMenu.active {
		if selected, act := m.actionMenu.handleKey(key, m.Keymap); selected {
			return m.executeAction(act)
		}
		return m, nil
	}

	if result := m.HandleOverlayKeys(key); result.Handled {
		if result.SelectSub != nil {
			return m.selectSubscription(*result.SelectSub)
		}
		if result.ThemeSelected {
			m.applyScheme(m.Schemes[m.ThemeOverlay.ActiveThemeIdx])
			ui.SaveThemeName(m.Schemes[m.ThemeOverlay.ActiveThemeIdx].Name)
		}
		return m, nil
	}

	focusedFilterActive := m.focusedListSettingFilter()

	// Esc peels selection state like a stack on the secrets pane:
	//  1. If the bubbles list has an applied filter → clear it and return.
	//  2. Otherwise fall through to normal esc handlers (visual mode, etc).
	if m.Keymap.Cancel.Matches(key) && m.focus == secretsPane {
		if m.secretsList.FilterState() != list.Unfiltered {
			m.secretsList.ResetFilter()
			return m, nil
		}
	}

	// Track visual range updates after list cursor moves.
	markVisualAfterListUpdate := m.focus == secretsPane && m.visualLineMode && !focusedFilterActive && m.Keymap.BlobVisualMove.Matches(key)

	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, focusedFilterActive):
		return m, tea.Quit
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
	case m.Keymap.NextFocus.Matches(key):
		if !focusedFilterActive {
			m.nextFocus()
			return m, nil
		}
	case m.Keymap.PreviousFocus.Matches(key):
		if !focusedFilterActive {
			m.previousFocus()
			return m, nil
		}
	case m.Keymap.RefreshScope.Matches(key):
		if !focusedFilterActive {
			return m.refresh()
		}
	case m.Keymap.OpenFocused.Matches(key):
		if focusedFilterActive {
			m.commitFocusedFilter()
			m.Notify(appshell.LevelInfo, fmt.Sprintf("Filter applied for %s", paneName(m.focus)))
			return m, nil
		}
		return m.handleEnter()
	case m.Keymap.OpenFocusedAlt.Matches(key):
		if !focusedFilterActive {
			return m.handleEnter()
		}
	case m.Keymap.NavigateLeft.Matches(key):
		if !focusedFilterActive {
			return m.navigateLeft()
		}
	case m.Keymap.ToggleVisualLine.Matches(key):
		if m.focus == secretsPane && !focusedFilterActive {
			m.toggleVisualLineMode()
			return m, nil
		}
	case m.Keymap.ToggleMark.Matches(key):
		if m.focus == secretsPane && !focusedFilterActive {
			m.toggleCurrentSecretMark()
			return m, nil
		}
	case m.Keymap.VisualSwapAnchor.Matches(key):
		if m.focus == secretsPane && m.visualLineMode && !focusedFilterActive {
			m.swapVisualAnchor()
			m.refreshSecretSelectionDisplay()
			return m, nil
		}
	case m.Keymap.ExitVisualLine.Matches(key):
		if m.focus == secretsPane && !focusedFilterActive {
			if m.visualLineMode {
				m.commitVisualSelection()
				m.visualLineMode = false
				m.visualAnchor = ""
				m.refreshSecretSelectionDisplay()
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.markedSecrets)))
				return m, nil
			}
			if len(m.markedSecrets) > 0 {
				count := len(m.markedSecrets)
				for name := range m.markedSecrets {
					delete(m.markedSecrets, name)
				}
				m.refreshSecretSelectionDisplay()
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
				return m, nil
			}
		}
	case m.Keymap.ActionMenu.Matches(key):
		if m.focus == secretsPane && !focusedFilterActive && m.hasVault {
			m.actionMenu.open(m.buildActions())
			return m, nil
		}
	case m.Keymap.YankSecret.Matches(key):
		if !focusedFilterActive {
			return m.handleYank()
		}
	case !m.EmbeddedMode && m.Keymap.ToggleThemePicker.Matches(key):
		if !focusedFilterActive && !m.ThemeOverlay.Active {
			m.ThemeOverlay.Open()
			return m, nil
		}
	case !m.EmbeddedMode && m.Keymap.ToggleHelp.Matches(key):
		if !focusedFilterActive && !m.ThemeOverlay.Active {
			if m.HelpOverlay.Active {
				m.HelpOverlay.Close()
			} else {
				m.HelpOverlay.Open("Azure Key Vault Explorer Help", m.HelpSections())
			}
			return m, nil
		}
	case m.Keymap.SubscriptionPicker.Matches(key):
		if !focusedFilterActive {
			m.SubOverlay.Open()
			m.startLoading(-1, "Refreshing subscriptions...")
			return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
		}
	case m.Keymap.Inspect.Matches(key):
		if !focusedFilterActive {
			m.toggleInspect()
			return m, nil
		}
	case m.Keymap.BackspaceUp.Matches(key):
		if !focusedFilterActive {
			return m.handleBackspace()
		}
	}

	// Key didn't match any app-specific handler — fall through to the
	// focused list so filter input and cursor keys reach it.
	var cmd tea.Cmd
	switch m.focus {
	case vaultsPane:
		m.vaultsList, cmd = m.vaultsList.Update(msg)
	case secretsPane:
		m.secretsList, cmd = m.secretsList.Update(msg)
	case versionsPane:
		m.versionsList, cmd = m.versionsList.Update(msg)
	}
	if markVisualAfterListUpdate {
		m.refreshSecretSelectionDisplay()
	}
	return m, cmd
}

func (m Model) handleYank() (Model, tea.Cmd) {
	if m.focus == secretsPane {
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}
		m.startLoading(m.focus, fmt.Sprintf("Fetching secret value for %s...", item.secret.Name))
		return m, tea.Batch(m.Spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, item.secret.Name, ""))
	}

	if m.focus == versionsPane {
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok {
			return m, nil
		}
		m.startLoading(m.focus, fmt.Sprintf("Fetching secret value for %s@%s...", m.currentSecret.Name, item.version.Version))
		return m, tea.Batch(m.Spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, m.currentSecret.Name, item.version.Version))
	}

	return m, nil
}
