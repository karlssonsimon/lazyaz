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
			m.SubOverlay.PasteText(text, m.Subscriptions)
			return m, nil
		case m.ThemeOverlay.Active:
			m.ThemeOverlay.PasteText(text, m.Schemes)
			return m, nil
		case m.HelpOverlay.Active:
			m.HelpOverlay.PasteText(text)
			return m, nil
		case m.filter.inputOpen && m.focus == blobsPane:
			ti := ui.TextInput{Value: m.filter.prefixQuery, Cursor: m.filter.prefixCaret}
			ti.Insert(text)
			m.filter.prefixQuery = ti.Value
			m.filter.prefixCaret = ti.Cursor
			return m, nil
		case m.textInput.Active:
			m.textInput.Insert(text)
			return m, nil
		case m.actionMenu.Active:
			m.actionMenu.TypeText(text)
			return m, nil
		case m.sortOverlay.Active:
			m.sortOverlay.TypeText(text)
			return m, nil
		case m.copyOverlay.Active:
			m.copyOverlay.TypeText(text)
			return m, nil
		default:
			return m.updateFocusedList(msg)
		}
	}

	// Route all messages to the cursor so both initialBlinkMsg and
	// BlinkMsg are handled. For non-cursor messages this is a no-op.
	if cursorModel, cursorCmd := m.Cursor.Update(msg); cursorCmd != nil {
		m.Cursor = cursorModel
		// Also forward to focused list so its built-in filter cursor blinks.
		var listCmd tea.Cmd
		m, listCmd = m.updateFocusedList(msg)
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

	case moreBlobsLoadedMsg:
		return m.handleMoreBlobsLoaded(msg)

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

	case uploadDestEnteredMsg:
		// Step 2 of the upload flow: stash the typed destination and
		// open the file browser. Trim a leading slash so users typing
		// "/foo/bar" get "foo/bar" — the SDK takes the latter.
		m.uploadDest = strings.TrimPrefix(msg.dest, "/")
		return m.openUploadBrowser()

	case uploadStartedMsg:
		if m.uploadProgress != nil {
			m.uploadProgress.total = msg.fileCount
			m.uploadProgress.totalBytes = msg.totalBytes
		}
		return m, msg.next

	case uploadProgressMsg:
		if m.uploadProgress != nil {
			m.uploadProgress.uploadedBytes += msg.bytesDelta
			m.uploadProgress.currentFile = msg.currentFile
			m.updateUploadThroughput()
		}
		return m, msg.next

	case uploadConflictMsg:
		m.uploadConflict = &pendingConflict{blobName: msg.blobName, reply: msg.reply}
		if m.uploadProgress != nil {
			m.uploadProgress.waitingInput = true
			m.uploadProgress.waitingInputSince = time.Now()
		}
		return m, msg.next

	case uploadDoneMsg:
		return m.finishUpload(msg)

	case crudDoneMsg:
		m.Notify(msg.level, msg.message)
		updated, cmd := m.refresh()
		return updated, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		// Modals and overlays are keyboard-only; swallow clicks so a
		// double-click can't drill into a list underneath them.
		switch m.inputMode() {
		case ModeNormal, ModeListFilter, ModeVisualLine, ModePreview:
		default:
			return m, nil
		}
		if m.preview.open {
			region := m.previewViewportRegion()
			if m.textSelection.HandleMouseClick(msg, region) {
				return m, nil
			}
		}
		if consumed, double := m.handleMouseClick(msg); consumed {
			if double {
				return m.handleEnter()
			}
			return m, nil
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

func (m Model) handleAccountsLoaded(msg accountsLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load storage accounts in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}

	m.accounts = msg.accounts
	m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(m.accounts))
	ui.SetItemsPreserveKey(&m.accountsList, accountsToItems(m.accounts), accountItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d storage accounts from %s in %s", len(m.accounts), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		updated, navCmd := m.advancePendingNav()
		return updated, navCmd
	}

	return m, msg.next
}

