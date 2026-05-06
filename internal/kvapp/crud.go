package kvapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	m.createSecret.OpenWithBreadcrumb(
		"Create secret",
		[]string{m.currentVault.Name},
		[]ui.FormField{
			{
				Label:       "Name",
				Placeholder: "db-password",
				Section:     "required",
				MaxChars:    127,
				Validate:    validateSecretName,
			},
			{
				Label:       "Value",
				Placeholder: "paste secret value",
				Hint:        "ctrl+r reveal",
				Mask:        true,
				Validate:    validateSecretValue,
			},
		},
	)
}

// handleFormKey routes a key to whichever form overlay is currently
// open. On Submit the overlay closes and the corresponding create cmd
// dispatches; on Cancel the overlay just closes.
func (m Model) handleFormKey(key string) (Model, tea.Cmd) {
	switch {
	case m.createSecret.Active:
		res := m.createSecret.HandleKey(key)
		if res.Action == ui.FormActionSubmit {
			if !m.hasVault || len(res.Values) < 2 {
				return m, nil
			}
			name, value := res.Values[0], res.Values[1]
			m.startLoading(secretsPane, fmt.Sprintf("Creating secret %s in %s...", name, m.currentVault.Name))
			return m, tea.Batch(m.Spinner.Tick, createSecretCmd(m.service, m.currentVault, name, value))
		}
		return m, nil
	case m.createKey.Active:
		res := m.createKey.HandleKey(key)
		if res.Action == ui.FormActionSubmit {
			if !m.hasVault || len(res.Values) < 2 {
				return m, nil
			}
			name, alg := res.Values[0], strings.ToUpper(strings.TrimSpace(res.Values[1]))
			kty, size, curve := parseKeyAlgorithm(alg)
			m.startLoading(secretsPane, fmt.Sprintf("Creating key %s (%s) in %s...", name, alg, m.currentVault.Name))
			return m, tea.Batch(m.Spinner.Tick, createKeyCmd(m.service, m.currentVault, name, kty, size, curve))
		}
		return m, nil
	case m.importCert.Active:
		res := m.importCert.HandleKey(key)
		if res.Action == ui.FormActionSubmit {
			if !m.hasVault || len(res.Values) < 2 || m.pendingCertPath == "" {
				return m, nil
			}
			name, password := res.Values[0], res.Values[1]
			path := m.pendingCertPath
			m.pendingCertPath = ""
			m.startLoading(secretsPane, fmt.Sprintf("Importing certificate %s in %s...", name, m.currentVault.Name))
			return m, tea.Batch(m.Spinner.Tick, importCertificateCmd(m.service, m.currentVault, name, path, password))
		}
		// Cancel path resets the pending cert path so a re-trigger doesn't
		// reuse a stale selection.
		if res.Action == ui.FormActionCancel {
			m.pendingCertPath = ""
		}
		return m, nil
	}
	return m, nil
}

// openCreateKeyForm mounts the two-field key-create form. Algorithm is
// a single string of one of: RSA-2048, RSA-3072, RSA-4096, EC-P256,
// EC-P384, EC-P521. Default RSA-2048 covers the common case; the help
// text spells out the rest.
func (m *Model) openCreateKeyForm() {
	if !m.hasVault {
		return
	}
	algorithm := FormField("Algorithm")
	algorithm.Value = "RSA-2048"
	m.createKey.OpenWithBreadcrumb(
		"Create key",
		[]string{m.currentVault.Name},
		[]ui.FormField{
			{
				Label:       "Name",
				Placeholder: "my-key",
				Section:     "required",
				MaxChars:    127,
				Validate:    validateSecretName,
			},
			{
				Label:       "Algorithm",
				Value:       "RSA-2048",
				Help:        "RSA-2048 · RSA-3072 · RSA-4096 · EC-P256 · EC-P384 · EC-P521",
				Validate:    validateKeyAlgorithm,
			},
		},
	)
}

// FormField is a typo-safe shorthand for declaring a default ui.FormField
// — kept private to this file because it's only useful here.
func FormField(label string) ui.FormField { return ui.FormField{Label: label} }

