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

func fetchSubscriptionsCmd(svc *servicebus.Service, broker *cache.Broker[azure.Subscription], seed []azure.Subscription) tea.Cmd {
	cmd, _ := broker.Subscribe("", seed, func(ctx context.Context, send func([]azure.Subscription)) error {
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
	return cmd
}

func fetchNamespacesCmd(svc *servicebus.Service, broker *cache.Broker[servicebus.Namespace], subscriptionID string, seed []servicebus.Namespace) tea.Cmd {
	cmd, _ := broker.Subscribe(subscriptionID, seed, func(ctx context.Context, send func([]servicebus.Namespace)) error {
		return svc.ListNamespaces(ctx, subscriptionID, send)
	}, func(p cache.Page[servicebus.Namespace]) tea.Msg {
		return namespacesLoadedMsg{subscriptionID: subscriptionID, namespaces: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchEntitiesCmd(svc *servicebus.Service, broker *cache.Broker[servicebus.Entity], ns servicebus.Namespace, cacheKey string, seed []servicebus.Entity) tea.Cmd {
	cmd, _ := broker.Subscribe(cacheKey, seed, func(ctx context.Context, send func([]servicebus.Entity)) error {
		return svc.ListEntities(ctx, ns, send)
	}, func(p cache.Page[servicebus.Entity]) tea.Msg {
		return entitiesLoadedMsg{namespace: ns, entities: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchTopicSubscriptionsCmd(svc *servicebus.Service, broker *cache.Broker[servicebus.TopicSubscription], ns servicebus.Namespace, topicName string, cacheKey string, seed []servicebus.TopicSubscription) tea.Cmd {
	cmd, _ := broker.Subscribe(cacheKey, seed, func(ctx context.Context, send func([]servicebus.TopicSubscription)) error {
		return svc.ListTopicSubscriptions(ctx, ns, topicName, send)
	}, func(p cache.Page[servicebus.TopicSubscription]) tea.Msg {
		return topicSubscriptionsLoadedMsg{namespace: ns, topicName: topicName, subs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func peekQueueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, queueName string, deadLetter, repeek, preserveCursor bool, fromSeqNo int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var messages []servicebus.PeekedMessage
		err := svc.PeekQueueMessages(ctx, ns, queueName, peekMaxMessages, deadLetter, fromSeqNo, func(batch []servicebus.PeekedMessage) {
			messages = append(messages, batch...)
		})
		return messagesLoadedMsg{namespace: ns, source: queueName, messages: messages, deadLetter: deadLetter, repeek: repeek, preserveCursor: preserveCursor, err: err}
	}
}

func peekSubscriptionMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, topicName, subName string, deadLetter, repeek, preserveCursor bool, fromSeqNo int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var messages []servicebus.PeekedMessage
		err := svc.PeekSubscriptionMessages(ctx, ns, topicName, subName, peekMaxMessages, deadLetter, fromSeqNo, func(batch []servicebus.PeekedMessage) {
			messages = append(messages, batch...)
		})
		return messagesLoadedMsg{namespace: ns, source: topicName + "/" + subName, messages: messages, deadLetter: deadLetter, repeek: repeek, preserveCursor: preserveCursor, err: err}
	}
}

func receiveDLQCmd(svc *servicebus.Service, ns servicebus.Namespace, entityName, subName string, maxCount int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := svc.Receive(ctx, ns, entityName, subName, true, maxCount)
		return dlqReceivedMsg{result: result, err: err}
	}
}

func completeDLQMarkedCmd(locked *servicebus.ReceivedMessages, markedIDs map[string]struct{}) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var completed []string
		for _, msg := range locked.MessagesSnapshot() {
			key := msg.OperationID()
			if _, ok := markedIDs[key]; !ok {
				continue
			}
			if err := locked.CompleteByID(ctx, key); err != nil {
				return dlqCompleteMsg{completed: completed, err: err}
			}
			completed = append(completed, key)
		}
		return dlqCompleteMsg{completed: completed}
	}
}

func requeueDLQMarkedCmd(svc *servicebus.Service, ns servicebus.Namespace, entityName string, locked *servicebus.ReceivedMessages, markedIDs map[string]struct{}) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		requeued, err := svc.RequeueLockedByID(ctx, ns, entityName, locked, markedIDs)
		return dlqRequeueMsg{requeued: requeued, err: err}
	}
}

func abandonDLQCmd(locked *servicebus.ReceivedMessages) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		locked.Close(ctx)
		return dlqAbandonMsg{}
	}
}

func requeueAllDLQCmd(svc *servicebus.Service, ns servicebus.Namespace, entityName, subName string, count int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		total, err := svc.ResendAllFromDLQ(ctx, ns, entityName, subName, entityName, count)
		return dlqRequeueAllMsg{requeued: total, err: err}
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

// moveAllCmd receives all messages (from DLQ or active queue) and
// sends them to a different target queue/topic, then completes the originals.
func moveAllCmd(svc *servicebus.Service, sourceNS servicebus.Namespace, entityName, subName string, deadLetter bool, targetNS servicebus.Namespace, targetEntity string, count int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		total, err := svc.ResendAllFromSource(ctx, sourceNS, entityName, subName, deadLetter, targetNS, targetEntity, count)
		return moveAllDoneMsg{moved: total, deadLetter: deadLetter, err: err}
	}
}

// moveMarkedCmd sends marked locked messages to a different target queue/topic,
// then completes the originals.
func moveMarkedCmd(svc *servicebus.Service, targetNS servicebus.Namespace, targetEntity string, locked *servicebus.ReceivedMessages, markedIDs map[string]struct{}) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		moved, err := svc.RequeueLockedByID(ctx, targetNS, targetEntity, locked, markedIDs)
		return moveMarkedDoneMsg{moved: moved, err: err}
	}
}

// fetchTargetEntitiesCmd loads entities from a namespace for the target picker.
func fetchTargetEntitiesCmd(svc *servicebus.Service, ns servicebus.Namespace) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var entities []servicebus.Entity
		err := svc.ListEntities(ctx, ns, func(batch []servicebus.Entity) {
			entities = append(entities, batch...)
		})
		return targetEntitiesLoadedMsg{namespace: ns, entities: entities, err: err}
	}
}
