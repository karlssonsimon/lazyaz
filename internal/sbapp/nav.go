package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"

	tea "charm.land/bubbletea/v2"
)

// OpenBlobReferenceMsg asks the parent app to open a Blob tab and land
// on a specific blob inside a specific account/container — emitted when
// the user activates the "Open blob in explorer" action on a Service
// Bus message that carries an Event Grid Microsoft.Storage.Blob*
// reference. Subscription is filled in by the sbapp from its known
// subscriptions list (matching by SubscriptionID); when the ref's
// subscription is unknown, the sbapp passes a stub with only the ID
// set so the parent can still attempt to open the tab.
type OpenBlobReferenceMsg struct {
	Subscription  azure.Subscription
	AccountName   string
	ContainerName string
	Prefix        string
	BlobName      string
}

// openBlobReferenceCmd packages an OpenBlobReferenceMsg as a tea.Cmd.
func openBlobReferenceCmd(sub azure.Subscription, ref BlobRef) tea.Cmd {
	return func() tea.Msg {
		return OpenBlobReferenceMsg{
			Subscription:  sub,
			AccountName:   ref.AccountName,
			ContainerName: ref.ContainerName,
			Prefix:        ref.Prefix,
			BlobName:      ref.BlobName,
		}
	}
}

// resolveSubscription finds the full azure.Subscription for the given
// ID in the model's known subscriptions, falling back to a stub
// containing only the ID. The stub still drives openBlobTabWithNav —
// the tab will display the ID until the subscriptions broker hydrates
// the name.
func (m Model) resolveSubscription(id string) azure.Subscription {
	for _, s := range m.Subscriptions {
		if s.ID == id {
			return s
		}
	}
	return azure.Subscription{ID: id}
}
