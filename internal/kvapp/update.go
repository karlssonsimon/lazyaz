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
			m.SubOverlay.PasteText(text, m.Subscriptions)
			return m, nil
		case m.ThemeOverlay.Active:
			m.ThemeOverlay.PasteText(text, m.Schemes)
			return m, nil
		case m.HelpOverlay.Active:
			m.HelpOverlay.PasteText(text)
			return m, nil
		case m.actionMenu.Active:
			m.actionMenu.TypeText(text)
			return m, nil
		case m.copyOverlay.Active:
			m.copyOverlay.TypeText(text)
			return m, nil
		case m.createSecret.Active:
			if f := m.createSecret.FocusedField(); f != nil {
				f.Insert(text)
			}
			return m, nil
		case m.addSecretVersion.Active:
			if f := m.addSecretVersion.FocusedField(); f != nil {
				f.Insert(text)
			}
			return m, nil
		case m.createKey.Active:
			if f := m.createKey.FocusedField(); f != nil {
				f.Insert(text)
			}
			return m, nil
		case m.importCert.Active:
			if f := m.importCert.FocusedField(); f != nil {
				f.Insert(text)
			}
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

	case vaultsLoadedMsg:
		return m.handleVaultsLoaded(msg)

	case secretsLoadedMsg:
		return m.handleSecretsLoaded(msg)

	case versionsLoadedMsg:
		return m.handleVersionsLoaded(msg)

	case certsLoadedMsg:
		return m.handleCertsLoaded(msg)

	case certVersionsLoadedMsg:
		return m.handleCertVersionsLoaded(msg)

	case keysLoadedMsg:
		return m.handleKeysLoaded(msg)

	case keyVersionsLoadedMsg:
		return m.handleKeyVersionsLoaded(msg)

	case secretValueYankedMsg:
		return m.handleSecretValueYanked(msg)

	case secretRevealedMsg:
		return m.handleSecretRevealed(msg)

	case clipboardMsg:
		if msg.err != nil {
			m.Notify(appshell.LevelError, fmt.Sprintf("Clipboard: %s", msg.err.Error()))
		} else {
			m.Notify(appshell.LevelSuccess, fmt.Sprintf("Copied to clipboard: %s", msg.text))
		}
		return m, nil

	case list.FilterMatchesMsg:
		// Dropped: filtering runs synchronously on each keystroke
		// (ui.SyncFilter). These async results apply last-writer-wins,
		// so a stale keystroke's result could overwrite a newer one.
		return m, nil

	case markedSecretsYankedMsg:
		m.ClearLoading()
		if msg.err != nil {
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to yank secrets: %s", msg.err.Error()))
		} else {
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Yanked %d secrets as JSON to clipboard", msg.count))
		}
		return m, nil

	case pasteResultMsg:
		m.ClearLoading()
		summary := fmt.Sprintf("Pasted: %d created, %d new version", msg.created, msg.newVersion)
		level := appshell.LevelSuccess
		if len(msg.errors) > 0 {
			level = appshell.LevelWarn
			summary += fmt.Sprintf(", %d failed (%s)", len(msg.errors), msg.errors[0])
		}
		m.ResolveSpinner(m.LoadingSpinnerID, level, summary)
		// Refresh the secrets list so newly created entries appear.
		return m.refresh()

	case secretCreatedMsg:
		return m.handleSecretCreated(msg)

	case secretVersionAddedMsg:
		return m.handleSecretVersionAdded(msg)

	case keyCreatedMsg:
		return m.handleKeyCreated(msg)

	case certImportedMsg:
		return m.handleCertImported(msg)

	case crudDoneMsg:
		return m.handleCrudDone(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		// Modals and overlays are keyboard-only; swallow clicks so a
		// double-click can't drill into a list underneath them.
		switch m.inputMode() {
		case ModeNormal, ModeListFilter, ModeVisualLine:
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

	// Fallthrough: propagate to the focused list so filter/selection keys
	// reach the underlying bubbles list.
	var cmd tea.Cmd
	switch m.focus {
	case vaultsPane:
		m.vaultsList, cmd = m.vaultsList.Update(msg)
	case kindPane:
		m.kindList, cmd = m.kindList.Update(msg)
	case secretsPane:
		m.secretsList, cmd = m.secretsList.Update(msg)
	case versionsPane:
		m.versionsList, cmd = m.versionsList.Update(msg)
	}
	return m, cmd
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

func (m Model) handleVaultsLoaded(msg vaultsLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load key vaults in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}

	m.vaults = msg.vaults
	m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(m.vaults))
	ui.SetItemsPreserveKey(&m.vaultsList, vaultsToItems(m.vaults), vaultItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d vaults in %s", len(m.vaults), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		if m.pendingNav.hasTarget() {
			updated, cmd := m.advancePendingNav()
			m = updated
			return m, cmd
		}
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleSecretsLoaded(msg secretsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasVault || m.currentVault.Name != msg.vault.Name {
		return m, nil
	}
	// Stale: a kind cycle could land secrets results into a now-different
	// view. Drop them silently so the middle column doesn't flash.
	if m.kvKind != kvKindSecrets {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load secrets in %s: %s", msg.vault.Name, msg.err.Error()))
		return m, nil
	}

	m.secrets = msg.secrets
	m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(m.secrets))
	ui.SetItemsPreserveKey(&m.secretsList, secretsToItems(m.secrets), secretItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d secrets from %s in %s", len(m.secrets), msg.vault.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		if m.pendingNav.hasTarget() {
			updated, cmd := m.advancePendingNav()
			m = updated
			return m, cmd
		}
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleVersionsLoaded(msg versionsLoadedMsg) (Model, tea.Cmd) {
	// Explicit kind guard to match the cert/key siblings — hasSecret
	// happens to be reset on kind switches today, but the shared
	// versions list must not depend on that indirection.
	if m.kvKind != kvKindSecrets || !m.hasSecret || m.currentSecret.Name != msg.secretName {
		return m, nil
	}
	if m.currentVault.Name != msg.vault.Name {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load versions for %s: %s", msg.secretName, msg.err.Error()))
		return m, nil
	}

	m.versions = msg.versions
	m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(m.versions))
	ui.SetItemsPreserveKey(&m.versionsList, versionsToItems(m.versions), versionItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d versions for %s in %s", len(m.versions), msg.secretName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.next
}

func (m Model) handleCertsLoaded(msg certsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasVault || m.currentVault.Name != msg.vault.Name || m.kvKind != kvKindCertificates {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load certificates in %s: %s", msg.vault.Name, msg.err.Error()))
		return m, nil
	}
	m.certs = msg.certs
	m.secretsList.Title = fmt.Sprintf("Certificates (%d)", len(m.certs))
	ui.SetItemsPreserveKey(&m.secretsList, certsToItems(m.certs), certItemKey)
	if msg.done {
		status := fmt.Sprintf("Loaded %d certificates from %s in %s", len(m.certs), msg.vault.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		if m.pendingNav.hasTarget() {
			updated, cmd := m.advancePendingNav()
			m = updated
			return m, cmd
		}
		return m, nil
	}
	return m, msg.next
}

func (m Model) handleCertVersionsLoaded(msg certVersionsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasCert || m.currentCert.Name != msg.certName || m.currentVault.Name != msg.vault.Name || m.kvKind != kvKindCertificates {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load versions for %s: %s", msg.certName, msg.err.Error()))
		return m, nil
	}
	m.certVersions = msg.versions
	m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(m.certVersions))
	ui.SetItemsPreserveKey(&m.versionsList, certVersionsToItems(m.certVersions), versionItemKey)
	if msg.done {
		status := fmt.Sprintf("Loaded %d versions for %s in %s", len(m.certVersions), msg.certName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}
	return m, msg.next
}

func (m Model) handleKeysLoaded(msg keysLoadedMsg) (Model, tea.Cmd) {
	if !m.hasVault || m.currentVault.Name != msg.vault.Name || m.kvKind != kvKindKeys {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load keys in %s: %s", msg.vault.Name, msg.err.Error()))
		return m, nil
	}
	m.keys = msg.keys
	m.secretsList.Title = fmt.Sprintf("Keys (%d)", len(m.keys))
	ui.SetItemsPreserveKey(&m.secretsList, keysToItems(m.keys), keyItemKey)
	if msg.done {
		status := fmt.Sprintf("Loaded %d keys from %s in %s", len(m.keys), msg.vault.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		if m.pendingNav.hasTarget() {
			updated, cmd := m.advancePendingNav()
			m = updated
			return m, cmd
		}
		return m, nil
	}
	return m, msg.next
}

func (m Model) handleKeyVersionsLoaded(msg keyVersionsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasKey || m.currentKey.Name != msg.keyName || m.currentVault.Name != msg.vault.Name || m.kvKind != kvKindKeys {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load versions for %s: %s", msg.keyName, msg.err.Error()))
		return m, nil
	}
	m.keyVersions = msg.versions
	m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(m.keyVersions))
	ui.SetItemsPreserveKey(&m.versionsList, keyVersionsToItems(m.keyVersions), versionItemKey)
	if msg.done {
		status := fmt.Sprintf("Loaded %d versions for %s in %s", len(m.keyVersions), msg.keyName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}
	return m, msg.next
}

// handleSecretRevealed lands the fetched value into the reveal map and
// auto-opens the inspect strip on the relevant pane so the user actually
// sees it. On error we notify but don't open inspect (nothing to show).
func (m Model) handleSecretRevealed(msg secretRevealedMsg) (Model, tea.Cmd) {
	// Drop reveals that finish after the user switched vaults — otherwise
	// the old vault's plaintext lands under a name the new vault may share.
	if !m.hasVault || m.currentVault.Name != msg.vault.Name ||
		m.currentVault.SubscriptionID != msg.vault.SubscriptionID {
		return m, nil
	}
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to reveal %s: %s", msg.secretName, msg.err.Error()))
		return m, nil
	}
	if msg.version == "" {
		m.revealedSecrets[msg.secretName] = msg.value
		m.inspectPanes[secretsPane] = true
	} else {
		m.revealedVersions[revealVersionKey(msg.secretName, msg.version)] = msg.value
		m.inspectPanes[versionsPane] = true
	}
	m.resize() // inspect strip toggling on changes pane height
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelInfo, fmt.Sprintf("Revealed %s — press R again to hide", msg.secretName))
	return m, nil
}

// revealVersionKey is the map key for revealedVersions. Centralised so
// the reveal/hide/render sites can't drift on the format.
func revealVersionKey(secretName, version string) string {
	return secretName + "@" + version
}

// toggleSecretReveal flips visibility for the cursor's secret/version.
// Already-revealed → drop from the map; not yet revealed → fire the
// async fetch (handleSecretRevealed lands the result).
func (m Model) toggleSecretReveal() (Model, tea.Cmd) {
	switch m.focus {
	case secretsPane:
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}
		name := item.secret.Name
		if _, revealed := m.revealedSecrets[name]; revealed {
			delete(m.revealedSecrets, name)
			m.resize()
			return m, nil
		}
		m.StartLoading(secretsPane, fmt.Sprintf("Revealing %s...", name))
		return m, tea.Batch(m.Spinner.Tick, revealSecretValueCmd(m.service, m.currentVault, name, ""))
	case versionsPane:
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok || m.currentSecret.Name == "" {
			return m, nil
		}
		key := revealVersionKey(m.currentSecret.Name, item.version.Version)
		if _, revealed := m.revealedVersions[key]; revealed {
			delete(m.revealedVersions, key)
			m.resize()
			return m, nil
		}
		m.StartLoading(versionsPane, fmt.Sprintf("Revealing %s @ %s...", m.currentSecret.Name, item.version.Version))
		return m, tea.Batch(m.Spinner.Tick, revealSecretValueCmd(m.service, m.currentVault, m.currentSecret.Name, item.version.Version))
	}
	return m, nil
}