func (m Model) handleContainersLoaded(msg containersLoadedMsg) (Model, tea.Cmd) {
	if !m.hasAccount || !sameAccount(m.currentAccount, msg.account) {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load containers for %s: %s", msg.account.Name, msg.err.Error()))
		return m, nil
	}

	m.containers = msg.containers
	m.containersList.Title = fmt.Sprintf("Containers (%d)", len(m.containers))
	ui.SetItemsPreserveKey(&m.containersList, containersToItems(m.containers), containerItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d containers from %s in %s", len(m.containers), msg.account.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		updated, navCmd := m.advancePendingNav()
		return updated, navCmd
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
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load blobs in %s/%s: %s", msg.account.Name, msg.container, msg.err.Error()))
		return m, nil
	}

	m.blobs = msg.blobs
	m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(m.blobs))
	m.refreshItems()

	if msg.done {
		m.blobNextMarker = msg.nextMarker
		m.blobsList.Title = m.blobsPaneTitle()
		elapsed := time.Since(m.LoadingStartedAt).Round(time.Millisecond)
		var status string
		if msg.loadAll {
			status = fmt.Sprintf("Loaded all %d blobs in %s/%s in %s", len(m.blobs), msg.account.Name, msg.container, elapsed)
		} else {
			status = fmt.Sprintf("Loaded %d entries in %s/%s under %q in %s", len(m.blobs), msg.account.Name, msg.container, displayPrefix(msg.prefix), elapsed)
		}
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		updated, navCmd := m.advancePendingNav()
		return updated, navCmd
	}

	return m, msg.next
}

// handleMoreBlobsLoaded appends a continuation page to the current view.
// Same scope guards as handleBlobsLoaded — a "load more" fired in a
// scope the user has left is dropped.
func (m Model) handleMoreBlobsLoaded(msg moreBlobsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasAccount || !m.hasContainer ||
		!sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container ||
		m.prefix != msg.prefix || m.blobLoadAll {
		return m, nil
	}

	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load more entries: %s", msg.err.Error()))
		return m, nil
	}

	// Markers continue strictly after the last delivered entry, so
	// duplicates shouldn't occur — dedup defensively anyway.
	seen := make(map[string]struct{}, len(m.blobs))
	for _, b := range m.blobs {
		seen[b.Name] = struct{}{}
	}
	added := 0
	for _, b := range msg.newBlobs {
		if _, dup := seen[b.Name]; dup {
			continue
		}
		m.blobs = append(m.blobs, b)
		added++
	}
	m.blobNextMarker = msg.nextMarker

	// Keep the broker store in sync so navigating away and back
	// rehydrates the extended list instead of snapping to the first page.
	m.cache.blobs.Set(blobsCacheKey(msg.account.SubscriptionID, msg.account.Name, msg.container, msg.prefix, false), m.blobs)

	m.blobsList.Title = m.blobsPaneTitle()
	m.refreshItems()
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Loaded %d more entries (%d total)", added, len(m.blobs)))
	return m, nil
}

// blobsPaneTitle renders the blobs pane title; a trailing "+" marks a
// truncated listing with more entries available via "Load more".
func (m Model) blobsPaneTitle() string {
	if m.blobNextMarker != "" {
		return fmt.Sprintf("Blobs (%d+)", len(m.blobs))
	}
	return fmt.Sprintf("Blobs (%d)", len(m.blobs))
}

