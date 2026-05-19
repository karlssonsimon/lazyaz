package kvapp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

var testConfig = ui.Config{
	ThemeName: "fallback",
	Schemes:   []ui.Scheme{ui.FallbackScheme()},
}

// TestValidateKeyAlgorithmAcceptsAllSixAndRejectsOthers locks the
// allowed-values list. Case-insensitive on the prefix; trim whitespace.
func TestValidateKeyAlgorithmAcceptsAllSixAndRejectsOthers(t *testing.T) {
	good := []string{"RSA-2048", "RSA-3072", "RSA-4096", "EC-P256", "EC-P384", "EC-P521", "rsa-2048", "  EC-P256  "}
	for _, v := range good {
		if msg := validateKeyAlgorithm(v); msg != "" {
			t.Fatalf("validateKeyAlgorithm(%q) = %q, want empty", v, msg)
		}
	}
	bad := map[string]string{
		"":           "algorithm is required",
		"RSA-1024":   "expected RSA-2048/3072/4096 or EC-P256/P384/P521",
		"DSA-1024":   "expected RSA-2048/3072/4096 or EC-P256/P384/P521",
		"EC-P25":     "expected RSA-2048/3072/4096 or EC-P256/P384/P521",
		"RSA":        "expected RSA-2048/3072/4096 or EC-P256/P384/P521",
		"RSA 2048":   "expected RSA-2048/3072/4096 or EC-P256/P384/P521",
	}
	for v, want := range bad {
		if got := validateKeyAlgorithm(v); got != want {
			t.Fatalf("validateKeyAlgorithm(%q) = %q, want %q", v, got, want)
		}
	}
}

// TestParseKeyAlgorithmMapsToSDKShape ensures the output matches what
// CreateKey expects: kty + size for RSA, kty + curve for EC.
func TestParseKeyAlgorithmMapsToSDKShape(t *testing.T) {
	cases := []struct {
		in        string
		wantKty   string
		wantSize  int32
		wantCurve string
	}{
		{"RSA-2048", "RSA", 2048, ""},
		{"RSA-3072", "RSA", 3072, ""},
		{"RSA-4096", "RSA", 4096, ""},
		{"EC-P256", "EC", 0, "P-256"},
		{"EC-P384", "EC", 0, "P-384"},
		{"EC-P521", "EC", 0, "P-521"},
		{"  rsa-3072  ", "RSA", 3072, ""}, // whitespace + lowercase tolerated
		{"unknown", "RSA", 2048, ""},      // fallback to default
	}
	for _, tc := range cases {
		gotKty, gotSize, gotCurve := parseKeyAlgorithm(tc.in)
		if gotKty != tc.wantKty || gotSize != tc.wantSize || gotCurve != tc.wantCurve {
			t.Fatalf("parseKeyAlgorithm(%q) = (%s, %d, %s), want (%s, %d, %s)",
				tc.in, gotKty, gotSize, gotCurve, tc.wantKty, tc.wantSize, tc.wantCurve)
		}
	}
}

// TestCreateActionsFollowKvKind confirms the per-kind create entries
// only appear on the matching kind. Secrets sees "Create secret...",
// certs sees "Import certificate...", keys sees "Create key..."; none
// of them surface across kinds.
func TestCreateActionsFollowKvKind(t *testing.T) {
	cases := []struct {
		kind  kvKind
		want  string
		other []string
	}{
		{kvKindSecrets, "Create secret...", []string{"Create key...", "Import certificate..."}},
		{kvKindCertificates, "Import certificate...", []string{"Create secret...", "Create key..."}},
		{kvKindKeys, "Create key...", []string{"Create secret...", "Import certificate..."}},
	}
	for _, tc := range cases {
		t.Run(tc.kind.String(), func(t *testing.T) {
			m := NewModel(nil, testConfig, nil)
			m.SubOverlay.Close()
			m.hasVault = true
			m.focus = secretsPane
			m.kvKind = tc.kind

			labels := make(map[string]bool)
			for _, a := range m.buildActions() {
				labels[a.label] = true
			}
			if !labels[tc.want] {
				t.Fatalf("kind %s: missing %q in action menu", tc.kind, tc.want)
			}
			for _, leaked := range tc.other {
				if labels[leaked] {
					t.Fatalf("kind %s: %q leaked from another kind", tc.kind, leaked)
				}
			}
		})
	}
}

