package acapp

import (
	"strings"
	"testing"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var testConfig = ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}

func testStore() appconfig.Store {
	return appconfig.Store{Name: "cfg-htg-prod", SubscriptionID: "sub-1", ResourceGroup: "rg", Endpoint: "https://cfg-htg-prod.azconfig.io"}
}

func testSettings() []appconfig.Setting {
	return []appconfig.Setting{
		{Key: "Db/ConnectionTimeout", Label: "", Value: "30", ContentType: "text/plain", LastModified: time.Now()},
		{Key: "Db/ConnectionTimeout", Label: "prod", Value: "60", LastModified: time.Now()},
		{Key: "Features/Checkout", Label: "prod", Value: `{"enabled":true,"retries":3}`, ContentType: "application/json"},
		{Key: "Db/Password", Label: "prod", ContentType: "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8",
			Value: `{"uri":"https://kv-htg-prod.vault.azure.net/secrets/DbPassword"}`},
		{Key: ".appconfig.featureflag/beta", ContentType: "application/vnd.microsoft.appconfig.ff+json;charset=utf-8", Value: `{"id":"beta","enabled":false}`},
	}
}

// storeModel is a model with a subscription, a store and its settings
// loaded from the cache, sized for rendering.
func storeModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.HasSubscription = true
	m.CurrentSub = azure.Subscription{ID: "sub-1", Name: "it-prod"}
	m.Width, m.Height = 140, 40
	m.resize()

	m.cache.stores.Set("sub-1", []appconfig.Store{testStore()})
	m.cache.settings.Set(settingsScope(testStore()), testSettings())
	m.stores = []appconfig.Store{testStore()}
	m.storesList.SetItems(storesToItems(m.stores))
	m.storesList.Select(0)
	return m
}

func keys(t *testing.T, m Model, ks ...string) Model {
	t.Helper()
	for _, k := range ks {
		next, _ := m.handleKey(tea.KeyPressMsg{Code: -1, Text: k})
		m = next
	}
	return m
}

// Enter on a store opens its settings from the cache; Enter on a
// setting opens the value pane and focuses it.
func TestDrillStoreThenValue(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	if !m.hasStore || m.focus != settingsPane {
		t.Fatalf("after Enter on store: hasStore=%v focus=%d", m.hasStore, m.focus)
	}
	if n := len(m.settingsList.Items()); n != 5 {
		t.Fatalf("settings hydrated from cache = %d, want 5", n)
	}

	m, _ = m.handleEnter()
	if !m.viewingValue || m.focus != valuePane {
		t.Fatalf("after Enter on setting: viewing=%v focus=%d", m.viewingValue, m.focus)
	}
	if m.currentSetting.Key != "Db/ConnectionTimeout" || m.currentSetting.Label != "" {
		t.Fatalf("opened %+v, want the first row", m.currentSetting)
	}
	// With the value focused the three slots are settings, value and
	// nothing: the stores column is off screen, as in the other apps.
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"SETTINGS", "VALUE", "Db/ConnectionTimeout", "30"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
	m, _ = m.navigateLeft()
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "STORES") {
		t.Error("stores column should return when the settings pane has focus")
	}
}

// The settings rows carry the label and a tag for references and
// flags, aligned after the key.
func TestSettingsRowsShowLabelsAndKinds(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"prod", "→ key vault", "flag", "beta"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, ".appconfig.featureflag/") {
		t.Error("feature flag prefix should be hidden in the list")
	}
}

// The value pane follows the settings cursor while open, and h returns
// to the list without closing it.
func TestValuePaneFollowsCursor(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m, _ = m.handleEnter()
	m, _ = m.navigateLeft()
	if m.focus != settingsPane || !m.viewingValue {
		t.Fatalf("h should return to the list and keep the value open: focus=%d viewing=%v", m.focus, m.viewingValue)
	}
	m.settingsList.CursorDown()
	m.syncValueToSelection()
	if m.currentSetting.Label != "prod" || m.currentSetting.Value != "60" {
		t.Fatalf("value did not follow the cursor: %+v", m.currentSetting)
	}
}

// A Key Vault reference renders as an explanation and Enter in the
// value pane asks the parent to open it.
func TestKeyVaultReferenceOpensTab(t *testing.T) {
	m := storeModel(t)
	m.EmbeddedMode = true
	m, _ = m.handleEnter()
	m.settingsList.Select(3)
	m, _ = m.handleEnter()
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "kv-htg-prod") || !strings.Contains(view, "DbPassword") {
		t.Fatalf("reference not explained in the value pane:\n%s", view)
	}
	_, cmd := m.handleEnter()
	if cmd == nil {
		t.Fatal("Enter on a reference should emit a command")
	}
	msg, ok := cmd().(OpenKeyVaultReferenceMsg)
	if !ok || msg.VaultName != "kv-htg-prod" || msg.SecretName != "DbPassword" || msg.Subscription.ID != "sub-1" {
		t.Fatalf("emitted %#v", msg)
	}
}

// Changing store closes the value pane so a value never outlives its scope.
func TestChangingStoreClosesValue(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m, _ = m.handleEnter()
	other := appconfig.Store{Name: "cfg-other", SubscriptionID: "sub-1", Endpoint: "https://cfg-other.azconfig.io"}
	m, _ = m.selectStore(other)
	if m.viewingValue || m.focus != settingsPane {
		t.Fatalf("value pane survived a store change: viewing=%v focus=%d", m.viewingValue, m.focus)
	}
}

// The jump snapshot round-trips through ApplyNav back to the opened value.
func TestCurrentNavRoundTrip(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(2)
	m, _ = m.handleEnter()
	snap := m.CurrentNav()
	if snap == nil || !strings.Contains(snap.Description(), "cfg-htg-prod") {
		t.Fatalf("snapshot = %v", snap)
	}

	fresh := storeModel(t)
	fresh.ApplyNav(snap)
	if !fresh.hasStore || fresh.currentStore.Name != "cfg-htg-prod" {
		t.Fatalf("store not restored: %+v", fresh.currentStore)
	}
	if !fresh.viewingValue || fresh.currentSetting.Key != "Features/Checkout" {
		t.Fatalf("value not restored: viewing=%v setting=%+v", fresh.viewingValue, fresh.currentSetting)
	}
}

// Inspect fields describe the setting, including the reference target.
func TestInspectSetting(t *testing.T) {
	ref := testSettings()[3]
	fields := settingInspect(ref)
	joined := ""
	for _, f := range fields {
		joined += f.Label + "=" + f.Value + "\n"
	}
	for _, want := range []string{"Kind=Key Vault reference", "Vault=kv-htg-prod", "Secret=DbPassword", "Label=prod"} {
		if !strings.Contains(joined, want) {
			t.Errorf("inspect missing %q:\n%s", want, joined)
		}
	}
}

// Copy targets expose key, value, label and the reference parts.
func TestCopyTargets(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(3)
	m, _ = m.handleEnter()
	labels := ""
	for _, tgt := range m.copyTargets() {
		labels += tgt.Label + "|"
	}
	for _, want := range []string{"Key|", "Value|", "Label|", "Vault|", "Secret|", "Secret URI|", "Store|", "Endpoint|"} {
		if !strings.Contains(labels, want) {
			t.Errorf("copy targets missing %q: %s", want, labels)
		}
	}
}