func (m Model) handleBlobsDownloaded(msg blobsDownloadedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to download blobs: %s", msg.err.Error()))
		return m, nil
	}

	if msg.failed > 0 {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelWarn, fmt.Sprintf("Downloaded %d/%d blobs to %s — failures: %s",
			msg.downloaded, msg.total, msg.destinationRoot, strings.Join(msg.failures, " | ")))
		return m, nil
	}

	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Downloaded %d blob(s) to %s", msg.downloaded, msg.destinationRoot))
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// CRUD modals take precedence over every other handler.
	if m.confirmModal.Active {
		switch m.confirmModal.HandleKey(key) {
		case ui.ConfirmActionConfirm:
			act := m.confirmAction
			m.confirmAction = nil
			if act != nil {
				return m, act()
			}
			return m, nil
		case ui.ConfirmActionCancel:
			m.confirmAction = nil
			return m, nil
		}
		return m, nil
	}
	if m.textInput.Active {
		res := m.textInput.HandleKey(key)
		switch res.Action {
		case ui.TextInputActionSubmit:
			act := m.textInputAction
			m.textInputAction = nil
			if act != nil {
				return m, act(res.Value)
			}
			return m, nil
		case ui.TextInputActionCancel:
			m.textInputAction = nil
			return m, nil
		}
		return m, nil
	}

	if m.uploadConflict != nil {
		switch key {
		case "y":
			m.resolveConflict(conflictOverwrite)
		case "n":
			m.resolveConflict(conflictSkip)
		case "a":
			m.resolveConflict(conflictOverwriteAll)
		case "s":
			m.resolveConflict(conflictSkipAll)
		case "c", "esc":
			m.resolveConflict(conflictCancel)
			if m.uploadCancelFn != nil {
				m.uploadCancelFn()
			}
		}
		return m, nil
	}

	if m.uploadBrowserActive {
		res := m.uploadBrowser.HandleKey(key)
		switch res.Action {
		case ui.FBActionNone:
			return m, nil
		case ui.FBActionCancel:
			m.uploadBrowserActive = false
			m.uploadDest = ""
			return m, nil
		case ui.FBActionConfirm:
			m.uploadBrowserActive = false
			dest := m.uploadDest
			m.uploadDest = ""
			return m.startUpload(res.Selected, dest)
		}
		return m, nil
	}

	switch m.inputMode() {
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

	case ModeActionMenu:
		if selected, act := m.actionMenu.handleKey(key, m.Keymap); selected {
			return m.executeAction(act)
		}
		return m, nil

	case ModeSortOverlay:
		if applied, field, desc := m.sortOverlay.handleKey(key, m.Keymap); applied {
			m.blobSortField = field
			m.blobSortDesc = desc
			m.refreshItems()
			m.Notify(appshell.LevelInfo, "Sort: "+blobSortLabel(field, desc))
		}
		return m, nil

	case ModeCopyPalette:
		if target, picked := m.copyOverlay.HandleKey(key, m.Keymap); picked {
			return m.copyToClipboard(target.Value)
		}
		return m, nil

	case ModePreview:
		return m.handlePreviewKey(msg)

	case ModePrefixSearch:
		return m.handlePrefixSearchKey(msg)

	case ModeListFilter:
		return m.handleListFilterKey(msg, key)

	case ModeVisualLine:
		return m.handleVisualLineKey(msg, key)

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
		cmd := m.commitFocusedFilter()
		if m.focus == blobsPane && m.visualLineMode {
			m.refreshBlobSelectionDisplay()
		}
		return m, cmd
	}
	m2, cmd := m.updateFocusedList(msg)
	// Typing a filter while a visual range is active shifts visible
	// indices under the highlight — recompute it per keystroke.
	if m2.focus == blobsPane && m2.visualLineMode {
		m2.refreshBlobSelectionDisplay()
	}
	return m2, cmd
}

func (m Model) handleVisualLineKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
	case m.Keymap.VisualSwapAnchor.Matches(key):
		m.swapVisualAnchor()
		m.refreshBlobSelectionDisplay()
		return m, nil
	case m.Keymap.ExitVisualLine.Matches(key):
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.markedBlobs)))
		return m, nil
	case m.Keymap.DownloadSelection.Matches(key):
		return m.startDownloadMarkedBlobs()
	case m.Keymap.ToggleMark.Matches(key):
		m.toggleCurrentBlobMark()
		return m, nil
	}

	// Cursor movement falls through to the list; refresh the highlight
	// whenever the cursor actually moved so custom cursor bindings work
	// too (the old check matched only the stock movement keys).
	before := m.blobsList.Index()
	m2, cmd := m.updateFocusedList(msg)
	if m2.focus == blobsPane && m2.visualLineMode && m2.blobsList.Index() != before {
		m2.refreshBlobSelectionDisplay()
		m2.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", m2.visualRangeCount()))
	}
	return m2, cmd
}

