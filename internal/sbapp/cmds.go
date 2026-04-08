package sbapp

import (
	"context"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
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

func fetchNamespacesCmd(svc *servicebus.Service, loader *cache.Loader[servicebus.Namespace], subscriptionID string, gen int) tea.Cmd {
	return loader.Fetch(subscriptionID, func(ctx context.Context, send func([]servicebus.Namespace)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListNamespaces(ctx, subscriptionID, send)
	}, func(p cache.Page[servicebus.Namespace]) tea.Msg {
		return namespacesLoadedMsg{gen: gen, subscriptionID: subscriptionID, namespaces: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchEntitiesCmd(svc *servicebus.Service, loader *cache.Loader[servicebus.Entity], ns servicebus.Namespace, cacheKey string, gen int) tea.Cmd {
	return loader.Fetch(cacheKey, func(ctx context.Context, send func([]servicebus.Entity)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListEntities(ctx, ns, send)
	}, func(p cache.Page[servicebus.Entity]) tea.Msg {
		return entitiesLoadedMsg{gen: gen, namespace: ns, entities: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchTopicSubscriptionsCmd(svc *servicebus.Service, loader *cache.Loader[servicebus.TopicSubscription], ns servicebus.Namespace, topicName string, cacheKey string, gen int) tea.Cmd {
	return loader.Fetch(cacheKey, func(ctx context.Context, send func([]servicebus.TopicSubscription)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListTopicSubscriptions(ctx, ns, topicName, send)
	}, func(p cache.Page[servicebus.TopicSubscription]) tea.Msg {
		return topicSubscriptionsLoadedMsg{gen: gen, namespace: ns, topicName: topicName, subs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func peekQueueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, queueName string, deadLetter, repeek bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var messages []servicebus.PeekedMessage
		err := svc.PeekQueueMessages(ctx, ns, queueName, peekMaxMessages, deadLetter, func(batch []servicebus.PeekedMessage) {
			messages = append(messages, batch...)
		})
		return messagesLoadedMsg{namespace: ns, source: queueName, messages: messages, repeek: repeek, err: err}
	}
}

func peekSubscriptionMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, topicName, subName string, deadLetter, repeek bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var messages []servicebus.PeekedMessage
		err := svc.PeekSubscriptionMessages(ctx, ns, topicName, subName, peekMaxMessages, deadLetter, func(batch []servicebus.PeekedMessage) {
			messages = append(messages, batch...)
		})
		return messagesLoadedMsg{namespace: ns, source: topicName + "/" + subName, messages: messages, repeek: repeek, err: err}
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

// requeueMessagesCmd requeues marked messages from a queue's DLQ or
// from a topic subscription's DLQ. When subName is empty, the entity
// itself is treated as a queue; otherwise the entity is the parent
// topic and subName is the subscription.
func requeueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, entity servicebus.Entity, subName string, messageIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var requeued int
		var err error
		if subName == "" {
			requeued, err = svc.RequeueFromDLQ(ctx, ns, entity.Name, messageIDs)
		} else {
			requeued, err = svc.RequeueFromSubscriptionDLQ(ctx, ns, entity.Name, subName, messageIDs)
		}
		return requeueDoneMsg{requeued: requeued, total: len(messageIDs), err: err}
	}
}

// deleteDuplicateCmd deletes a single duplicate message from a queue's
// DLQ or a topic subscription's DLQ. Same subName convention as
// requeueMessagesCmd.
func deleteDuplicateCmd(svc *servicebus.Service, ns servicebus.Namespace, entity servicebus.Entity, subName string, messageID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var err error
		if subName == "" {
			err = svc.DeleteFromDLQ(ctx, ns, entity.Name, messageID)
		} else {
			err = svc.DeleteFromSubscriptionDLQ(ctx, ns, entity.Name, subName, messageID)
		}
		return deleteDuplicateDoneMsg{messageID: messageID, err: err}
	}
}
