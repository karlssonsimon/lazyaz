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
