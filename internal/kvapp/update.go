package kvapp

import (
	"fmt"
	"time"

	"azure-storage/internal/appshell"
	"azure-storage/internal/cache"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case appshell.LoadingHoldExpiredMsg:
		m.ClearLoading()
		m.Status = msg.Status
		return m, nil

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
		m.LastErr = msg.Err.Error()
		m.Status = "Failed to load subscriptions"
		return m, nil
	}

	m.LastErr = ""
	m.Subscriptions = msg.Subscriptions

	if msg.Done {
		m.cache.subscriptions.Set("", msg.Subscriptions)
		if !m.HasSubscription {
			m.SubOverlay.Open()
		}
		status := fmt.Sprintf("Loaded %d subscriptions in %s", len(msg.Subscriptions), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.Next
}

func (m Model) handleVaultsLoaded(msg vaultsLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.LastErr = msg.err.Error()
		m.Status = fmt.Sprintf("Failed to load key vaults in %s", ui.SubscriptionDisplayName(m.CurrentSub))
		return m, nil
	}

	m.LastErr = ""
	m.vaults = msg.vaults
	m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(msg.vaults))
	ui.SetItemsPreserveIndex(&m.vaultsList, vaultsToItems(msg.vaults))

	if msg.done {
		m.cache.vaults.Set(msg.subscriptionID, msg.vaults)
		status := fmt.Sprintf("Loaded %d vaults in %s", len(msg.vaults), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleSecretsLoaded(msg secretsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasVault || m.currentVault.Name != msg.vault.Name {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.LastErr = msg.err.Error()
		m.Status = fmt.Sprintf("Failed to load secrets in %s", msg.vault.Name)
		return m, nil
	}

	m.LastErr = ""
	m.secrets = msg.secrets
	m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(msg.secrets))
	ui.SetItemsPreserveIndex(&m.secretsList, secretsToItems(msg.secrets))

	if msg.done {
		m.cache.secrets.Set(cache.Key(m.CurrentSub.ID, msg.vault.Name), msg.secrets)
		status := fmt.Sprintf("Loaded %d secrets from %s in %s", len(msg.secrets), msg.vault.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
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
		m.LastErr = msg.err.Error()
		m.Status = fmt.Sprintf("Failed to load versions for %s", msg.secretName)
		return m, nil
	}

	m.LastErr = ""
	m.versions = msg.versions
	m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(msg.versions))
	ui.SetItemsPreserveIndex(&m.versionsList, versionsToItems(msg.versions))

	if msg.done {
		m.cache.versions.Set(cache.Key(m.CurrentSub.ID, msg.vault.Name, msg.secretName), msg.versions)
		status := fmt.Sprintf("Loaded %d versions for %s in %s", len(msg.versions), msg.secretName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleSecretValueYanked(msg secretValueYankedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.LastErr = msg.err.Error()
		m.Status = "Failed to yank secret value"
		return m, nil
	}

	m.LastErr = ""
	label := msg.secretName
	if msg.version != "" {
		v := msg.version
		if len(v) > 12 {
			v = v[:12]
		}
		label = fmt.Sprintf("%s@%s", msg.secretName, v)
	}
	m.Status = fmt.Sprintf("Yanked %s to clipboard", label)
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

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
			m.Status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
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
			m.SetLoading(-1)
			m.LastErr = ""
			m.Status = "Refreshing subscriptions..."
			return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
		}
	case m.Keymap.Inspect.Matches(key):
		if !focusedFilterActive {
			m.inspectFocusedItem()
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
	return m, cmd
}

func (m Model) handleYank() (Model, tea.Cmd) {
	if m.focus == secretsPane {
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Fetching secret value for %s...", item.secret.Name)
		return m, tea.Batch(spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, item.secret.Name, ""))
	}

	if m.focus == versionsPane {
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok {
			return m, nil
		}
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Fetching secret value for %s@%s...", m.currentSecret.Name, item.version.Version)
		return m, tea.Batch(spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, m.currentSecret.Name, item.version.Version))
	}

	return m, nil
}
