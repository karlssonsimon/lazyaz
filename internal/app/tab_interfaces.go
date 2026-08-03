package app

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

type notifyingTab interface {
	WithNotification(appshell.NotificationLevel, string) tea.Model
}

type subscriptionTab interface {
	CurrentSubscription() (azure.Subscription, bool)
	WithSubscription(azure.Subscription) tea.Model
	WithSubscriptions([]azure.Subscription) tea.Model
	WithoutSubscription([]azure.Subscription) tea.Model
}

type credentialTab interface {
	WithCredential(azcore.TokenCredential) tea.Model
}
type textInputTab interface{ IsTextInputActive() bool }

// searchableTab is implemented by tabs whose focused pane is a buffer
// that owns the vim search keys. Without this, ? and N would be consumed
// as help and notifications before ever reaching the buffer.
type searchableTab interface{ BufferSearchFocused() bool }
type themedTab interface{ WithScheme(ui.Scheme) tea.Model }
type helpTab interface{ HelpSections() []ui.HelpSection }
type uploadConflictTab interface {
	HasPendingUploadConflict() bool
	RenderUploadConflictPrompt(string, int, int) string
}
type navigationTab interface {
	CurrentNav() jumplist.NavSnapshot
	WithAppliedNav(jumplist.NavSnapshot) (tea.Model, tea.Cmd)
}

// CredentialResolver resolves a token credential for a given tenant ID.
// Implemented by the parent Model so tabs don't construct credentials
// themselves. The returned credential is shared across calls for the
// same tenant — do not cache or wrap it.
type CredentialResolver interface {
	CredentialFor(tenantID string) (azcore.TokenCredential, error)
}
