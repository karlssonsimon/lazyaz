package sbapp

import (
	"context"
	"time"

	"azure-storage/internal/appshell"
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/cache"

	tea "github.com/charmbracelet/bubbletea"
)

func fetchSubscriptionsCmd(svc *servicebus.Service, loader *cache.Loader[azure.Subscription], fresh bool) tea.Cmd {
	fetchFn := func(ctx context.Context, send func([]azure.Subscription)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListSubscriptions(ctx, send)
	}
	wrap := func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	}
	if fresh {
		return loader.FetchFresh("", fetchFn, wrap)
	}
	return loader.Fetch("", fetchFn, wrap)
}

func fetchNamespacesCmd(svc *servicebus.Service, loader *cache.Loader[servicebus.Namespace], subscriptionID string) tea.Cmd {
	return loader.Fetch(subscriptionID, func(ctx context.Context, send func([]servicebus.Namespace)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListNamespaces(ctx, subscriptionID, send)
	}, func(p cache.Page[servicebus.Namespace]) tea.Msg {
		return namespacesLoadedMsg{subscriptionID: subscriptionID, namespaces: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchEntitiesCmd(svc *servicebus.Service, loader *cache.Loader[servicebus.Entity], ns servicebus.Namespace, cacheKey string) tea.Cmd {
	return loader.Fetch(cacheKey, func(ctx context.Context, send func([]servicebus.Entity)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListEntities(ctx, ns, send)
	}, func(p cache.Page[servicebus.Entity]) tea.Msg {
		return entitiesLoadedMsg{namespace: ns, entities: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchTopicSubscriptionsCmd(svc *servicebus.Service, loader *cache.Loader[servicebus.TopicSubscription], ns servicebus.Namespace, topicName string, cacheKey string) tea.Cmd {
	return loader.Fetch(cacheKey, func(ctx context.Context, send func([]servicebus.TopicSubscription)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListTopicSubscriptions(ctx, ns, topicName, send)
	}, func(p cache.Page[servicebus.TopicSubscription]) tea.Msg {
		return topicSubscriptionsLoadedMsg{namespace: ns, topicName: topicName, subs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func peekQueueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, queueName string, deadLetter bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var messages []servicebus.PeekedMessage
		err := svc.PeekQueueMessages(ctx, ns, queueName, peekMaxMessages, deadLetter, func(batch []servicebus.PeekedMessage) {
			messages = append(messages, batch...)
		})
		return messagesLoadedMsg{namespace: ns, source: queueName, messages: messages, err: err}
	}
}

func peekSubscriptionMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, topicName, subName string, deadLetter bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var messages []servicebus.PeekedMessage
		err := svc.PeekSubscriptionMessages(ctx, ns, topicName, subName, peekMaxMessages, deadLetter, func(batch []servicebus.PeekedMessage) {
			messages = append(messages, batch...)
		})
		return messagesLoadedMsg{namespace: ns, source: topicName + "/" + subName, messages: messages, err: err}
	}
}

func refreshEntitiesCmd(svc *servicebus.Service, ns servicebus.Namespace) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var entities []servicebus.Entity
		err := svc.ListEntities(ctx, ns, func(batch []servicebus.Entity) {
			entities = append(entities, batch...)
		})
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