func (m Model) handleNormalKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	// Esc peels filters like a stack on the blobs pane:
	//  1. If the bubbles list has an applied filter → clear it.
	//  2. If a prefix search is active → clear it.
	//  3. Otherwise fall through.
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

	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
	case m.Keymap.DownloadSelection.Matches(key):
		if m.focus == blobsPane {
			return m.startDownloadMarkedBlobs()
		}
	case m.Keymap.YankBlobContent.Matches(key):
		if m.focus == blobsPane {
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
		m.actionMenu.open(m.buildActions())
		return m, nil
	case m.Keymap.ToggleLoadAll.Matches(key):
		if m.focus == blobsPane {
			return m.toggleBlobLoadAllMode()
		}
	case m.Keymap.SortBlobs.Matches(key):
		if m.focus == blobsPane && m.hasContainer {
			m.sortOverlay.open(m.blobSortField, m.blobSortDesc)
			return m, nil
		}
	case m.Keymap.CopyPalette.Matches(key):
		return m.openCopyPalette()
	case m.Keymap.ToggleVisualLine.Matches(key):
		if m.focus == blobsPane {
			m.toggleVisualLineMode()
			return m, nil
		}
	case m.Keymap.ToggleMark.Matches(key):
		if m.focus == blobsPane {
			m.toggleCurrentBlobMark()
			return m, nil
		}
	case m.Keymap.ExitVisualLine.Matches(key):
		if m.focus == blobsPane && len(m.markedBlobs) > 0 {
			count := len(m.markedBlobs)
			for name := range m.markedBlobs {
				delete(m.markedBlobs, name)
			}
			m.refreshBlobSelectionDisplay()
			m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
			return m, nil
		}
	case m.Keymap.NextFocus.Matches(key):
		m.nextFocus()
		return m, nil
	case m.Keymap.PreviousFocus.Matches(key):
		m.previousFocus()
		return m, nil
	case m.Keymap.RefreshScope.Matches(key):
		return m.refresh()
	case m.Keymap.OpenFocused.Matches(key):
		return m.handleEnter()
	case m.Keymap.OpenFocusedAlt.Matches(key):
		return m.handleEnter()
	case m.Keymap.NavigateLeft.Matches(key):
		return m.navigateLeft()
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
				m.HelpOverlay.Open("Azure Blob Explorer Help", m.HelpSections())
			}
			return m, nil
		}
	case m.Keymap.SubscriptionPicker.Matches(key):
		// Standalone tabs (connection-string / Azurite) aren't tied to
		// an Azure subscription — switching subs would have no meaning.
		if m.standalone {
			return m, nil
		}
		m.SubOverlay.Open()
		m.StartLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	case m.Keymap.ReloadSubscriptions.Matches(key):
		if m.standalone {
			return m, nil
		}
		m.SubOverlay.Open()
		m.StartLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	case m.Keymap.Inspect.Matches(key):
		if m.focus != previewPane {
			m.toggleInspect()
			return m, nil
		}
	case m.Keymap.BackspaceUp.Matches(key):
		if m.focus == blobsPane && m.hasContainer && !m.blobLoadAll && m.prefix != "" {
			return m.prefixUp()
		}
		if m.focus > accountsPane {
			return m.navigateLeft()
		}
	}

	return m.updateFocusedList(msg)
}

func (m Model) updateFocusedList(msg tea.Msg) (Model, tea.Cmd) {
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