// openImportCertBrowser launches the file picker for selecting a PFX.
// When the user confirms a single file, the import form opens with the
// path stashed in m.pendingCertPath.
func (m *Model) openImportCertBrowser() {
	if !m.hasVault {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "/"
	}
	m.certImportBrowser = ui.FileBrowserState{}
	m.certImportBrowser.Open(cwd, ui.OSDirReader{})
	m.certImportBrowserActive = true
}

// handleCertImportBrowserKey routes keys to the file browser. On
// Confirm with exactly one selected path it closes the browser and
// opens the import form pre-loaded with the path. Cancel closes
// everything cleanly. Multiple selections aren't meaningful for
// certificate import — only the first path is honoured.
func (m Model) handleCertImportBrowserKey(key string) (Model, tea.Cmd) {
	res := m.certImportBrowser.HandleKey(key)
	switch res.Action {
	case ui.FBActionNone:
		return m, nil
	case ui.FBActionCancel:
		m.certImportBrowserActive = false
		return m, nil
	case ui.FBActionConfirm:
		m.certImportBrowserActive = false
		if len(res.Selected) == 0 {
			return m, nil
		}
		path := res.Selected[0]
		if !strings.EqualFold(filepath.Ext(path), ".pfx") {
			m.Notify(appshell.LevelWarn, "Expected a .pfx file — Azure Key Vault only imports PFX (PKCS#12).")
			return m, nil
		}
		m.pendingCertPath = path
		m.openImportCertForm(path)
		return m, nil
	}
	return m, nil
}

// openImportCertForm mounts the cert import form once the PFX path is
// known. The path appears in the breadcrumb so the user sees what
// they're about to upload.
func (m *Model) openImportCertForm(path string) {
	m.importCert.OpenWithBreadcrumb(
		"Import certificate",
		[]string{m.currentVault.Name, filepath.Base(path)},
		[]ui.FormField{
			{
				Label:       "Name",
				Placeholder: "my-cert",
				Section:     "required",
				MaxChars:    127,
				Validate:    validateSecretName,
			},
			{
				Label:       "Password",
				Placeholder: "PFX password (leave blank if none)",
				Hint:        "ctrl+r reveal",
				Mask:        true,
			},
		},
	)
}

// validateKeyAlgorithm accepts the six values listed in the form's
// Help text. Case-insensitive on the prefix; the curve / size suffix
// is normalised to upper-case before matching so "rsa-2048" works too.
func validateKeyAlgorithm(value string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	switch v {
	case "RSA-2048", "RSA-3072", "RSA-4096", "EC-P256", "EC-P384", "EC-P521":
		return ""
	case "":
		return "algorithm is required"
	}
	return "expected RSA-2048/3072/4096 or EC-P256/P384/P521"
}

// parseKeyAlgorithm splits an algorithm string into the (kty, keySize,
// curve) triple the SDK expects. The validator runs first so callers
// can rely on shape; unrecognised input falls back to RSA-2048.
func parseKeyAlgorithm(alg string) (kty string, size int32, curve string) {
	switch strings.ToUpper(strings.TrimSpace(alg)) {
	case "RSA-3072":
		return "RSA", 3072, ""
	case "RSA-4096":
		return "RSA", 4096, ""
	case "EC-P256":
		return "EC", 0, "P-256"
	case "EC-P384":
		return "EC", 0, "P-384"
	case "EC-P521":
		return "EC", 0, "P-521"
	default:
		return "RSA", 2048, ""
	}
}

// keyCreatedMsg is the async result of createKeyCmd.
type keyCreatedMsg struct {
	vaultName string
	keyName   string
	err       error
}

func createKeyCmd(svc *keyvault.Service, vault keyvault.Vault, name, kty string, size int32, curve string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := svc.CreateKey(ctx, vault, name, kty, size, curve)
		return keyCreatedMsg{vaultName: vault.Name, keyName: name, err: err}
	}
}

// handleKeyCreated reports the outcome and refreshes the keys list so
// the new entry appears without manual refresh. Mirrors handleSecretCreated.
func (m Model) handleKeyCreated(msg keyCreatedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to create key: %s", msg.err.Error()))
		return m, nil
	}
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Created key %s in %s", msg.keyName, msg.vaultName))
	if !m.hasVault || m.currentVault.Name != msg.vaultName || m.kvKind != kvKindKeys {
		return m, nil
	}
	m.startLoading(secretsPane, fmt.Sprintf("Refreshing keys in %s", m.currentVault.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchKeysCmd(m.service, m.cache.keys, m.currentVault, m.keys))
}

