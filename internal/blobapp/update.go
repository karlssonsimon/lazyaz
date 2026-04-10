package blobapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle paste before anything else so it reaches the right input.
	// The cursor update below can swallow PasteMsg before the main
	// switch sees it, which is why ctrl+v works in the bubbles list
	// filter (textinput handles it) but not in our custom overlays.
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
		case m.filter.inputOpen && m.focus == blobsPane:
			m.filter.prefixQuery += text
			return m, nil
		case m.actionMenu.active:
			m.actionMenu.query += text
			m.actionMenu.refilter()
			return m, nil
		case m.sortOverlay.active:
			m.sortOverlay.query += text
			m.sortOverlay.refilter()
			return m, nil
		default:
			var cmd tea.Cmd
			switch m.focus {
			case accountsPane:
				m.accountsList, cmd = m.accountsList.Update(msg)
			case containersPane:
				m.containersList, cmd = m.containersList.Update(msg)
			case blobsPane:
				m.blobsList, cmd = m.blobsList.Update(msg)
			}
			return m, cmd
		}
	}

	// Route all messages to the cursor so both initialBlinkMsg and
	// BlinkMsg are handled. For non-cursor messages this is a no-op.
	if cursorModel, cursorCmd := m.Cursor.Update(msg); cursorCmd != nil {
		m.Cursor = cursorModel
		// Also forward to focused list so its built-in filter cursor blinks.
		var listCmd tea.Cmd
		switch m.focus {
		case accountsPane:
			m.accountsList, listCmd = m.accountsList.Update(msg)
		case containersPane:
			m.containersList, listCmd = m.containersList.Update(msg)
		case blobsPane:
			m.blobsList, listCmd = m.blobsList.Update(msg)
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

	case blobContentClipboardMsg:
		if msg.err != nil {
			m.Notify(appshell.LevelError, fmt.Sprintf("Failed to download %s: %s", msg.blobName, msg.err.Error()))
			return m, nil
		}
		return m.copyToClipboard(msg.content)

	case clipboardMsg:
		if msg.err != nil {
			m.Notify(appshell.LevelError, fmt.Sprintf("Clipboard: %s", msg.err.Error()))
		} else {
			m.Notify(appshell.LevelSuccess, fmt.Sprintf("Copied to clipboard: %s", ui.TrimToWidth(msg.text, 60)))
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		if m.preview.open {
			region := m.previewViewportRegion()
			if m.textSelection.HandleMouseClick(msg, region) {
				return m, nil
			}
		}

	case tea.MouseMotionMsg:
		if m.textSelection.Active {
			region := m.previewViewportRegion()
			m.textSelection.HandleMouseMotion(msg, region)
			return m, nil
		}

	case tea.MouseReleaseMsg:
		if m.textSelection.Active {
			region := m.previewViewportRegion()
			text, ok := m.textSelection.HandleMouseRelease(msg, m.preview.viewport, region)
			if ok {
				return m, func() tea.Msg {
					if err := ui.WriteClipboard(text); err != nil {
						return clipboardMsg{err: err}
					}
					return clipboardMsg{text: text}
				}
			}
			return m, nil
		}
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
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load subscriptions: %s", msg.Err.Error()))
		return m, nil
	}


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

func (m Model) handleAccountsLoaded(msg accountsLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load storage accounts in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}


	m.accounts = msg.accounts
	m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(m.accounts))
	ui.SetItemsPreserveKey(&m.accountsList, accountsToItems(m.accounts), accountItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d storage accounts from %s in %s", len(m.accounts), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleContainersLoaded(msg containersLoadedMsg) (Model, tea.Cmd) {
	if !m.hasAccount || !sameAccount(m.currentAccount, msg.account) {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load containers for %s: %s", msg.account.Name, msg.err.Error()))
		return m, nil
	}


	m.containers = msg.containers
	m.containersList.Title = fmt.Sprintf("Containers (%d)", len(m.containers))
	ui.SetItemsPreserveKey(&m.containersList, containersToItems(m.containers), containerItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d containers from %s in %s", len(m.containers), msg.account.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
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
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load blobs in %s/%s: %s", msg.account.Name, msg.container, msg.err.Error()))
		return m, nil
	}


	m.blobs = msg.blobs
	m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(m.blobs))
	m.refreshItems()

	if msg.done {
		elapsed := time.Since(m.LoadingStartedAt).Round(time.Millisecond)
		var status string
		if msg.loadAll {
			status = fmt.Sprintf("Loaded all %d blobs in %s/%s in %s", len(m.blobs), msg.account.Name, msg.container, elapsed)
		} else {
			status = fmt.Sprintf("Loaded %d entries in %s/%s under %q in %s", len(m.blobs), msg.account.Name, msg.container, displayPrefix(msg.prefix), elapsed)
		}
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleBlobsDownloaded(msg blobsDownloadedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to download blobs: %s", msg.err.Error()))
		return m, nil
	}

	if msg.failed > 0 {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelWarn, fmt.Sprintf("Downloaded %d/%d blobs to %s — failures: %s",
			msg.downloaded, msg.total, msg.destinationRoot, strings.Join(msg.failures, " | ")))
		return m, nil
	}

	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Downloaded %d blob(s) to %s", msg.downloaded, msg.destinationRoot))
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

	if m.actionMenu.active {
		if selected, act := m.actionMenu.handleKey(key, m.Keymap); selected {
			return m.executeAction(act)
		}
		return m, nil
	}

	if m.sortOverlay.active {
		if applied, field, desc := m.sortOverlay.handleKey(key, m.Keymap); applied {
			m.blobSortField = field
			m.blobSortDesc = desc
			m.refreshItems()
			m.Notify(appshell.LevelInfo, "Sort: "+blobSortLabel(field, desc))
		}
		return m, nil
	}

	if m.preview.open && m.focus == previewPane {
		return m.handlePreviewKey(msg)
	}

	// Server prefix search overlay.
	if m.filter.inputOpen && m.focus == blobsPane {
		return m.handlePrefixSearchKey(msg)
	}

	// Esc peels filters like a stack:
	//  1. If the bubbles list has an applied filter → clear it and return.
	//  2. If no list filter but a prefix search is active → clear it here.
	//  3. Otherwise fall through to normal esc handlers (visual mode, etc).
	if m.Keymap.Cancel.Matches(key) && m.focus == blobsPane {
		if m.blobsList.FilterState() != list.Unfiltered {
			m.blobsList.ResetFilter()
			return m, nil
		}
		if m.hasActiveFilter() {
			m.clearFilter()
			m.Notify(appshell.LevelInfo, "Prefix filter cleared")
			return m, nil
		}
	}

	focusedFilterActive := m.focusedListSettingFilter() || (m.focus == blobsPane && m.filter.inputOpen)

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
	case m.Keymap.YankBlobContent.Matches(key):
		if m.focus == blobsPane && !focusedFilterActive {
			if item, ok := m.blobsList.SelectedItem().(blobItem); ok && !item.blob.IsPrefix {
				if item.blob.Size == 0 || item.blob.Size >= 5*1024*1024 {
					m.Notify(appshell.LevelError, "Blob too large to yank (must be < 5 MB)")
					return m, nil
				}
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Downloading %s...", item.blob.Name))
				return m, downloadBlobToClipboardCmd(m.service, m.currentAccount, m.containerName, item.blob.Name, item.blob.Size)
			}
		}
	case m.Keymap.ActionMenu.Matches(key):
		if m.focus == blobsPane && !focusedFilterActive && m.hasContainer {
			m.actionMenu.open(m.buildActions())
			return m, nil
		}
	case m.Keymap.ToggleLoadAll.Matches(key):
		if m.focus == blobsPane && !focusedFilterActive {
			return m.toggleBlobLoadAllMode()
		}
	case m.Keymap.SortBlobs.Matches(key):
		if m.focus == blobsPane && !focusedFilterActive && m.hasContainer {
			m.sortOverlay.open(m.blobSortField, m.blobSortDesc)
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
			m.refreshBlobSelectionDisplay()
			return m, nil
		}
	case m.Keymap.ExitVisualLine.Matches(key):
		if m.focus == blobsPane && !focusedFilterActive {
			if m.visualLineMode {
				// Commit visual range into marks, then exit visual mode.
				m.commitVisualSelection()
				m.visualLineMode = false
				m.visualAnchor = ""
				m.refreshBlobSelectionDisplay()
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.markedBlobs)))
				return m, nil
			}
			if len(m.markedBlobs) > 0 {
				// Clear all marks.
				count := len(m.markedBlobs)
				for name := range m.markedBlobs {
					delete(m.markedBlobs, name)
				}
				m.refreshBlobSelectionDisplay()
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
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
		
			m.loadingSpinnerID = m.NotifySpinner("Refreshing subscriptions...")
			return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
		}
	// FilterInput for the blobs pane is handled by the bubbles list's
	// built-in filter (SetFilteringEnabled=true). No explicit handler
	// needed — the keypress falls through to the list's Update below.
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

				m.rebuildParentBlobsList()

				m.SetLoading(blobsPane)
				m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, displayPrefix(m.prefix)))
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
		m.refreshBlobSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionBlobNames())))
	}

	return m, cmd
}
