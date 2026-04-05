package blobapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/cache"
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
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "Failed to load subscriptions"
			return m, nil
		}

		m.lastErr = ""
		m.subscriptions = msg.subscriptions

		if msg.done {
			m.loading = false
			m.cache.subscriptions.Set("", msg.subscriptions)
			m.status = fmt.Sprintf("Loaded %d subscriptions.", len(msg.subscriptions))
			if !m.hasSubscription {
				m.subOverlay.Open()
			}
		}

		return m, msg.next

	case accountsLoadedMsg:
		if msg.done && msg.err == nil {
			m.cache.accounts.Set(msg.subscriptionID, msg.accounts)
		}

		if !m.hasSubscription || m.currentSub.ID != msg.subscriptionID {
			return m, msg.next
		}

		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load storage accounts in %s", subscriptionDisplayName(m.currentSub))
			return m, nil
		}

		m.lastErr = ""
		m.accounts = msg.accounts
		m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(msg.accounts))
		ui.SetItemsPreserveIndex(&m.accountsList, accountsToItems(msg.accounts))

		if msg.done {
			m.loading = false
			m.status = fmt.Sprintf("Loaded %d storage accounts from %s.", len(msg.accounts), subscriptionDisplayName(m.currentSub))
		}

		return m, msg.next

	case containersLoadedMsg:
		if msg.done && msg.err == nil {
			m.cache.containers.Set(cache.Key(msg.account.SubscriptionID, msg.account.Name), msg.containers)
		}

		if !m.hasAccount || !sameAccount(m.currentAccount, msg.account) {
			return m, msg.next
		}

		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load containers for %s", msg.account.Name)
			return m, nil
		}

		m.lastErr = ""
		m.containers = msg.containers
		m.containersList.Title = fmt.Sprintf("Containers (%d)", len(msg.containers))
		ui.SetItemsPreserveIndex(&m.containersList, containersToItems(msg.containers))

		if msg.done {
			m.loading = false
			m.status = fmt.Sprintf("Loaded %d containers from %s.", len(msg.containers), msg.account.Name)
		}

		return m, msg.next

	case blobsLoadedMsg:
		if msg.done && msg.err == nil && msg.query == "" {
			m.cache.blobs.Set(blobsCacheKey(msg.account.SubscriptionID, msg.account.Name, msg.container, msg.prefix, msg.loadAll), msg.blobs)
		}

		// Route to search handler if search is actively fetching.
		if m.search.active && m.search.fetching && msg.query != "" {
			return m.handleSearchBlobsLoaded(msg)
		}

		if !m.hasAccount || !m.hasContainer {
			return m, msg.next
		}
		if !sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container {
			return m, msg.next
		}
		if m.prefix != msg.prefix {
			return m, msg.next
		}
		if m.blobLoadAll != msg.loadAll {
			return m, msg.next
		}
		// Non-search results should only apply when search is not active.
		if msg.query != "" && !m.search.active {
			return m, msg.next
		}

		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load blobs in %s/%s", msg.account.Name, msg.container)
			return m, nil
		}

		m.lastErr = ""
		m.blobs = msg.blobs
		m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(msg.blobs))
		m.refreshBlobItems()

		if msg.done {
			m.loading = false
			if msg.loadAll {
				m.status = fmt.Sprintf("Loaded all %d blobs in %s/%s", len(msg.blobs), msg.account.Name, msg.container)
			} else {
				m.status = fmt.Sprintf("Loaded %d entries (max %d) in %s/%s under %q", len(msg.blobs), defaultHierarchyBlobLoadLimit, msg.account.Name, msg.container, msg.prefix)
			}
		}

		return m, msg.next

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

		if m.preview.open && m.focus == previewPane {
			return m.handlePreviewKey(msg)
		}

		// Blob search pipeline.
		if m.search.active && m.focus == blobsPane {
			return m.handleSearchKey(msg)
		}

		focusedFilterActive := m.focusedListSettingFilter() || (m.focus == blobsPane && m.search.active)
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
				return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions))
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
					m.helpOverlay.Open("Azure Blob Explorer Help", m.HelpSections())
				}
				return m, nil
			}
		case m.keymap.SubscriptionPicker.Matches(key):
			if !focusedFilterActive {
				m.subOverlay.Open()
				return m, nil
			}
		case m.keymap.FilterInput.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive && m.hasContainer {
				m.activateSearch()
				return m, nil
			}
		case m.keymap.BackspaceUp.Matches(key):
			if !focusedFilterActive {
				if m.focus == blobsPane && m.hasContainer && !m.blobLoadAll && m.prefix != "" {
					m.deactivateSearch()
					m.prefix = parentPrefix(m.prefix)

					if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.currentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)); ok {
						m.blobs = cached
						m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
						m.refreshBlobItems()
					}

					m.loading = true
					m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
					return m, tea.Batch(spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
				}
			}
		}
	}

	switch m.focus {
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