// certImportedMsg is the async result of importCertificateCmd.
type certImportedMsg struct {
	vaultName string
	certName  string
	err       error
}

func importCertificateCmd(svc *keyvault.Service, vault keyvault.Vault, name, path, password string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		pfx, err := os.ReadFile(path)
		if err != nil {
			return certImportedMsg{vaultName: vault.Name, certName: name, err: fmt.Errorf("read %s: %w", path, err)}
		}
		err = svc.ImportCertificate(ctx, vault, name, pfx, password)
		return certImportedMsg{vaultName: vault.Name, certName: name, err: err}
	}
}

// handleCertImported reports the outcome and refreshes the certs list
// so the new entry shows up without manual refresh.
func (m Model) handleCertImported(msg certImportedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to import certificate: %s", msg.err.Error()))
		return m, nil
	}
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Imported certificate %s into %s", msg.certName, msg.vaultName))
	if !m.hasVault || m.currentVault.Name != msg.vaultName || m.kvKind != kvKindCertificates {
		return m, nil
	}
	m.startLoading(secretsPane, fmt.Sprintf("Refreshing certificates in %s", m.currentVault.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchCertsCmd(m.service, m.cache.certs, m.currentVault, m.certs))
}

// confirmDelete opens the confirm modal for a delete-by-name. The
// message reflects soft-delete semantics — the vault keeps a recovery
// copy for the configured retention window. Returns no command; the
// confirmAction closure is invoked when the user confirms.
func (m Model) confirmDelete(name, kindLabel string, cmd tea.Cmd) (Model, tea.Cmd) {
	body := fmt.Sprintf("%s will be moved to the recovery bin. With soft-delete enabled it can be restored or purged from there.", name)
	m.confirmModal.OpenWithBreadcrumb(
		fmt.Sprintf("Delete %s", kindLabel),
		[]string{m.currentVault.Name, name},
		body,
		"delete", "cancel", true,
	)
	m.confirmAction = func() tea.Cmd { return cmd }
	return m, nil
}

// crudDoneMsg carries a finished-CRUD outcome with a level + message.
// Mirrors blobapp's pattern so the update.go handler can post one
// notification and trigger a list refresh in a single switch arm.
type crudDoneMsg struct {
	level   appshell.NotificationLevel
	message string
	// kind tells the refresh path which list to re-fetch. Empty means
	// "no refresh" (treated as a purely informational message).
	kind kvKind
}

func deleteSecretCmd(svc *keyvault.Service, vault keyvault.Vault, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := svc.DeleteSecret(ctx, vault, name); err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete %s failed: %v", name, err), kind: kvKindSecrets}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted secret %s", name), kind: kvKindSecrets}
	}
}

func deleteCertificateCmd(svc *keyvault.Service, vault keyvault.Vault, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := svc.DeleteCertificate(ctx, vault, name); err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete %s failed: %v", name, err), kind: kvKindCertificates}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted certificate %s", name), kind: kvKindCertificates}
	}
}

func deleteKeyCmd(svc *keyvault.Service, vault keyvault.Vault, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := svc.DeleteKey(ctx, vault, name); err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete %s failed: %v", name, err), kind: kvKindKeys}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted key %s", name), kind: kvKindKeys}
	}
}

// handleCrudDone posts the message and refreshes the matching list.
// Refresh only fires on success and only when the result kind matches
// the active kind — switching kinds mid-operation shouldn't trigger
// an unexpected refresh.
func (m Model) handleCrudDone(msg crudDoneMsg) (Model, tea.Cmd) {
	m.Notify(msg.level, msg.message)
	if msg.level != appshell.LevelSuccess || !m.hasVault || m.kvKind != msg.kind {
		return m, nil
	}
	switch msg.kind {
	case kvKindSecrets:
		return m, fetchSecretsCmd(m.service, m.cache.secrets, m.currentVault, m.secrets)
	case kvKindCertificates:
		return m, fetchCertsCmd(m.service, m.cache.certs, m.currentVault, m.certs)
	case kvKindKeys:
		return m, fetchKeysCmd(m.service, m.cache.keys, m.currentVault, m.keys)
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
