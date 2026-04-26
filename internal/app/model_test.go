package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/dashapp"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/kvapp"
	"github.com/karlssonsimon/lazyaz/internal/sbapp"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

type testCredential struct{ id string }

func (testCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, nil
}

func testConfig() ui.Config {
	scheme := ui.FallbackScheme()
	return ui.Config{ThemeName: scheme.Name, Schemes: []ui.Scheme{scheme}}
}

func TestNewModelAllowsNilDB(t *testing.T) {
	m := NewModel(nil, nil, nil, testConfig(), nil, keymap.Default())
	if len(m.tabs) != 1 {
		t.Fatalf("len(tabs) = %d, want 1", len(m.tabs))
	}
	if m.tabs[0].Kind != TabDashboard {
		t.Fatalf("tabs[0].Kind = %v, want %v", m.tabs[0].Kind, TabDashboard)
	}
}

func TestPostLoginSubsReconcilesDashboardTabs(t *testing.T) {
	cfg := testConfig()
	m := NewModel(nil, servicebus.NewService(nil), nil, cfg, nil, keymap.Default())
	dm, ok := m.tabs[0].Model.(dashapp.Model)
	if !ok {
		t.Fatalf("tabs[0].Model = %T, want dashapp.Model", m.tabs[0].Model)
	}
	dm.SetSubscription(azure.Subscription{ID: "old", Name: "Old"})
	m.tabs[0].Model = dm
	m.width = 80
	m.height = 24

	updated, _ := m.handlePostLoginSubs(postLoginSubsMsg{subs: []azure.Subscription{{ID: "old", Name: "Old"}}})
	dash, ok := updated.tabs[0].Model.(dashapp.Model)
	if !ok {
		t.Fatalf("tabs[0].Model = %T, want dashapp.Model", updated.tabs[0].Model)
	}
	if len(dash.Subscriptions) != 1 || dash.Subscriptions[0].ID != "old" {
		t.Fatalf("dash.Subscriptions = %#v, want old subscription", dash.Subscriptions)
	}
	current, ok := dash.CurrentSubscription()
	if !ok {
		t.Fatal("dash.CurrentSubscription() ok = false, want true")
	}
	if current.ID != "old" || current.Name != "Old" {
		t.Fatalf("dash.CurrentSubscription() = %#v, want old subscription", current)
	}
}

func TestApplyNewCredentialAllowsNilRootServices(t *testing.T) {
	m := NewModel(nil, nil, nil, testConfig(), nil, keymap.Default())

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("applyNewCredential panicked with nil services: %v", r)
		}
	}()
	m.applyNewCredential(testCredential{id: "new"}, "logged in")
}

func TestHandleOpenAzLoginAllowsNilBlobService(t *testing.T) {
	m := NewModel(nil, nil, nil, testConfig(), nil, keymap.Default())
	m.tenantPicker.open()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleOpenAzLogin panicked with nil blob service: %v", r)
		}
	}()

	updated, cmd := m.handleOpenAzLogin()
	if cmd != nil {
		t.Fatalf("handleOpenAzLogin returned command with nil blob service")
	}
	if updated.tenantPicker.active || updated.tenantPicker.loading {
		t.Fatalf("tenant picker state = active %v loading %v, want closed", updated.tenantPicker.active, updated.tenantPicker.loading)
	}
	notifications := updated.notifier.Snapshot()
	if len(notifications) == 0 || notifications[len(notifications)-1].Level != appshell.LevelError || !strings.Contains(notifications[len(notifications)-1].Message, "Azure credential unavailable") {
		t.Fatalf("last notification = %#v, want Azure credential unavailable error", notifications)
	}
}

func TestNewPerTabServicesCopyCredentialWithoutSharingService(t *testing.T) {
	cred := testCredential{id: "cred"}
	m := Model{
		blobSvc: blob.NewService(cred),
		sbSvc:   servicebus.NewService(cred),
		kvSvc:   keyvault.NewService(cred),
	}

	if got := m.newBlobService(); got == nil || got == m.blobSvc || got.Credential() != cred {
		t.Fatalf("newBlobService() = %#v, want distinct service with original credential", got)
	}
	if got := m.newServiceBusService(); got == nil || got == m.sbSvc || got.Credential() != cred {
		t.Fatalf("newServiceBusService() = %#v, want distinct service with original credential", got)
	}
	if got := m.newKeyVaultService(); got == nil || got == m.kvSvc || got.Credential() != cred {
		t.Fatalf("newKeyVaultService() = %#v, want distinct service with original credential", got)
	}

	empty := Model{}
	if got := empty.newBlobService(); got == nil || got.Credential() != nil {
		t.Fatalf("nil parent blob service produced %#v, want service with nil credential", got)
	}
	if got := empty.newServiceBusService(); got == nil || got.Credential() != nil {
		t.Fatalf("nil parent service bus service produced %#v, want service with nil credential", got)
	}
	if got := empty.newKeyVaultService(); got == nil || got.Credential() != nil {
		t.Fatalf("nil parent key vault service produced %#v, want service with nil credential", got)
	}
}

