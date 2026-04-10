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

func peekQueueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, queueName string, deadLetter, repeek bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var messages []servicebus.PeekedMessage
		err := svc.PeekQueueMessages(ctx, ns, queueName, peekMaxMessages, deadLetter, func(batch []servicebus.PeekedMessage) {
			messages = append(messages, batch...)
		})
		return messagesLoadedMsg{namespace: ns, source: queueName, messages: messages, deadLetter: deadLetter, repeek: repeek, err: err}
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
		return messagesLoadedMsg{namespace: ns, source: topicName + "/" + subName, messages: messages, deadLetter: deadLetter, repeek: repeek, err: err}
	}
}

func receiveDLQCmd(svc *servicebus.Service, ns servicebus.Namespace, entityName, subName string, maxCount int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var result *servicebus.ReceivedMessages
		var err error
		if subName == "" {
			result, err = svc.ReceiveFromDLQ(ctx, ns, entityName, maxCount)
		} else {
			result, err = svc.ReceiveFromSubscriptionDLQ(ctx, ns, entityName, subName, maxCount)
		}
		return dlqReceivedMsg{result: result, err: err}
	}
}

func completeDLQMarkedCmd(locked *servicebus.ReceivedMessages, markedIDs map[string]struct{}) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var completed []string
		for _, msg := range locked.Messages {
			if _, ok := markedIDs[msg.MessageID]; !ok {
				continue
			}
			if err := locked.Complete(ctx, msg); err != nil {
				return dlqCompleteMsg{completed: completed, err: err}
			}
			completed = append(completed, msg.MessageID)
		}
		return dlqCompleteMsg{completed: completed}
	}
}

func requeueDLQMarkedCmd(svc *servicebus.Service, ns servicebus.Namespace, entityName string, locked *servicebus.ReceivedMessages, markedIDs map[string]struct{}) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var requeued []string
		for _, msg := range locked.Messages {
			if _, ok := markedIDs[msg.MessageID]; !ok {
				continue
			}
			if err := svc.SendToQueue(ctx, ns, entityName, msg.Body); err != nil {
				return dlqRequeueMsg{requeued: requeued, err: err}
			}
			if err := locked.Complete(ctx, msg); err != nil {
				return dlqRequeueMsg{requeued: requeued, err: err}
			}
			requeued = append(requeued, msg.MessageID)
		}
		return dlqRequeueMsg{requeued: requeued}
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

func requeueAllDLQCmd(svc *servicebus.Service, ns servicebus.Namespace, entityName, subName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		total := 0
		for {
			var result *servicebus.ReceivedMessages
			var err error
			if subName == "" {
				result, err = svc.ReceiveFromDLQ(ctx, ns, entityName, 50)
			} else {
				result, err = svc.ReceiveFromSubscriptionDLQ(ctx, ns, entityName, subName, 50)
			}
			if err != nil {
				return dlqRequeueAllMsg{requeued: total, err: err}
			}
			if len(result.Messages) == 0 {
				result.Receiver.Close(ctx)
				break
			}

			for _, msg := range result.Messages {
				if err := svc.SendToQueue(ctx, ns, entityName, msg.Body); err != nil {
					result.Close(ctx)
					return dlqRequeueAllMsg{requeued: total, err: err}
				}
				if err := result.Complete(ctx, msg); err != nil {
					result.Receiver.Close(ctx)
					return dlqRequeueAllMsg{requeued: total, err: err}
				}
				total++
			}
			result.Receiver.Close(ctx)
		}

		return dlqRequeueAllMsg{requeued: total}
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
