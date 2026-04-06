package blobapp

import (
	"fmt"
	"strings"
	"time"

	"azure-storage/internal/appshell"
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
		m.Width = msg.Width
		m.Height = msg.Height
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.Loading {
			return m, nil
		}
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case appshell.LoadingHoldExpiredMsg:
		m.ClearLoading()
		m.Status = msg.Status
		return m, nil

	case appshell.SubscriptionsLoadedMsg:
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

	case accountsLoadedMsg:
		if msg.done && msg.err == nil {
			m.cache.accounts.Set(msg.subscriptionID, msg.accounts)
		}

		if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
			return m, msg.next
		}

		if msg.err != nil {
			m.ClearLoading()
			m.LastErr = msg.err.Error()
			m.Status = fmt.Sprintf("Failed to load storage accounts in %s", ui.SubscriptionDisplayName(m.CurrentSub))
			return m, nil
		}

		m.LastErr = ""
		m.accounts = msg.accounts
		m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(msg.accounts))
		ui.SetItemsPreserveIndex(&m.accountsList, accountsToItems(msg.accounts))

		if msg.done {
			status := fmt.Sprintf("Loaded %d storage accounts from %s in %s", len(msg.accounts), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
			return m, m.FinishLoading(status)
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
			m.ClearLoading()
			m.LastErr = msg.err.Error()
			m.Status = fmt.Sprintf("Failed to load containers for %s", msg.account.Name)
			return m, nil
		}

		m.LastErr = ""
		m.containers = msg.containers
		m.containersList.Title = fmt.Sprintf("Containers (%d)", len(msg.containers))
		ui.SetItemsPreserveIndex(&m.containersList, containersToItems(msg.containers))

		if msg.done {
			status := fmt.Sprintf("Loaded %d containers from %s in %s", len(msg.containers), msg.account.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
			return m, m.FinishLoading(status)
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
			m.ClearLoading()
			m.LastErr = msg.err.Error()
			m.Status = fmt.Sprintf("Failed to load blobs in %s/%s", msg.account.Name, msg.container)
			return m, nil
		}

		m.LastErr = ""
		m.blobs = msg.blobs
		m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(msg.blobs))
		m.refreshItems()

		if msg.done {
			elapsed := time.Since(m.LoadingStartedAt).Round(time.Millisecond)
			var status string
			if msg.loadAll {
				status = fmt.Sprintf("Loaded all %d blobs in %s/%s in %s", len(msg.blobs), msg.account.Name, msg.container, elapsed)
			} else {
				status = fmt.Sprintf("Loaded %d entries in %s/%s under %q in %s", len(msg.blobs), msg.account.Name, msg.container, msg.prefix, elapsed)
			}
			return m, m.FinishLoading(status)
		}

		return m, msg.next

	case blobsDownloadedMsg:
		m.ClearLoading()
		if msg.err != nil {
			m.LastErr = msg.err.Error()
			m.Status = "Failed to download blobs"
			return m, nil
		}

		if msg.failed > 0 {
			m.LastErr = strings.Join(msg.failures, " | ")
			m.Status = fmt.Sprintf("Downloaded %d/%d blobs to %s", msg.downloaded, msg.total, msg.destinationRoot)
			return m, nil
		}

		m.LastErr = ""
		m.Status = fmt.Sprintf("Downloaded %d blob(s) to %s", msg.downloaded, msg.destinationRoot)
		return m, nil

	case previewWindowLoadedMsg:
		return m.handlePreviewWindowLoaded(msg)

	case tea.KeyMsg:
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

		if m.preview.open && m.focus == previewPane {
			return m.handlePreviewKey(msg)
		}

		// Blob search pipeline.
		if m.search.active && m.focus == blobsPane {
			return m.handleSearchKey(msg)
		}

		focusedFilterActive := m.focusedListSettingFilter() || (m.focus == blobsPane && m.search.active)
		if m.focus == blobsPane && m.visualLineMode && m.Keymap.FilterInput.Matches(key) {
			m.visualLineMode = false
			m.visualAnchor = ""
			m.refreshItems()
			m.Status = "Visual mode off"
		}
		if m.focus == blobsPane && m.visualLineMode && !focusedFilterActive && m.Keymap.BlobVisualMove.Matches(key) {
			markVisualAfterListUpdate = true
		}

		switch {
		case ui.ShouldQuit(key, m.Keymap.Quit, focusedFilterActive):
			return m, tea.Quit
		case m.Keymap.HalfPageDown.Matches(key):
			m.scrollFocusedHalfPage(1)
			return m, nil
		case m.Keymap.HalfPageUp.Matches(key):
			m.scrollFocusedHalfPage(-1)
			return m, nil
		case m.Keymap.DownloadSelection.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				return m.startMarkedAction("download")
			}
		case m.Keymap.ToggleLoadAll.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				return m.toggleBlobLoadAllMode()
			}
		case m.Keymap.ToggleVisualLine.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				m.toggleVisualLineMode()
				return m, nil
			}
		case m.Keymap.ToggleMark.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				m.toggleCurrentBlobMark()
				return m, nil
			}
		case m.Keymap.VisualSwapAnchor.Matches(key):
			if m.focus == blobsPane && m.visualLineMode && !focusedFilterActive {
				m.swapVisualAnchor()
				m.refreshItems()
				return m, nil
			}
		case m.Keymap.ExitVisualLine.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive {
				if m.visualLineMode {
					// Commit visual range into marks, then exit visual mode.
					m.commitVisualSelection()
					m.visualLineMode = false
					m.visualAnchor = ""
					m.refreshItems()
					m.Status = fmt.Sprintf("Visual mode off. %d marked.", len(m.markedBlobs))
					return m, nil
				}
				if len(m.markedBlobs) > 0 {
					// Clear all marks.
					count := len(m.markedBlobs)
					for name := range m.markedBlobs {
						delete(m.markedBlobs, name)
					}
					m.refreshItems()
					m.Status = fmt.Sprintf("Cleared %d marks", count)
					return m, nil
				}
			}
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
				cmd := m.commitFocusedFilter()
				return m, cmd
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
					m.HelpOverlay.Open("Azure Blob Explorer Help", m.HelpSections())
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
		case m.Keymap.FilterInput.Matches(key):
			if m.focus == blobsPane && !focusedFilterActive && m.hasContainer {
				m.activateSearch()
				return m, nil
			}
		case m.Keymap.Inspect.Matches(key):
			if !focusedFilterActive {
				m.inspectFocusedItem()
				return m, nil
			}
		case m.Keymap.BackspaceUp.Matches(key):
			if !focusedFilterActive {
				if m.focus == blobsPane && m.hasContainer && !m.blobLoadAll && m.prefix != "" {
					m.deactivateSearch()
					m.prefix = parentPrefix(m.prefix)

					if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)); ok {
						m.blobs = cached
						m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
						m.refreshItems()
					}

					m.SetLoading(blobsPane)
					m.Status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
					return m, tea.Batch(spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, false))
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
		m.refreshItems()
		m.Status = fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionBlobNames()))
	}

	return m, cmd
}
