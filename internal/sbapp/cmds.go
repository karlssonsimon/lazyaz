package sbapp

import (
	"context"
	"time"

	"azure-storage/internal/servicebus"

	tea "github.com/charmbracelet/bubbletea"
)

func loadSubscriptionsCmd(svc *servicebus.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		subs, err := svc.ListSubscriptions(ctx)
		return subscriptionsLoadedMsg{subscriptions: subs, err: err}
	}
}

func loadNamespacesCmd(svc *servicebus.Service, subscriptionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		namespaces, err := svc.ListNamespaces(ctx, subscriptionID)
		return namespacesLoadedMsg{subscriptionID: subscriptionID, namespaces: namespaces, err: err}
	}
}

func loadEntitiesCmd(svc *servicebus.Service, ns servicebus.Namespace) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		entities, err := svc.ListEntities(ctx, ns)
		return entitiesLoadedMsg{namespace: ns, entities: entities, err: err}
	}
}

func loadTopicSubscriptionsCmd(svc *servicebus.Service, ns servicebus.Namespace, topicName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		subs, err := svc.ListTopicSubscriptions(ctx, ns, topicName)
		return topicSubscriptionsLoadedMsg{namespace: ns, topicName: topicName, subs: subs, err: err}
	}
}

func peekQueueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, queueName string, deadLetter bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		messages, err := svc.PeekQueueMessages(ctx, ns, queueName, peekMaxMessages, deadLetter)
		return messagesLoadedMsg{namespace: ns, source: queueName, messages: messages, err: err}
	}
}

func refreshEntitiesCmd(svc *servicebus.Service, ns servicebus.Namespace) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		entities, err := svc.ListEntities(ctx, ns)
		return entitiesRefreshedMsg{entities: entities, err: err}
	}
}

func requeueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, entity servicebus.Entity, isTopicSub bool, topicSub servicebus.TopicSubscription, messageIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var requeued int
		var err error
		if entity.Kind == servicebus.EntityQueue {
			requeued, err = svc.RequeueFromDLQ(ctx, ns, entity.Name, messageIDs)
		} else if isTopicSub {
			requeued, err = svc.RequeueFromSubscriptionDLQ(ctx, ns, entity.Name, topicSub.Name, messageIDs)
		}
		return requeueDoneMsg{requeued: requeued, total: len(messageIDs), err: err}
	}
}

func deleteDuplicateCmd(svc *servicebus.Service, ns servicebus.Namespace, entity servicebus.Entity, isTopicSub bool, topicSub servicebus.TopicSubscription, messageID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var err error
		if entity.Kind == servicebus.EntityQueue {
			err = svc.DeleteFromDLQ(ctx, ns, entity.Name, messageID)
		} else if isTopicSub {
			err = svc.DeleteFromSubscriptionDLQ(ctx, ns, entity.Name, topicSub.Name, messageID)
		}
		return deleteDuplicateDoneMsg{messageID: messageID, err: err}
	}
}

func peekSubscriptionMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, topicName, subName string, deadLetter bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		messages, err := svc.PeekSubscriptionMessages(ctx, ns, topicName, subName, peekMaxMessages, deadLetter)
		return messagesLoadedMsg{namespace: ns, source: topicName + "/" + subName, messages: messages, err: err}
	}
}