func TestAddTabUsesPrivateServices(t *testing.T) {
	cred := testCredential{id: "cred"}
	m := NewModel(blob.NewService(cred), servicebus.NewService(cred), keyvault.NewService(cred), testConfig(), nil, keymap.Default())

	m.addTab(TabBlob, "")
	if child, ok := m.tabs[m.activeIdx].Model.(blobapp.Model); !ok || servicePointer(child) == 0 || servicePointer(child) == reflect.ValueOf(m.blobSvc).Pointer() {
		t.Fatalf("blob tab service pointer = %#x, want private service", servicePointer(child))
	}

	m.addTab(TabServiceBus, "")
	if child, ok := m.tabs[m.activeIdx].Model.(sbapp.Model); !ok || servicePointer(child) == 0 || servicePointer(child) == reflect.ValueOf(m.sbSvc).Pointer() {
		t.Fatalf("service bus tab service pointer = %#x, want private service", servicePointer(child))
	}

	m.addTab(TabKeyVault, "")
	if child, ok := m.tabs[m.activeIdx].Model.(kvapp.Model); !ok || servicePointer(child) == 0 || servicePointer(child) == reflect.ValueOf(m.kvSvc).Pointer() {
		t.Fatalf("key vault tab service pointer = %#x, want private service", servicePointer(child))
	}

	m.addTab(TabDashboard, "")
	if child, ok := m.tabs[m.activeIdx].Model.(dashapp.Model); !ok || servicePointer(child) == 0 || servicePointer(child) == reflect.ValueOf(m.sbSvc).Pointer() {
		t.Fatalf("dashboard tab service pointer = %#x, want private service", servicePointer(child))
	}
}

func servicePointer(model any) uintptr {
	return reflect.ValueOf(model).FieldByName("service").Pointer()
}

func serviceCredential(model any) azcore.TokenCredential {
	value := reflect.New(reflect.ValueOf(model).Type())
	value.Elem().Set(reflect.ValueOf(model))
	field := value.Elem().FieldByName("service")
	service := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	switch svc := service.(type) {
	case *blob.Service:
		return svc.Credential()
	case *servicebus.Service:
		return svc.Credential()
	case *keyvault.Service:
		return svc.Credential()
	default:
		return nil
	}
}

func TestPostLoginSubsReappliesMatchedSubscription(t *testing.T) {
	oldSub := azure.Subscription{ID: "sub", Name: "Old", TenantID: "old-tenant"}
	matched := azure.Subscription{ID: "sub", Name: "New", TenantID: "new-tenant"}
	m := NewModel(blob.NewService(nil), servicebus.NewService(nil), keyvault.NewService(nil), testConfig(), nil, keymap.Default())
	m.tabs = nil
	m.activeIdx = 0
	m.addTab(TabBlob, "")
	m.addTab(TabServiceBus, "")
	m.addTab(TabKeyVault, "")
	m.addTab(TabDashboard, "")

	for i := range m.tabs {
		switch child := m.tabs[i].Model.(type) {
		case blobapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case sbapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case kvapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case dashapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		}
	}

	updated, _ := m.handlePostLoginSubs(postLoginSubsMsg{subs: []azure.Subscription{matched}})

	for _, tab := range updated.tabs {
		var current azure.Subscription
		var ok bool
		switch child := tab.Model.(type) {
		case blobapp.Model:
			current, ok = child.CurrentSubscription()
		case sbapp.Model:
			current, ok = child.CurrentSubscription()
		case kvapp.Model:
			current, ok = child.CurrentSubscription()
		case dashapp.Model:
			current, ok = child.CurrentSubscription()
		default:
			t.Fatalf("unexpected tab model %T", tab.Model)
		}
		if !ok || current != matched {
			t.Fatalf("%v current subscription = %#v, %v; want %#v, true", tab.Kind, current, ok, matched)
		}
	}
}

