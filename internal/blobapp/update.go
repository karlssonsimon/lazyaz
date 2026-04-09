package blobapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
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

	case accountsLoadedMsg:
		return m.handleAccountsLoaded(msg)

	case containersLoadedMsg:
		return m.handleContainersLoaded(msg)

	case blobsLoadedMsg:
		return m.handleBlobsLoaded(msg)

	case blobsDownloadedMsg:
		return m.handleBlobsDownloaded(msg)

	case previewWindowLoadedMsg:
		return m.handlePreviewWindowLoaded(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Fallthrough: propagate to focused list.
	var cmd tea.Cmd
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
	return m, cmd
}

func (m Model) handleSubscriptionsLoaded(msg appshell.SubscriptionsLoadedMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.ClearLoading()
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load subscriptions: %s", msg.Err.Error()))
		return m, nil
	}

	m.LastErr = ""
	m.Subscriptions = msg.Subscriptions
	// Keep the overlay's filtered view in sync with streaming results
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
				return next, tea.Batch(selectCmd, next.FinishLoading(status))
			}
			m.SubOverlay.Open()
		}
		return m, m.FinishLoading(status)
	}

	return m, msg.Next
}

func (m Model) handleAccountsLoaded(msg accountsLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load storage accounts in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}

	m.LastErr = ""
	m.accounts = msg.accounts
	m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(m.accounts))
	ui.SetItemsPreserveKey(&m.accountsList, accountsToItems(m.accounts), accountItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d storage accounts from %s in %s", len(m.accounts), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleContainersLoaded(msg containersLoadedMsg) (Model, tea.Cmd) {
	if !m.hasAccount || !sameAccount(m.currentAccount, msg.account) {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load containers for %s: %s", msg.account.Name, msg.err.Error()))
		return m, nil
	}

	m.LastErr = ""
	m.containers = msg.containers
	m.containersList.Title = fmt.Sprintf("Containers (%d)", len(m.containers))
	ui.SetItemsPreserveKey(&m.containersList, containersToItems(m.containers), containerItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d containers from %s in %s", len(m.containers), msg.account.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleBlobsLoaded(msg blobsLoadedMsg) (Model, tea.Cmd) {
	// Filter prefix-search results go through handleFilterBlobsLoaded.
	if m.filter.fetching && msg.query != "" {
		return m.handleFilterBlobsLoaded(msg)
	}

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
	// Results with a query set are filter results — if they weren't
	// handled above, they're stale and should be dropped.
	if msg.query != "" {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load blobs in %s/%s: %s", msg.account.Name, msg.container, msg.err.Error()))
		return m, nil
	}

	m.LastErr = ""
	m.blobs = msg.blobs
	m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(m.blobs))
	m.refreshItems()

	if msg.done {
		elapsed := time.Since(m.LoadingStartedAt).Round(time.Millisecond)
		var status string
		if msg.loadAll {
			status = fmt.Sprintf("Loaded all %d blobs in %s/%s in %s", len(m.blobs), msg.account.Name, msg.container, elapsed)
		} else {
			status = fmt.Sprintf("Loaded %d entries in %s/%s under %q in %s", len(m.blobs), msg.account.Name, msg.container, msg.prefix, elapsed)
		}
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleBlobsDownloaded(msg blobsDownloadedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to download blobs: %s", msg.err.Error()))
		return m, nil
	}

	if msg.failed > 0 {
		m.Notify(appshell.LevelWarn, fmt.Sprintf("Downloaded %d/%d blobs to %s — failures: %s",
			msg.downloaded, msg.total, msg.destinationRoot, strings.Join(msg.failures, " | ")))
		return m, nil
	}

	m.Notify(appshell.LevelSuccess, fmt.Sprintf("Downloaded %d blob(s) to %s", msg.downloaded, msg.destinationRoot))
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

	if m.preview.open && m.focus == previewPane {
		return m.handlePreviewKey(msg)
	}

	// Filter input pipeline.
	if m.filter.inputOpen && m.focus == blobsPane {
		return m.handleFilterKey(msg)
	}

	// Esc on the blob pane clears an active filter when input is closed.
	if m.Keymap.Cancel.Matches(key) && m.focus == blobsPane && m.hasActiveFilter() {
		m.clearFilter()
		m.Status = "Filter cleared"
		return m, nil
	}

	focusedFilterActive := m.focusedListSettingFilter() || (m.focus == blobsPane && m.filter.inputOpen)

	// Pressing the filter-input key in visual mode on the blobs pane exits
	// visual mode. (The search pipeline above catches the active case.)
	if m.focus == blobsPane && m.visualLineMode && m.Keymap.FilterInput.Matches(key) {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshItems()
		m.Status = "Visual mode off"
	}

	// If this is a visual-range move key, remember to refresh the visual
	// selection status after the list update at the bottom of this function.
	markVisualAfterListUpdate := m.focus == blobsPane && m.visualLineMode && !focusedFilterActive && m.Keymap.BlobVisualMove.Matches(key)

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
	case m.Keymap.SortBlobs.Matches(key):
		if m.focus == blobsPane && !focusedFilterActive && m.hasContainer {
			m.cycleBlobSort()
			return m, nil
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
			return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
		}
	case m.Keymap.FilterInput.Matches(key):
		if m.focus == blobsPane && !focusedFilterActive && m.hasContainer {
			cmd := m.openFilterInput()
			return m, cmd
		}
	case m.Keymap.Inspect.Matches(key):
		if !focusedFilterActive && m.focus != previewPane {
			m.toggleInspect()
			return m, nil
		}
	case m.Keymap.BackspaceUp.Matches(key):
		if !focusedFilterActive {
			if m.focus == blobsPane && m.hasContainer && !m.blobLoadAll && m.prefix != "" {
				// Snapshot current prefix's blobs list before going up.
				oldKey := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)
				m.blobsHistory[oldKey] = ui.SnapshotListState(&m.blobsList, blobItemKey)

				m.clearFilter()
				m.prefix = parentPrefix(m.prefix)

				blobsScope := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)
				if cached, ok := m.cache.blobs.Get(blobsScope); ok {
					m.blobs = cached
					m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
					m.refreshItems()
				}
				ui.RestoreListState(&m.blobsList, m.blobsHistory[blobsScope], blobItemKey)

				m.SetLoading(blobsPane)
				m.Status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
				return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
			}
		}
	}

	// Key didn't match any app-specific handler — fall through to the
	// focused list so filter input and cursor keys reach it.
	var cmd tea.Cmd
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