// TestDeleteActionFollowsCursorAndKvKind locks the per-kind delete
// entries: only the matching kind sees its delete row, only when an
// item is actually selected, and never across kinds.
func TestDeleteActionFollowsCursorAndKvKind(t *testing.T) {
	cases := []struct {
		kind       kvKind
		seedItems  []list.Item
		wantLabel  string
		otherLabels []string
	}{
		{
			kind:       kvKindSecrets,
			seedItems:  []list.Item{secretItem{secret: keyvault.Secret{Name: "api-key"}}},
			wantLabel:  "Delete secret (api-key)...",
			otherLabels: []string{"Delete certificate", "Delete key"},
		},
		{
			kind:       kvKindCertificates,
			seedItems:  []list.Item{certItem{cert: keyvault.Certificate{Name: "tls-cert"}}},
			wantLabel:  "Delete certificate (tls-cert)...",
			otherLabels: []string{"Delete secret", "Delete key"},
		},
		{
			kind:       kvKindKeys,
			seedItems:  []list.Item{keyItem{key: keyvault.Key{Name: "signing"}}},
			wantLabel:  "Delete key (signing)...",
			otherLabels: []string{"Delete secret", "Delete certificate"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.kind.String(), func(t *testing.T) {
			m := NewModel(nil, testConfig, nil)
			m.SubOverlay.Close()
			m.hasVault = true
			m.focus = secretsPane
			m.kvKind = tc.kind
			m.secretsList.SetItems(tc.seedItems)

			labels := make(map[string]bool)
			for _, a := range m.buildActions() {
				labels[a.label] = true
			}
			if !labels[tc.wantLabel] {
				t.Fatalf("kind %s: missing %q in action menu", tc.kind, tc.wantLabel)
			}
			for label := range labels {
				for _, other := range tc.otherLabels {
					if strings.HasPrefix(label, other) {
						t.Fatalf("kind %s: %q leaked from another kind", tc.kind, label)
					}
				}
			}
		})
	}
}

// TestDeleteActionAbsentWithoutSelection confirms the delete entry
// only surfaces when there's actually an item to delete — empty list
// or wrong-kind cursor must not produce a stray Delete row.
func TestDeleteActionAbsentWithoutSelection(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasVault = true
	m.focus = secretsPane
	m.kvKind = kvKindSecrets
	// Empty items list — no selection possible.
	m.secretsList.SetItems(nil)

	for _, a := range m.buildActions() {
		if strings.HasPrefix(a.label, "Delete ") {
			t.Fatalf("delete entry %q should not surface with empty list", a.label)
		}
	}
}

// TestKindPaneSelectionDrivesKvKindAndFocus locks in the new
// vaults → kind → items → versions hierarchy. Selecting a row in the
// kindPane sets m.kvKind to that row's kind and moves focus to the
// items pane (where Enter would then drill into a specific item).
func TestKindPaneSelectionDrivesKvKindAndFocus(t *testing.T) {
	cases := []struct {
		row  int
		want kvKind
	}{
		{row: 0, want: kvKindSecrets},
		{row: 1, want: kvKindCertificates},
		{row: 2, want: kvKindKeys},
	}
	for _, tc := range cases {
		t.Run(tc.want.String(), func(t *testing.T) {
			m := NewModel(nil, testConfig, nil)
			m.SubOverlay.Close()
			m.HasSubscription = true
			m.hasVault = true
			m.currentVault.Name = "v"
			m.focus = kindPane
			m.kindList.Select(tc.row)
			// Force a non-secrets starting kind for the secrets case so
			// the assertion catches "did anything change?".
			if tc.want == kvKindSecrets {
				m.kvKind = kvKindKeys
			}

			updated, _ := m.handleEnter()
			if updated.kvKind != tc.want {
				t.Fatalf("kvKind = %v, want %v", updated.kvKind, tc.want)
			}
			if updated.focus != secretsPane {
				t.Fatalf("focus = %d, want secretsPane (%d)", updated.focus, secretsPane)
			}
		})
	}
}

// TestNavigateLeftWalksThroughKindPane confirms the four-column back
// path: versions → items → kind → vaults. Each step regresses one
// pane.
func TestNavigateLeftWalksThroughKindPane(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.focus = versionsPane

	for _, want := range []int{secretsPane, kindPane, vaultsPane} {
		updated, _ := m.navigateLeft()
		m = updated
		if m.focus != want {
			t.Fatalf("after navigateLeft: focus = %d, want %d", m.focus, want)
		}
	}
}

// TestActionMenuHidesSecretActionsOnCertKeyKinds confirms the per-kind
// action filtering: secret-only entries (yank/reveal/mark/visual line)
// shouldn't surface when the middle column is showing certs or keys.
func TestActionMenuHidesSecretActionsOnCertKeyKinds(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasVault = true
	m.focus = secretsPane

	for _, kind := range []kvKind{kvKindCertificates, kvKindKeys} {
		t.Run(kind.String(), func(t *testing.T) {
			m.kvKind = kind
			labels := make(map[string]bool)
			for _, a := range m.buildActions() {
				labels[a.label] = true
			}
			for _, hidden := range []string{"Yank secret name", "Yank secret value", "Reveal secret value", "Toggle mark", "Create secret..."} {
				if labels[hidden] {
					t.Fatalf("kind %s: %q should not appear in action menu", kind, hidden)
				}
			}
		})
	}
}

// TestSecretRevealHideRoundTrip exercises the toggle on the reveal map
// directly (no Azure call): once a value lands in revealedSecrets the
// inspect strip renders it; toggling again drops it back to a mask.
// Also confirms revealing a secret auto-opens its pane's inspect strip.
func TestSecretRevealHideRoundTrip(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	if _, masked := m.revealedSecrets["api-key"]; masked {
		t.Fatal("zero-value model should have nothing revealed")
	}

	updated, _ := m.handleSecretRevealed(secretRevealedMsg{secretName: "api-key", value: "s3cret"})
	if updated.revealedSecrets["api-key"] != "s3cret" {
		t.Fatalf("revealedSecrets[api-key] = %q, want s3cret", updated.revealedSecrets["api-key"])
	}
	if !updated.inspectPanes[secretsPane] {
		t.Fatal("revealing should auto-open the secrets-pane inspect strip")
	}

	// Hide path: clearReveals (called on subscription/vault change) drops
	// every revealed value. Equivalent to a manual delete in the toggle.
	updated.clearReveals()
	if _, on := updated.revealedSecrets["api-key"]; on {
		t.Fatal("clearReveals should drop every revealed secret")
	}
}

// TestRevealedValueOrMaskFormat locks in the inspect-strip rendering
// for revealed and unrevealed secrets. The mask string is part of the
// UX contract — changes here should be intentional.
func TestRevealedValueOrMaskFormat(t *testing.T) {
	if got := revealedValueOrMask(""); !strings.Contains(got, "•") || !strings.Contains(got, "R to reveal") {
		t.Fatalf("masked render should show bullets and hint, got %q", got)
	}
	if got := revealedValueOrMask("plain"); got != "plain" {
		t.Fatalf("revealed render should pass value through verbatim, got %q", got)
	}
}

// TestIsTextInputActiveTrueForActionMenu guards against the parent
// tabapp eating keys (q→quit, 1–9→tab-jump) while the action menu — which
// fuzzy-filters typed characters — is open.
func TestIsTextInputActiveTrueForActionMenu(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	if m.IsTextInputActive() {
		t.Fatal("normal mode should not be text input")
	}
	m.actionMenu.open([]action{{label: "Yank"}})
	if !m.IsTextInputActive() {
		t.Fatal("action menu open: want text input active")
	}
}

func TestKeyVaultHelpDescribesMillerColumns(t *testing.T) {
	m := NewModel(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, nil)
	sections := m.HelpSections()
	joined := fmt.Sprint(sections)
	if !strings.Contains(joined, "column") || !strings.Contains(joined, "filter focused column") {
		t.Fatalf("help does not describe Miller column navigation: %v", sections)
	}
	if helpHasBlankGoUpBack(sections) || !strings.Contains(joined, "backspace  go up/back") {
		t.Fatalf("help must bind go up/back to backspace without blank entries: %v", sections)
	}
}

func helpHasBlankGoUpBack(sections []ui.HelpSection) bool {
	for _, section := range sections {
		for _, item := range section.Items {
			if strings.HasPrefix(item, "  go up/back") {
				return true
			}
		}
	}
	return false
}

func TestSetSubscriptionAllowsNilServiceWithTenant(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	if m.service == nil {
		t.Fatalf("NewModel(nil) left service nil")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetSubscription panicked with nil service: %v", r)
		}
	}()
	m.SetSubscription(azure.Subscription{ID: "sub", TenantID: "tenant"})
}