func (m Model) handleSecretValueYanked(msg secretValueYankedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to yank secret value: %s", msg.err.Error()))
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
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Yanked %s to clipboard", label))
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// Confirm modal takes precedence over every other handler — same as
	// blobapp. Built before the inputMode switch so it routes regardless
	// of focus / form / overlay state.
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

	switch m.inputMode() {
	case ModeForm:
		// File browser is its own ModeForm via inputMode() — route it
		// before the form overlays because it has different key semantics.
		if m.certImportBrowserActive {
			return m.handleCertImportBrowserKey(key)
		}
		return m.handleFormKey(key)

	case ModeActionMenu:
		if selected, act := m.actionMenu.handleKey(key, m.Keymap); selected {
			return m.executeAction(act)
		}
		return m, nil

	case ModePasteModal:
		return m.handlePasteModalKey(key)

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

	case ModeVisualLine:
		return m.handleVisualLineKey(msg, key)

	case ModeNormal:
		return m.handleNormalKey(msg, key)
	}

	return m, nil
}

// handleListFilterKey handles keys while the user is typing a list filter.
// Enter commits, Esc cancels, everything else goes to the bubbles list.
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

// handlePasteModalKey routes keys to the paste modal. Submit dispatches
// the paste command, cancel/esc closes the modal without writing
// anything.
func (m Model) handlePasteModalKey(key string) (Model, tea.Cmd) {
	switch {
	case m.Keymap.OpenFocused.Matches(key), m.Keymap.OpenFocusedAlt.Matches(key):
		plan := m.pasteModal.Plan()
		vault := m.currentVault
		m.pasteModal = m.pasteModal.close()
		if len(plan.Apply) == 0 {
			m.Notify(appshell.LevelInfo, "Paste cancelled (nothing to do)")
			return m, nil
		}
		m.StartLoading(m.focus, fmt.Sprintf("Pasting %d secrets...", len(plan.Apply)))
		return m, tea.Batch(m.Spinner.Tick, pasteSecretsCmd(m.service, vault, plan))
	}
	next, _ := m.pasteModal.HandleKey(key, m.Keymap)
	m.pasteModal = next
	return m, nil
}

