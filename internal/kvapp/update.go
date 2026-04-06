package kvapp

import (
	"fmt"
	"time"

	"azure-storage/internal/cache"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case loadingHoldExpiredMsg:
		m.clearLoading()
		m.status = msg.status
		return m, nil

	case subscriptionsLoadedMsg:
		if msg.err != nil {
			m.clearLoading()
			m.lastErr = msg.err.Error()
			m.status = "Failed to load subscriptions"
			return m, nil
		}

		m.lastErr = ""
		m.subscriptions = msg.subscriptions

		if msg.done {
			m.cache.subscriptions.Set("", msg.subscriptions)
			if !m.hasSubscription {
				m.subOverlay.Open()
			}
			status := fmt.Sprintf("Loaded %d subscriptions in %s", len(msg.subscriptions), time.Since(m.loadingStartedAt).Round(time.Millisecond))
			return m, m.finishLoading(status)
		}

		return m, msg.next

	case vaultsLoadedMsg:
		if !m.hasSubscription || m.currentSub.ID != msg.subscriptionID {
			return m, nil
		}

		if msg.err != nil {
			m.clearLoading()
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load key vaults in %s", subscriptionDisplayName(m.currentSub))
			return m, nil
		}

		m.lastErr = ""
		m.vaults = msg.vaults
		m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(msg.vaults))
		ui.SetItemsPreserveIndex(&m.vaultsList, vaultsToItems(msg.vaults))

		if msg.done {
			m.cache.vaults.Set(msg.subscriptionID, msg.vaults)
			status := fmt.Sprintf("Loaded %d vaults in %s", len(msg.vaults), time.Since(m.loadingStartedAt).Round(time.Millisecond))
			return m, m.finishLoading(status)
		}

		return m, msg.next

	case secretsLoadedMsg:
		if !m.hasVault || m.currentVault.Name != msg.vault.Name {
			return m, nil
		}

		if msg.err != nil {
			m.clearLoading()
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load secrets in %s", msg.vault.Name)
			return m, nil
		}

		m.lastErr = ""
		m.secrets = msg.secrets
		m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(msg.secrets))
		ui.SetItemsPreserveIndex(&m.secretsList, secretsToItems(msg.secrets))

		if msg.done {
			m.cache.secrets.Set(cache.Key(m.currentSub.ID, msg.vault.Name), msg.secrets)
			status := fmt.Sprintf("Loaded %d secrets from %s in %s", len(msg.secrets), msg.vault.Name, time.Since(m.loadingStartedAt).Round(time.Millisecond))
			return m, m.finishLoading(status)
		}

		return m, msg.next

	case versionsLoadedMsg:
		if !m.hasSecret || m.currentSecret.Name != msg.secretName {
			return m, nil
		}
		if m.currentVault.Name != msg.vault.Name {
			return m, nil
		}

		if msg.err != nil {
			m.clearLoading()
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load versions for %s", msg.secretName)
			return m, nil
		}

		m.lastErr = ""
		m.versions = msg.versions
		m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(msg.versions))
		ui.SetItemsPreserveIndex(&m.versionsList, versionsToItems(msg.versions))

		if msg.done {
			m.cache.versions.Set(cache.Key(m.currentSub.ID, msg.vault.Name, msg.secretName), msg.versions)
			status := fmt.Sprintf("Loaded %d versions for %s in %s", len(msg.versions), msg.secretName, time.Since(m.loadingStartedAt).Round(time.Millisecond))
			return m, m.finishLoading(status)
		}

		return m, msg.next

	case secretValueYankedMsg:
		m.clearLoading()
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to yank secret value"
			return m, nil
		}

		m.lastErr = ""
		label := msg.secretName
		if msg.version != "" {
			v := msg.version
			if len(v) > 12 {
				v = v[:12]
			}
			label = fmt.Sprintf("%s@%s", msg.secretName, v)
		}
		m.status = fmt.Sprintf("Yanked %s to clipboard", label)
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		if m.subOverlay.Active {
			if sub, ok := m.subOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}, m.subscriptions); ok {
				return m.selectSubscription(sub)
			}
			return m, nil
		}

		if !m.EmbeddedMode && m.helpOverlay.Active {
			m.helpOverlay.HandleKey(key, ui.HelpKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Close: m.keymap.ToggleHelp,
			})
			return m, nil
		}
		if !m.EmbeddedMode && m.themeOverlay.Active {
			if m.themeOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}, m.schemes) {
				m.applyScheme(m.schemes[m.themeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.schemes[m.themeOverlay.ActiveThemeIdx].Name)
			}
			return m, nil
		}

		// Inspect overlay — dismiss.
		if m.inspectFields != nil {
			if m.keymap.Inspect.Matches(key) || key == "esc" || key == "q" {
				m.inspectFields = nil
			}
			return m, nil
		}

		focusedFilterActive := m.focusedListSettingFilter()

		switch {
		case ui.ShouldQuit(key, m.keymap.Quit, focusedFilterActive):
			return m, tea.Quit
		case m.keymap.HalfPageDown.Matches(key):
			m.scrollFocusedHalfPage(1)
			return m, nil
		case m.keymap.HalfPageUp.Matches(key):
			m.scrollFocusedHalfPage(-1)
			return m, nil
		case m.keymap.NextFocus.Matches(key):
			if !focusedFilterActive {
				m.nextFocus()
				return m, nil
			}
		case m.keymap.PreviousFocus.Matches(key):
			if !focusedFilterActive {
				m.previousFocus()
				return m, nil
			}
		case m.keymap.ReloadSubscriptions.Matches(key):
			if !focusedFilterActive {
				m.setLoading(m.focus)
				m.lastErr = ""
				m.status = "Refreshing subscriptions..."
				return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions))
			}
		case m.keymap.RefreshScope.Matches(key):
			if !focusedFilterActive {
				return m.refresh()
			}
		case m.keymap.OpenFocused.Matches(key):
			if focusedFilterActive {
				m.commitFocusedFilter()
				m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
				return m, nil
			}
			return m.handleEnter()
		case m.keymap.OpenFocusedAlt.Matches(key):
			if !focusedFilterActive {
				return m.handleEnter()
			}
		case m.keymap.NavigateLeft.Matches(key):
			if !focusedFilterActive {
				return m.navigateLeft()
			}
		case m.keymap.YankSecret.Matches(key):
			if !focusedFilterActive {
				return m.handleYank()
			}
		case !m.EmbeddedMode && m.keymap.ToggleThemePicker.Matches(key):
			if !focusedFilterActive && !m.themeOverlay.Active {
				m.themeOverlay.Open()
				return m, nil
			}
		case !m.EmbeddedMode && m.keymap.ToggleHelp.Matches(key):
			if !focusedFilterActive && !m.themeOverlay.Active {
				if m.helpOverlay.Active {
					m.helpOverlay.Close()
				} else {
					m.helpOverlay.Open("Azure Key Vault Explorer Help", m.HelpSections())
				}
				return m, nil
			}
		case m.keymap.SubscriptionPicker.Matches(key):
			if !focusedFilterActive {
				m.subOverlay.Open()
				return m, nil
			}
		case m.keymap.Inspect.Matches(key):
			if !focusedFilterActive {
				m.inspectFocusedItem()
				return m, nil
			}
		case m.keymap.BackspaceUp.Matches(key):
			if !focusedFilterActive {
				return m.handleBackspace()
			}
		}
	}

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
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Fetching secret value for %s...", item.secret.Name)
		return m, tea.Batch(spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, item.secret.Name, ""))
	}

	if m.focus == versionsPane {
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok {
			return m, nil
		}
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Fetching secret value for %s@%s...", m.currentSecret.Name, item.version.Version)
		return m, tea.Batch(spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, m.currentSecret.Name, item.version.Version))
	}

	return m, nil
}