func TestPaneName(t *testing.T) {
	tests := []struct {
		pane int
		want string
	}{
		{vaultsPane, "vaults"},
		{secretsPane, "secrets"},
		{versionsPane, "versions"},
		{99, "items"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := paneName(tc.pane); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTypingQWhileFilteringDoesNotQuit(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.focus = vaultsPane
	m.vaultsList.SetFilterState(1) // list.Filtering

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if _, ok := updated.(Model); !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q in active filter not to quit")
	}
}

func TestHelpToggleOpensAndCloses(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	model := updated.(Model)
	if !model.HelpOverlay.Active {
		t.Fatal("expected ? to open help overlay")
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	model = updated.(Model)
	if model.HelpOverlay.Active {
		t.Fatal("expected ? to close help overlay")
	}
}

func TestViewShowsStatusBar(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 120
	m.Height = 40
	m.resize()

	view := m.View()
	if view.Content == "" {
		t.Fatal("expected view to render content")
	}
}

func TestKeyVaultViewUsesMillerChrome(t *testing.T) {
	m := NewModel(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, nil)
	m.Width = 100
	m.Height = 30
	m.SubOverlay.Close() // exercise the underlying chrome, not the picker
	m.resize()
	out := m.View().Content
	// "Key Vault" no longer appears in the breadcrumb — the tab bar
	// labels the explorer; the in-tab breadcrumb starts at the
	// subscription. The brand is still rendered.
	if !strings.Contains(out, "lazyaz") {
		t.Fatalf("compact Key Vault header missing brand: %q", out)
	}
	if !strings.Contains(out, "VAULTS") {
		t.Fatalf("vault column badge missing: %q", out)
	}
}

func TestSecretAndVersionRowsShowState(t *testing.T) {
	secret := secretItem{secret: keyvault.Secret{Name: "api-key", ContentType: "text/plain", Enabled: false}}
	if title := secret.Title(); !strings.Contains(title, "api-key") || !strings.Contains(secret.Description(), "disabled") {
		t.Fatalf("secret row missing state: title=%q desc=%q", title, secret.Description())
	}
	version := versionItem{version: keyvault.SecretVersion{Version: "1234567890abcdef", Enabled: false}}
	if !strings.Contains(version.Description(), "disabled") {
		t.Fatalf("version description missing disabled state: %q", version.Description())
	}
}

func TestMouseClickFirstMillerRowSelectsFirstVault(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 100
	m.Height = 30
	m.focus = vaultsPane
	m.vaultsList.SetItems(vaultsToItems([]keyvault.Vault{{Name: "first"}, {Name: "second"}}))
	m.resize()
	m.vaultsList.Select(1)

	consumed, _ := m.handleMouseClick(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      m.Width*20/100 + 1,
		Y:      m.paneAreaY() + 2,
	})

	if !consumed {
		t.Fatal("expected click in vault pane to be consumed")
	}
	if got := m.vaultsList.Index(); got != 0 {
		t.Fatalf("expected first rendered row click to select index 0, got %d", got)
	}
}

func TestKeyVaultMillerColumnRendersInspectFooter(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 120
	m.Height = 40
	m.SubOverlay.Close()
	m.focus = vaultsPane
	m.inspectPanes[vaultsPane] = true
	m.vaultsList.SetItems(vaultsToItems([]keyvault.Vault{{
		Name:           "vault-one",
		SubscriptionID: "sub-1",
		ResourceGroup:  "rg-one",
		VaultURI:       "https://vault-one.vault.azure.net/",
	}}))
	m.resize()

	out := m.View().Content
	if !strings.Contains(out, "Resource Group") || !strings.Contains(out, "rg-one") {
		t.Fatalf("inspect footer missing selected vault details: %q", out)
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestCurrentNavCapturesRootPane ensures the vaults-list view is a
// recordable jump target (ctrl+o must be able to walk back here after
// the user drills into a vault).
func TestCurrentNavCapturesRootPane(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SetSubscription(azure.Subscription{ID: "sub-1", Name: "Test"})
	m.focus = vaultsPane

	snap := m.CurrentNav()
	if snap == nil {
		t.Fatal("expected non-nil snapshot on vaults pane with subscription set")
	}
	kv, ok := snap.(kvNavSnapshot)
	if !ok {
		t.Fatalf("expected kvNavSnapshot, got %T", snap)
	}
	if kv.vaultName != "" {
		t.Errorf("expected empty vaultName, got %q", kv.vaultName)
	}
	if kv.focusedPane != vaultsPane {
		t.Errorf("expected focusedPane=vaultsPane, got %d", kv.focusedPane)
	}
}

// TestValidateSecretName covers Azure's naming rules so client-side
// feedback matches what SetSecret would return.
func TestValidateSecretName(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"", true},
		{"ok-name", false},
		{"OKName123", false},
		{"has_underscore", true},
		{"has space", true},
		{"has.dot", true},
	}
	for _, tc := range tests {
		got := validateSecretName(tc.in)
		if tc.wantErr && got == "" {
			t.Errorf("validateSecretName(%q) = empty, want error", tc.in)
		}
		if !tc.wantErr && got != "" {
			t.Errorf("validateSecretName(%q) = %q, want empty", tc.in, got)
		}
	}
}

// TestOpenCreateSecretFormGatesOnVault ensures opening the form
// without a selected vault is a no-op — executeAction relies on this.
func TestOpenCreateSecretFormGatesOnVault(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasVault = false
	m.openCreateSecretForm()
	if m.createSecret.Active {
		t.Fatal("expected form to stay inactive with no vault selected")
	}

	m.hasVault = true
	m.currentVault.Name = "vault-a"
	m.openCreateSecretForm()
	if !m.createSecret.Active {
		t.Fatal("expected form to open with vault selected")
	}
	if len(m.createSecret.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(m.createSecret.Fields))
	}
}

// TestOpenAddSecretVersionFormGatesOnSecret confirms the form only
// opens when both a vault and a secret are selected — adding a version
// to "no secret" is meaningless.
func TestOpenAddSecretVersionFormGatesOnSecret(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.openAddSecretVersionForm()
	if m.addSecretVersion.Active {
		t.Fatal("expected form to stay inactive without vault/secret selected")
	}

	m.hasVault = true
	m.currentVault.Name = "vault-a"
	m.openAddSecretVersionForm()
	if m.addSecretVersion.Active {
		t.Fatal("expected form to stay inactive without secret selected")
	}

	m.hasSecret = true
	m.currentSecret.Name = "db-password"
	m.openAddSecretVersionForm()
	if !m.addSecretVersion.Active {
		t.Fatal("expected form to open with vault and secret selected")
	}
	if len(m.addSecretVersion.Fields) != 1 {
		t.Fatalf("expected single Value field, got %d", len(m.addSecretVersion.Fields))
	}
	if m.addSecretVersion.Fields[0].Label != "Value" {
		t.Fatalf("expected sole field labelled Value, got %q", m.addSecretVersion.Fields[0].Label)
	}
}

// TestVisualSelectionRespectsActiveFilter pins the rule that visual-line
// mode walks the filtered view, not the underlying secrets slice. With
// a filter applied so only "alpha-*" rows are visible, anchoring on
// alpha-1 and moving the cursor to alpha-3 must yield the three alpha
// rows — never the beta rows that sit between them in m.secrets.
func TestVisualSelectionRespectsActiveFilter(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasVault = true
	m.focus = secretsPane
	m.kvKind = kvKindSecrets
	m.secrets = []keyvault.Secret{
		{Name: "alpha-1"},
		{Name: "beta-1"},
		{Name: "alpha-2"},
		{Name: "beta-2"},
		{Name: "alpha-3"},
	}
	m.refreshSecretItems()
	m.secretsList.SetFilterText("alpha")

	visible := m.secretsList.VisibleItems()
	if len(visible) != 3 {
		t.Fatalf("filter should expose 3 alpha rows, got %d", len(visible))
	}

	m.secretsList.Select(0) // alpha-1
	m.toggleVisualLineMode()
	if !m.visualLineMode {
		t.Fatal("expected visual line mode on")
	}
	m.secretsList.Select(2) // alpha-3

	got := m.visualSelectionItems()
	wantNames := []string{"alpha-1", "alpha-2", "alpha-3"}
	if len(got) != len(wantNames) {
		names := make([]string, 0, len(got))
		for _, it := range got {
			names = append(names, it.secret.Name)
		}
		t.Fatalf("visualSelectionItems = %v, want %v", names, wantNames)
	}
	for i, want := range wantNames {
		if got[i].secret.Name != want {
			t.Fatalf("visualSelectionItems[%d] = %q, want %q", i, got[i].secret.Name, want)
		}
	}
}

// TestApplyNavEmptyVaultRestoresFocus ensures the root-pane snapshot
// restores focus without dispatching a PendingNav drill-in.
func TestApplyNavEmptyVaultRestoresFocus(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SetSubscription(azure.Subscription{ID: "sub-1", Name: "Test"})
	m.focus = secretsPane
	m.hasVault = true

	cmd := m.ApplyNav(kvNavSnapshot{focusedPane: vaultsPane})

	if cmd != nil {
		t.Errorf("expected no cmd for root-pane restore, got %v", cmd)
	}
	if m.focus != vaultsPane {
		t.Errorf("expected focus=vaultsPane after restore, got %d", m.focus)
	}
	if m.pendingNav.hasTarget() {
		t.Error("expected no pending nav target after root-pane restore")
	}
}