func TestPostLoginSubsUpdatesPrivateServiceCredentialForMatchedSubscription(t *testing.T) {
	oldCred := testCredential{id: "old"}
	newCred := testCredential{id: "new"}
	oldSub := azure.Subscription{ID: "sub", Name: "Old", TenantID: "old-tenant"}
	matched := azure.Subscription{ID: "sub", Name: "New"}
	m := NewModel(blob.NewService(oldCred), servicebus.NewService(oldCred), keyvault.NewService(oldCred), testConfig(), nil, keymap.Default())
	m.tabs = nil
	m.activeIdx = 0
	m.addTab(TabBlob, "")
	m.addTab(TabServiceBus, "")
	m.addTab(TabKeyVault, "")
	m.addTab(TabDashboard, "")

	for i := range m.tabs {
		switch child := m.tabs[i].Model.(type) {
		case blobapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case sbapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case kvapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case dashapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		}
	}

	updated, _ := m.applyNewCredential(newCred, "logged in")
	updated, _ = updated.handlePostLoginSubs(postLoginSubsMsg{subs: []azure.Subscription{matched}})

	for _, tab := range updated.tabs {
		if got := serviceCredential(tab.Model); got != newCred {
			t.Fatalf("%v service credential = %#v, want new credential", tab.Kind, got)
		}
	}
}

func TestPostLoginSubsUpdatesPrivateServiceCredentialWhenSubscriptionGone(t *testing.T) {
	oldCred := testCredential{id: "old"}
	newCred := testCredential{id: "new"}
	oldSub := azure.Subscription{ID: "old-sub", Name: "Old"}
	newSub := azure.Subscription{ID: "new-sub", Name: "New"}
	m := NewModel(blob.NewService(oldCred), servicebus.NewService(oldCred), keyvault.NewService(oldCred), testConfig(), nil, keymap.Default())
	m.tabs = nil
	m.activeIdx = 0
	m.addTab(TabBlob, "")
	m.addTab(TabServiceBus, "")
	m.addTab(TabKeyVault, "")
	m.addTab(TabDashboard, "")

	for i := range m.tabs {
		switch child := m.tabs[i].Model.(type) {
		case blobapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case sbapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case kvapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		case dashapp.Model:
			child.SetSubscription(oldSub)
			m.tabs[i].Model = child
		}
	}

	updated, _ := m.applyNewCredential(newCred, "logged in")
	updated, _ = updated.handlePostLoginSubs(postLoginSubsMsg{subs: []azure.Subscription{newSub}})

	for _, tab := range updated.tabs {
		if got := serviceCredential(tab.Model); got != newCred {
			t.Fatalf("%v service credential = %#v, want new credential", tab.Kind, got)
		}
		switch child := tab.Model.(type) {
		case blobapp.Model:
			if child.HasSubscription || !child.SubOverlay.Active || len(child.Subscriptions) != 1 || child.Subscriptions[0] != newSub {
				t.Fatalf("blob gone state = has:%v overlay:%v subs:%#v", child.HasSubscription, child.SubOverlay.Active, child.Subscriptions)
			}
		case sbapp.Model:
			if child.HasSubscription || !child.SubOverlay.Active || len(child.Subscriptions) != 1 || child.Subscriptions[0] != newSub {
				t.Fatalf("service bus gone state = has:%v overlay:%v subs:%#v", child.HasSubscription, child.SubOverlay.Active, child.Subscriptions)
			}
		case kvapp.Model:
			if child.HasSubscription || !child.SubOverlay.Active || len(child.Subscriptions) != 1 || child.Subscriptions[0] != newSub {
				t.Fatalf("key vault gone state = has:%v overlay:%v subs:%#v", child.HasSubscription, child.SubOverlay.Active, child.Subscriptions)
			}
		case dashapp.Model:
			if child.HasSubscription || !child.SubOverlay.Active || len(child.Subscriptions) != 1 || child.Subscriptions[0] != newSub {
				t.Fatalf("dashboard gone state = has:%v overlay:%v subs:%#v", child.HasSubscription, child.SubOverlay.Active, child.Subscriptions)
			}
		default:
			t.Fatalf("unexpected tab model %T", tab.Model)
		}
	}
}
