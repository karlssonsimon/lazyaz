package dashapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	tea "charm.land/bubbletea/v2"
)

// OpenSBNamespaceMsg asks the parent app to open a Service Bus tab on
// the given subscription, with the namespace pre-selected. Used by the
// namespace counts widget's "open" action.
type OpenSBNamespaceMsg struct {
	Subscription azure.Subscription
	Namespace    servicebus.Namespace
}

// OpenSBEntityMsg asks the parent app to open a Service Bus tab and
// drill all the way down to a specific entity's DLQ messages. SubName
// is empty for queues; non-empty for topic-subscription DLQs.
type OpenSBEntityMsg struct {
	Subscription azure.Subscription
	Namespace    servicebus.Namespace
	EntityName   string
	SubName      string // empty for queues
	DeadLetter   bool   // true to land on the DLQ pane
}

// openSBNamespaceCmd packages an OpenSBNamespaceMsg as a tea.Cmd so a
// widget Action can return it without depending on tea internals.
func openSBNamespaceCmd(sub azure.Subscription, ns servicebus.Namespace) tea.Cmd {
	return func() tea.Msg {
		return OpenSBNamespaceMsg{Subscription: sub, Namespace: ns}
	}
}

// openSBEntityCmd packages an OpenSBEntityMsg.
func openSBEntityCmd(sub azure.Subscription, ns servicebus.Namespace, entity, subName string, deadLetter bool) tea.Cmd {
	return func() tea.Msg {
		return OpenSBEntityMsg{
			Subscription: sub,
			Namespace:    ns,
			EntityName:   entity,
			SubName:      subName,
			DeadLetter:   deadLetter,
		}
	}
}

// OpenBlobAccountMsg asks the parent app to open a Blob tab on the
// given subscription with the storage account pre-selected. Used by
// the blob usage widget's "open" action.
type OpenBlobAccountMsg struct {
	Subscription azure.Subscription
	AccountName  string
}

// OpenBlobContainerMsg asks the parent app to open a Blob tab and
// drill all the way to a specific container under an account.
type OpenBlobContainerMsg struct {
	Subscription  azure.Subscription
	AccountName   string
	ContainerName string
}

func openBlobAccountCmd(sub azure.Subscription, account string) tea.Cmd {
	return func() tea.Msg {
		return OpenBlobAccountMsg{Subscription: sub, AccountName: account}
	}
}

func openBlobContainerCmd(sub azure.Subscription, account, container string) tea.Cmd {
	return func() tea.Msg {
		return OpenBlobContainerMsg{Subscription: sub, AccountName: account, ContainerName: container}
	}
}
