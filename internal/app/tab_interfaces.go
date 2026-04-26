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