// handleVisualLineKey handles keys during visual line selection.
func (m Model) handleVisualLineKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.scrollMotion(key):
		return m, nil
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
	case m.Keymap.VisualSwapAnchor.Matches(key):
		m.swapVisualAnchor()
		m.refreshSecretSelectionDisplay()
		return m, nil
	case m.Keymap.ExitVisualLine.Matches(key):
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshSecretSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.markedSecrets)))
		return m, nil
	}

	// Cursor movement falls through to the list; refresh the highlight
	// whenever the cursor actually moved so custom cursor bindings work
	// too (the old check matched only the stock movement keys).
	before := m.secretsList.Index()
	mdl, cmd := m.updateFocusedList(msg)
	if mdl.visualLineMode && mdl.secretsList.Index() != before {
		mdl.refreshSecretSelectionDisplay()
		mdl.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", len(mdl.visualSelectionNames())))
	}
	return mdl, cmd
}

// handleNormalKey handles keys during normal browsing.
func (m Model) handleNormalKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	// Esc peels selection state like a stack on the secrets pane:
	//  1. If the bubbles list has an applied filter → clear it.
	//  2. If marks exist → clear them.
	//  3. Otherwise fall through.
	if m.Keymap.Cancel.Matches(key) && m.focus == secretsPane {
		if m.secretsList.FilterState() != list.Unfiltered {
			m.secretsList.ResetFilter()
			return m, nil
		}
	}

	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.scrollMotion(key):
		return m, nil
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
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
	case m.Keymap.ToggleVisualLine.Matches(key):
		// Marks and visual selection operate on m.secrets — with the
		// certs/keys kind shown, exiting visual mode would repopulate the
		// column with the vault's stale secrets list.
		if m.focus == secretsPane && m.kvKind == kvKindSecrets {
			m.toggleVisualLineMode()
			return m, nil
		}
	case m.Keymap.ToggleMark.Matches(key):
		if m.focus == secretsPane && m.kvKind == kvKindSecrets {
			m.toggleCurrentSecretMark()
			return m, nil
		}
	case m.Keymap.ExitVisualLine.Matches(key):
		if m.focus == secretsPane && len(m.markedSecrets) > 0 {
			count := len(m.markedSecrets)
			for name := range m.markedSecrets {
				delete(m.markedSecrets, name)
			}
			m.refreshSecretSelectionDisplay()
			m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
			return m, nil
		}
	case m.Keymap.ActionMenu.Matches(key):
		m.actionMenu.open(m.buildActions())
		return m, nil
	case m.Keymap.YankSecret.Matches(key):
		return m.handleYank()
	case m.Keymap.CopyPalette.Matches(key):
		return m.openCopyPalette()
	case m.Keymap.PasteSecrets.Matches(key):
		if m.focus == secretsPane && m.hasVault && m.kvKind == kvKindSecrets {
			return m.tryOpenPasteModal()
		}
	case m.Keymap.RevealSecret.Matches(key):
		return m.toggleSecretReveal()
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
				m.HelpOverlay.Open("Azure Key Vault Explorer Help", m.HelpSections())
			}
			return m, nil
		}
	case m.Keymap.SubscriptionPicker.Matches(key):
		m.SubOverlay.Open()
		m.StartLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	case m.Keymap.ReloadSubscriptions.Matches(key):
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

// updateFocusedList forwards a message to the currently focused list
// and returns the updated model.
func (m Model) updateFocusedList(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case vaultsPane:
		cmd = ui.UpdateListSyncFilter(&m.vaultsList, msg)
	case kindPane:
		cmd = ui.UpdateListSyncFilter(&m.kindList, msg)
	case secretsPane:
		cmd = ui.UpdateListSyncFilter(&m.secretsList, msg)
	case versionsPane:
		cmd = ui.UpdateListSyncFilter(&m.versionsList, msg)
	}
	return m, cmd
}

func (m Model) handleYank() (Model, tea.Cmd) {
	if m.focus == secretsPane {
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}
		m.StartLoading(m.focus, fmt.Sprintf("Fetching secret value for %s...", item.secret.Name))
		return m, tea.Batch(m.Spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, item.secret.Name, ""))
	}

	if m.focus == versionsPane {
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok {
			return m, nil
		}
		m.StartLoading(m.focus, fmt.Sprintf("Fetching secret value for %s@%s...", m.currentSecret.Name, item.version.Version))
		return m, tea.Batch(m.Spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, m.currentSecret.Name, item.version.Version))
	}

	return m, nil
}
