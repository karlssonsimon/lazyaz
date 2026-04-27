package sbapp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func TestSbappDoesNotImportAzureServiceBusSDK(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus") {
			t.Fatalf("%s imports azservicebus; SDK types must stay inside servicebus package", entry.Name())
		}
	}
}

func TestMessageItemKeyUsesLockedOperationKey(t *testing.T) {
	first := messageItem{message: servicebus.PeekedMessage{MessageID: "dup", LockID: "0"}}
	second := messageItem{message: servicebus.PeekedMessage{MessageID: "dup", LockID: "1"}}

	if got := messageItemKey(first); got != "0" {
		t.Fatalf("first messageItemKey = %q, want lock key", got)
	}
	if got := messageItemKey(second); got != "1" {
		t.Fatalf("second messageItemKey = %q, want lock key", got)
	}
	if messageItemKey(first) == messageItemKey(second) {
		t.Fatalf("duplicate MessageID locked messages must have distinct list keys")
	}
}

var testConfig = ui.Config{
	ThemeName: "fallback",
	Schemes:   []ui.Scheme{{Name: "fallback", Base00: "1e293b", Base01: "4B5563", Base02: "334155", Base03: "94A3B8", Base04: "94A3B8", Base05: "E5E7EB", Base06: "F8FAFC", Base07: "F8FAFC", Base08: "F87171", Base09: "F59E0B", Base0A: "F59E0B", Base0B: "22C55E", Base0C: "38BDF8", Base0D: "60A5FA", Base0E: "C084FC", Base0F: "94A3B8"}},
}

func TestServiceBusHelpDescribesMillerColumns(t *testing.T) {
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

func TestTrimToWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{name: "short string", input: "hello", max: 10, want: "hello"},
		{name: "exact fit", input: "hello", max: 5, want: "hello"},
		{name: "truncated", input: "hello world", max: 8, want: "hello..."},
		{name: "zero max", input: "hello", max: 0, want: ""},
		{name: "max 3", input: "hello", max: 3, want: "hel"},
		{name: "max 2", input: "hello", max: 2, want: "he"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ui.TrimToWidth(tc.input, tc.max); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTruncateForStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{name: "short", input: "hello", max: 10, want: "hello"},
		{name: "exact", input: "hello", max: 5, want: "hello"},
		{name: "truncated", input: "hello world", max: 5, want: "hello..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateForStatus(tc.input, tc.max); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestPaneName(t *testing.T) {
	tests := []struct {
		pane int
		want string
	}{
		{namespacesPane, "namespaces"},
		{entitiesPane, "entities"},
		{subscriptionsPane, "subscriptions"},
		{messagesPane, "messages"},
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

func TestEntityDisplayName(t *testing.T) {
	tests := []struct {
		name string
		e    servicebus.Entity
		want string
	}{
		{name: "queue", e: servicebus.Entity{Name: "orders", Kind: servicebus.EntityQueue}, want: "☰ orders"},
		{name: "topic", e: servicebus.Entity{Name: "events", Kind: servicebus.EntityTopic}, want: "▶ events"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := entityDisplayName(tc.e); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTypingQWhileFilteringDoesNotQuit(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.focus = namespacesPane
	m.namespacesList.SetFilterState(list.Filtering)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	model, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q in active filter not to quit")
	}

	if model.namespacesList.FilterValue() != "q" {
		t.Fatalf("expected filter value %q, got %q", "q", model.namespacesList.FilterValue())
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

func TestPasteRoutesToActionMenu(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.actionMenu.open([]action{
		{label: "Download"},
		{label: "Delete"},
	})

	updated, _ := m.Update(tea.PasteMsg{Content: "del"})
	model := updated.(Model)

	if model.actionMenu.Query != "del" {
		t.Fatalf("expected action menu query %q, got %q", "del", model.actionMenu.Query)
	}
	selected, ok := model.actionMenu.Selected()
	if !ok || selected.label != "Delete" {
		t.Fatalf("expected pasted query to select Delete, got %+v ok=%v", selected, ok)
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

func TestServiceBusViewUsesMillerChrome(t *testing.T) {
	m := NewModel(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, nil)
	m.Width = 100
	m.Height = 30
	m.resize()
	out := m.View().Content
	// "Service Bus" no longer appears in the breadcrumb — the tab bar
	// labels the explorer. Brand stays.
	if !strings.Contains(out, "lazyaz") {
		t.Fatalf("compact Service Bus header missing brand: %q", out)
	}
	if !strings.Contains(out, "NAMESPACES") {
		t.Fatalf("namespace column badge missing: %q", out)
	}
	if strings.Contains(out, "Namespaces ·") {
		t.Fatalf("old namespace pane title rendered: %q", out)
	}
}

func TestMessagePreviewViewportRegionMatchesFlatDetailsColumn(t *testing.T) {
	m := NewModel(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, nil)
	m.Width = 120
	m.Height = 30
	m.EmbeddedMode = true
	m.hasPeekTarget = true
	m.focus = messagesPane
	m.viewingMessage = true
	m.selectedMessage = servicebus.PeekedMessage{MessageID: "msg-1"}
	m.resize()

	wantHeight := ui.MillerListBodyHeight(m.paneHeight, false) - 1
	if got := m.messageViewport.Height(); got != wantHeight {
		t.Fatalf("message viewport height = %d, want %d", got, wantHeight)
	}

	region := m.messageViewportRegion()
	// Tab bar (1) + app header (1) + horizontal rule (1) +
	// column title row (1) + preview-title row (1) = 5.
	wantY := ui.AppHeaderHeight + ui.TabBarHeight + 1 + 2
	if region.Y != wantY {
		t.Fatalf("message viewport Y = %d, want %d", region.Y, wantY)
	}
	if region.Height != wantHeight {
		t.Fatalf("message viewport region height = %d, want %d", region.Height, wantHeight)
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}
