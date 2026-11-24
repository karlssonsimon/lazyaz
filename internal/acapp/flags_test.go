package acapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"

	"github.com/charmbracelet/x/ansi"
)

// The list shows flag state; the action menu offers to flip it; the
// confirmation flips only "enabled".
func TestFlagStateAndToggle(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "flag off") {
		t.Fatalf("flag state not shown:\n%s", view)
	}
	m.settingsList.Select(4) // beta, enabled:false
	label, ok := menuHas(m, actionToggleFlag)
	if !ok || !strings.Contains(label, "Turn flag beta on") {
		t.Fatalf("toggle entry = %q ok=%v", label, ok)
	}
	m, _ = m.openToggleFlagConfirm()
	if !m.confirmModal.Active || !strings.Contains(m.confirmModal.Message, "off → on") || m.confirmAction == nil {
		t.Fatalf("toggle confirm: active=%v msg=%q", m.confirmModal.Active, m.confirmModal.Message)
	}
	if !m.confirmModal.Destructive {
		t.Error("turning a flag on should use the destructive prompt")
	}

	// A plain setting has no toggle.
	m.confirmModal.Close()
	m.settingsList.Select(0)
	if _, ok := menuHas(m, actionToggleFlag); ok {
		t.Fatal("toggle offered on a plain key-value")
	}
}

// Creating a flag validates the name and builds the portal-shaped value.
func TestCreateFlagForm(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	if _, ok := menuHas(m, actionCreateFlag); !ok {
		t.Fatal("create flag not offered")
	}
	m, _ = m.openCreateFlagForm()
	if !m.createFlagForm.Active || m.inputMode() != ModeForm {
		t.Fatal("flag form not open")
	}
	m.createFlagForm.Fields[flagNameField].Value = "bad/name"
	m, cmd := m.handleCreateFlagFormKey("enter")
	if cmd != nil || m.createFlagForm.Fields[flagNameField].Error == "" {
		t.Fatal("invalid flag name accepted")
	}
	m.createFlagForm.Fields[flagNameField].Value = "beta"
	m, cmd = m.handleCreateFlagFormKey("enter")
	if cmd != nil || !strings.Contains(m.Status, "already exists") {
		t.Fatalf("duplicate flag not refused: status=%q", m.Status)
	}

	m, _ = m.openCreateFlagForm()
	m.createFlagForm.Fields[flagNameField].Value = "new-checkout"
	m.createFlagForm.Fields[flagDescriptionField].Value = "New checkout"
	m.createFlagForm.Fields[flagEnabledField].Value = "on"
	m, cmd = m.handleCreateFlagFormKey("enter")
	if cmd == nil || !m.Loading || !strings.Contains(m.Status, "new-checkout") {
		t.Fatalf("valid flag did not dispatch: cmd=%v status=%q", cmd != nil, m.Status)
	}

	// The value the form builds parses back as an enabled flag.
	v := appconfig.NewFeatureFlagValue("new-checkout", "New checkout", true)
	ff, ok := appconfig.ParseFeatureFlag(appconfig.Setting{Key: appconfig.FeatureFlagKey("new-checkout"), ContentType: appconfig.FeatureFlagContentType, Value: v})
	if !ok || !ff.Enabled || ff.Description != "New checkout" {
		t.Fatalf("built flag = %+v", ff)
	}
}

func TestOnOffParsing(t *testing.T) {
	for _, on := range []string{"on", "ON", "true", "yes"} {
		if !parseOnOff(on) || validateOnOff(on) != "" {
			t.Errorf("%q should be on and valid", on)
		}
	}
	for _, off := range []string{"off", "false", "no", ""} {
		if parseOnOff(off) || validateOnOff(off) != "" {
			t.Errorf("%q should be off and valid", off)
		}
	}
	if validateOnOff("maybe") == "" {
		t.Error("maybe accepted")
	}
}
