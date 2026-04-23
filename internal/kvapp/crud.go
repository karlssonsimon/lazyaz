package kvapp

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// secretNameRe enforces Azure Key Vault's name rules: 1-127 chars,
// alphanumerics and hyphens only.
var secretNameRe = regexp.MustCompile(`^[0-9a-zA-Z-]{1,127}$`)

// validateSecretName returns an empty string when the name is valid;
// otherwise a human-readable message suitable for display under the
// form field.
func validateSecretName(name string) string {
	switch {
	case name == "":
		return "name is required"
	case len(name) > 127:
		return "must be at most 127 characters"
	case !secretNameRe.MatchString(name):
		return "only letters, digits, and hyphens allowed"
	}
	return ""
}

// validateSecretValue rejects empty values — Azure rejects them too,
// catching it client-side avoids a round trip.
func validateSecretValue(value string) string {
	if value == "" {
		return "value is required"
	}
	return ""
}

// openCreateSecretForm mounts the two-field form. Must be called from
// a context where hasVault is true; the action menu gates this so the
// check is asserted here defensively.
func (m *Model) openCreateSecretForm() {
	if !m.hasVault {
		return
	}
	m.createSecret.Open(
		fmt.Sprintf("Create secret in %s", m.currentVault.Name),
		[]ui.FormField{
			{Label: "Name", Placeholder: "db-password", Validate: validateSecretName},
			{Label: "Value", Placeholder: "secret value", Validate: validateSecretValue},
		},
	)
}

// handleFormKey routes a key to the active form overlay. On Submit
// the overlay is closed and the create cmd dispatched; on Cancel the
// overlay is just closed.
func (m Model) handleFormKey(key string) (Model, tea.Cmd) {
	res := m.createSecret.HandleKey(key)
	switch res.Action {
	case ui.FormActionSubmit:
		if !m.hasVault || len(res.Values) < 2 {
			return m, nil
		}
		name, value := res.Values[0], res.Values[1]
		m.startLoading(secretsPane, fmt.Sprintf("Creating secret %s in %s...", name, m.currentVault.Name))
		return m, tea.Batch(m.Spinner.Tick, createSecretCmd(m.service, m.currentVault, name, value))
	}
	return m, nil
}

// secretCreatedMsg is the async result of createSecretCmd.
type secretCreatedMsg struct {
	vaultName  string
	secretName string
	err        error
}

// createSecretCmd calls SetSecret on the service and returns a
// secretCreatedMsg with either the created name or an error. The call
// has a 30-second timeout independent of the app's loading spinner.
func createSecretCmd(svc *keyvault.Service, vault keyvault.Vault, name, value string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := svc.SetSecret(ctx, vault, name, value)
		return secretCreatedMsg{
			vaultName:  vault.Name,
			secretName: name,
			err:        err,
		}
	}
}

// handleSecretCreated reports the outcome and refreshes the secrets
// list so the new entry appears without the user having to press R.
// Refresh is unconditional on success because SetSecret also produces
// a new version row for updates to an existing name.
func (m Model) handleSecretCreated(msg secretCreatedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to create secret: %s", msg.err.Error()))
		return m, nil
	}
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Created secret %s in %s", msg.secretName, msg.vaultName))

	// Refresh the secrets list so the new row shows up. Reuse the
	// existing fetch cmd; the broker merges the new entry into the list.
	if !m.hasVault || m.currentVault.Name != msg.vaultName {
		return m, nil
	}
	m.startLoading(secretsPane, fmt.Sprintf("Refreshing secrets in %s", m.currentVault.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchSecretsCmd(m.service, m.cache.secrets, m.currentVault, m.secrets))
}
