package blobapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/azure"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	markVisualAfterListUpdate := false

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

	case subscriptionsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to load subscriptions"
			return m, nil
		}

		m.lastErr = ""
		m.subscriptions = msg.subscriptions
		m.subscriptionsList.ResetFilter()
		m.subscriptionsList.SetItems(subscriptionsToItems(msg.subscriptions))
		m.subscriptionsList.Title = fmt.Sprintf("Subscriptions (%d)", len(msg.subscriptions))

		if len(msg.subscriptions) == 0 {
			m.hasSubscription = false
			m.hasAccount = false
			m.hasContainer = false
			m.status = "No subscriptions found. Verify az login context and tenant access."
			m.clearBlobSelectionState()
			m.resetBlobLoadState()
			m.resetPreviewState()
			m.accounts = nil
			m.containers = nil
			m.blobs = nil
			m.accountsList.ResetFilter()
			m.containersList.ResetFilter()
			m.blobsList.ResetFilter()
			m.accountsList.SetItems(nil)
			m.containersList.SetItems(nil)
			m.blobsList.SetItems(nil)
			m.accountsList.Title = "Storage Accounts"
			m.containersList.Title = "Containers"
			m.blobsList.Title = "Blobs"
			return m, nil
		}

		m.subscriptionsList.Select(0)
		m.hasSubscription = false
		m.currentSub = azure.Subscription{}
		m.hasAccount = false
		m.hasContainer = false
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.status = fmt.Sprintf("Loaded %d subscriptions. Select one and press Enter.", len(msg.subscriptions))
		return m, nil

	case accountsLoadedMsg:
		if !m.hasSubscription || m.currentSub.ID != msg.subscriptionID {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load storage accounts in %s", subscriptionDisplayName(m.currentSub))
			return m, nil
		}

		m.lastErr = ""
		m.accounts = msg.accounts
		m.accountsList.ResetFilter()
		m.accountsList.SetItems(accountsToItems(msg.accounts))
		m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(msg.accounts))

		if len(msg.accounts) == 0 {
			m.hasAccount = false
			m.hasContainer = false
			m.status = fmt.Sprintf("No storage accounts found in %s", subscriptionDisplayName(m.currentSub))
			m.clearBlobSelectionState()
			m.resetBlobLoadState()
			m.resetPreviewState()
			m.containers = nil
			m.blobs = nil
			m.containersList.ResetFilter()
			m.blobsList.ResetFilter()
			m.containersList.SetItems(nil)
			m.blobsList.SetItems(nil)
			m.containersList.Title = "Containers"
			m.blobsList.Title = "Blobs"
			return m, nil
		}

		m.accountsList.Select(0)
		m.hasAccount = false
		m.currentAccount = azure.Account{}
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.containers = nil
		m.blobs = nil
		m.containersList.ResetFilter()
		m.blobsList.ResetFilter()
		m.containersList.SetItems(nil)
		m.blobsList.SetItems(nil)
		m.containersList.Title = "Containers"
		m.blobsList.Title = "Blobs"
		m.status = fmt.Sprintf("Loaded %d storage accounts from %s. Open an account to view containers.", len(msg.accounts), subscriptionDisplayName(m.currentSub))
		return m, nil

	case containersLoadedMsg:
		if !m.hasAccount || !sameAccount(m.currentAccount, msg.account) {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load containers for %s", msg.account.Name)
			m.clearBlobSelectionState()
			m.resetBlobLoadState()
			m.resetPreviewState()
			m.containers = nil
			m.blobs = nil
			m.containersList.ResetFilter()
			m.blobsList.ResetFilter()
			m.containersList.SetItems(nil)
			m.blobsList.SetItems(nil)
			m.hasContainer = false
			m.containerName = ""
			m.prefix = ""
			return m, nil
		}

		m.lastErr = ""
		m.containers = msg.containers
		m.containersList.ResetFilter()
		m.containersList.SetItems(containersToItems(msg.containers))
		m.containersList.Title = fmt.Sprintf("Containers (%d)", len(msg.containers))
		m.containersList.Select(0)

		if len(msg.containers) == 0 {
			m.hasContainer = false
			m.containerName = ""
			m.prefix = ""
			m.clearBlobSelectionState()
			m.resetBlobLoadState()
			m.resetPreviewState()
			m.blobs = nil
			m.blobsList.ResetFilter()
			m.blobsList.SetItems(nil)
			m.blobsList.Title = "Blobs"
			m.status = fmt.Sprintf("No containers found in %s", msg.account.Name)
			return m, nil
		}

		m.hasContainer = false
		m.containerName = ""
		m.prefix = ""
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.blobs = nil
		m.blobsList.ResetFilter()
		m.blobsList.SetItems(nil)
		m.blobsList.Title = "Blobs"
		m.status = fmt.Sprintf("Loaded %d containers from %s. Open a container to browse blobs.", len(msg.containers), msg.account.Name)
		return m, nil

	case blobsLoadedMsg:
		if !m.hasAccount || !m.hasContainer {
			return m, nil
		}
		if !sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container {
			return m, nil
		}
		if m.prefix != msg.prefix {
			return m, nil
		}
		if m.blobLoadAll != msg.loadAll {
			return m, nil
		}
		if m.blobSearchQuery != msg.query {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load blobs in %s/%s", msg.account.Name, msg.container)
			m.visualLineMode = false
			m.visualAnchor = ""
			m.blobs = nil
			m.blobsList.ResetFilter()
			m.blobsList.SetItems(nil)
			m.blobsList.Title = "Blobs"
			return m, nil
		}

		m.lastErr = ""
		m.visualLineMode = false
		m.visualAnchor = ""
		m.blobs = msg.blobs
		m.blobsList.ResetFilter()
		m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(msg.blobs))
		m.refreshBlobItems()
		if msg.loadAll {
			m.status = fmt.Sprintf("Loaded all %d blobs in %s/%s", len(msg.blobs), msg.account.Name, msg.container)
		} else if msg.query != "" {
			effectivePrefix := blobSearchPrefix(m.prefix, msg.query)
			m.status = fmt.Sprintf("Found %d blobs by prefix %q in %s/%s", len(msg.blobs), effectivePrefix, msg.account.Name, msg.container)
		} else {
			m.status = fmt.Sprintf("Loaded %d entries (max %d) in %s/%s under %q", len(msg.blobs), defaultHierarchyBlobLoadLimit, msg.account.Name, msg.container, msg.prefix)
		}
		return m, nil

	case blobsDownloadedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to download blobs"
			return m, nil
		}

		if msg.failed > 0 {
			m.lastErr = strings.Join(msg.failures, " | ")
			m.status = fmt.Sprintf("Downloaded %d/%d blobs to %s", msg.downloaded, msg.total, msg.destinationRoot)
			return m, nil
		}

		m.lastErr = ""
		m.status = fmt.Sprintf("Downloaded %d blob(s) to %s", msg.downloaded, msg.destinationRoot)
		return m, nil

	case previewWindowLoadedMsg:
		return m.handlePreviewWindowLoaded(msg)

	case tea.KeyMsg:
		key := msg.String()

		if m.helpOverlay.Active {
			switch {
			case m.keymap.ToggleHelp.Matches(key), key == "esc":
				m.helpOverlay.Close()
				return m, nil
			default:
				return m, nil
			}
		}

		if m.themeOverlay.Active {
			if m.themeOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}, m.themes) {
				m.applyTheme(m.themes[m.themeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.appName, m.themes[m.themeOverlay.ActiveThemeIdx].Name)
			}
			return m, nil
		}

		if m.preview.open && m.focus == previewPane {
			return m.handlePreviewKey(msg)
		}

		focusedFilterActive := m.focusedListSettingFilter()
		if m.focus == blobsPane && m.visualLineMode && m.keymap.FilterInput.Matches(key) {
			m.visualLineMode = false
			m.visualAnchor = ""
			m.refreshBlobItems()
			m.status = "Visual mode off"
		}
		if m.focus == blobsPane && m.visualLineMode && !focusedFilterActive && m.keymap.BlobVisualMove.Matches(key) {
			markVisualAfterListUpdate = true
		}

		switch {
		case ui.ShouldQuit(key, m.keymap.Quit, focusedFilterActive):
			return m, tea.Quit
		case m.keymap.HalfPageDown.Matches(key):
			m.scrollFocusedHalfPage(1)
			return m, nil
		case m.keymap.HalfPageUp.Matches(key):
			m.scrollFocusedHalfPage(-1)
			return m, nil
		case m.keymap.DownloadSelection.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				return m.startMarkedAction("download")
			}
		case m.keymap.ToggleLoadAll.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				return m.toggleBlobLoadAllMode()
			}
		case m.keymap.ToggleVisualLine.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				m.toggleVisualLineMode()
				return m, nil
			}
		case m.keymap.ToggleMark.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				m.toggleCurrentBlobMark()
				return m, nil
			}
		case m.keymap.ExitVisualLine.Matches(key):
			if m.focus == blobsPane && m.visualLineMode && !focusedFilterActive {
				m.visualLineMode = false
				m.visualAnchor = ""
				m.refreshBlobItems()
				m.status = "Visual mode off"
				return m, nil
			}
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
				m.loading = true
				m.lastErr = ""
				m.status = "Refreshing subscriptions..."
				return m, tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
			}
		case m.keymap.RefreshScope.Matches(key):
			if !focusedFilterActive {
				return m.refresh()
			}
		case m.keymap.OpenFocused.Matches(key):
			if focusedFilterActive {
				cmd := m.commitFocusedFilter()
				return m, cmd
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
		case m.keymap.ToggleThemePicker.Matches(key):
			if !focusedFilterActive && !m.themeOverlay.Active {
				m.themeOverlay.Open()
				return m, nil
			}
		case m.keymap.ToggleHelp.Matches(key):
			if !focusedFilterActive && !m.themeOverlay.Active {
				m.helpOverlay.Toggle()
				return m, nil
			}
		case m.keymap.BackspaceUp.Matches(key):
			if !focusedFilterActive {
				if m.focus == blobsPane && m.hasContainer && !m.blobLoadAll && m.prefix != "" {
					m.prefix = parentPrefix(m.prefix)
					m.blobSearchQuery = ""
					m.loading = true
					m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
					return m, tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
				}
			}
		}
	}

	switch m.focus {
	case subscriptionsPane:
		m.subscriptionsList, cmd = m.subscriptionsList.Update(msg)
	case accountsPane:
		m.accountsList, cmd = m.accountsList.Update(msg)
	case containersPane:
		m.containersList, cmd = m.containersList.Update(msg)
	case blobsPane:
		m.blobsList, cmd = m.blobsList.Update(msg)
	case previewPane:
		cmd = nil
	}

	if markVisualAfterListUpdate && m.focus == blobsPane && m.visualLineMode {
		m.refreshBlobItems()
		m.status = fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionBlobNames()))
	}

	return m, cmd
}
