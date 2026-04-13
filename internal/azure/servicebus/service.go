package servicebus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/servicebus/armservicebus"
)

const maxBodyPreview = 512

type EntityKind int

const (
	EntityQueue EntityKind = iota
	EntityTopic
)

type Namespace struct {
	Name           string
	SubscriptionID string
	ResourceGroup  string
	FQDN           string
}

type Entity struct {
	Name            string
	Kind            EntityKind
	ActiveMsgCount  int64
	DeadLetterCount int64
}

type TopicSubscription struct {
	Name            string
	ActiveMsgCount  int64
	DeadLetterCount int64
}

// Key functions for cache deduplication.
func NamespaceKey(ns Namespace) string          { return ns.Name }
func EntityKey(e Entity) string                 { return e.Name }
func TopicSubscriptionKey(s TopicSubscription) string { return s.Name }

type PeekedMessage struct {
	MessageID      string
	SequenceNumber int64
	EnqueuedAt     time.Time
	BodyPreview    string
	FullBody       string
}

type Service struct {
	cred           azcore.TokenCredential
	mu             sync.Mutex
	clients        map[string]*azservicebus.Client
	connStrClients map[string]*azservicebus.Client
}

func NewService(cred azcore.TokenCredential) *Service {
	return &Service{
		cred:           cred,
		clients:        make(map[string]*azservicebus.Client),
		connStrClients: make(map[string]*azservicebus.Client),
	}
}

func (s *Service) getClient(fqdn string) (*azservicebus.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.clients[fqdn]; ok {
		return c, nil
	}

	c, err := azservicebus.NewClient(fqdn, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create service bus client for %s: %w", fqdn, err)
	}

	s.clients[fqdn] = c
	return c, nil
}

func (s *Service) getConnStrClient(ctx context.Context, ns Namespace) (*azservicebus.Client, error) {
	s.mu.Lock()
	if c, ok := s.connStrClients[ns.FQDN]; ok {
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	if strings.TrimSpace(ns.SubscriptionID) == "" || strings.TrimSpace(ns.ResourceGroup) == "" || strings.TrimSpace(ns.Name) == "" {
		return nil, fmt.Errorf("insufficient namespace metadata for connection string fallback")
	}

	nsClient, err := armservicebus.NewNamespacesClient(ns.SubscriptionID, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create namespaces client for keys: %w", err)
	}

	var ruleName string
	pager := nsClient.NewListAuthorizationRulesPager(ns.ResourceGroup, ns.Name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list auth rules for %s: %w", ns.Name, err)
		}
		for _, rule := range page.Value {
			if rule != nil && rule.Name != nil {
				ruleName = *rule.Name
				break
			}
		}
		if ruleName != "" {
			break
		}
	}
	if ruleName == "" {
		return nil, fmt.Errorf("no authorization rules found for namespace %s", ns.Name)
	}

	keysResp, err := nsClient.ListKeys(ctx, ns.ResourceGroup, ns.Name, ruleName, nil)
	if err != nil {
		return nil, fmt.Errorf("list keys for %s/%s: %w", ns.Name, ruleName, err)
	}
	if keysResp.PrimaryConnectionString == nil || *keysResp.PrimaryConnectionString == "" {
		return nil, fmt.Errorf("no primary connection string returned for %s/%s", ns.Name, ruleName)
	}

	c, err := azservicebus.NewClientFromConnectionString(*keysResp.PrimaryConnectionString, nil)
	if err != nil {
		return nil, fmt.Errorf("create service bus client from connection string for %s: %w", ns.Name, err)
	}

	s.mu.Lock()
	// Re-check under lock: another goroutine may have raced us.
	if existing, ok := s.connStrClients[ns.FQDN]; ok {
		s.mu.Unlock()
		return existing, nil
	}
	s.connStrClients[ns.FQDN] = c
	s.mu.Unlock()

	return c, nil
}

func (s *Service) ListSubscriptions(ctx context.Context, send func([]azure.Subscription)) error {
	return azure.ListSubscriptions(ctx, s.cred, send)
}

func (s *Service) ListNamespaces(ctx context.Context, subscriptionID string, send func([]Namespace)) error {
	client, err := armservicebus.NewNamespacesClient(subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create namespaces client: %w", err)
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list namespaces: %w", err)
		}

		var batch []Namespace
		for _, ns := range page.Value {
			if ns == nil || ns.Name == nil {
				continue
			}

			entry := Namespace{
				Name:           *ns.Name,
				SubscriptionID: subscriptionID,
				ResourceGroup:  parseResourceGroup(ns.ID),
			}
			if ns.Properties != nil && ns.Properties.ServiceBusEndpoint != nil {
				entry.FQDN = endpointToFQDN(*ns.Properties.ServiceBusEndpoint)
			}
			if entry.FQDN == "" {
				entry.FQDN = *ns.Name + ".servicebus.windows.net"
			}

			batch = append(batch, entry)
		}
		if len(batch) > 0 {
			send(batch)
		}
	}

	return nil
}

func (s *Service) ListEntities(ctx context.Context, ns Namespace, send func([]Entity)) error {
	client, err := armservicebus.NewQueuesClient(ns.SubscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create queues client: %w", err)
	}

	queuePager := client.NewListByNamespacePager(ns.ResourceGroup, ns.Name, nil)
	for queuePager.More() {
		page, err := queuePager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list queues in %s: %w", ns.Name, err)
		}
		var batch []Entity
		for _, q := range page.Value {
			if q == nil || q.Name == nil {
				continue
			}
			e := Entity{Name: *q.Name, Kind: EntityQueue}
			if q.Properties != nil && q.Properties.CountDetails != nil {
				if q.Properties.CountDetails.ActiveMessageCount != nil {
					e.ActiveMsgCount = *q.Properties.CountDetails.ActiveMessageCount
				}
				if q.Properties.CountDetails.DeadLetterMessageCount != nil {
					e.DeadLetterCount = *q.Properties.CountDetails.DeadLetterMessageCount
				}
			}
			batch = append(batch, e)
		}
		if len(batch) > 0 {
			send(batch)
		}
	}

	topicsClient, err := armservicebus.NewTopicsClient(ns.SubscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create topics client: %w", err)
	}

	topicPager := topicsClient.NewListByNamespacePager(ns.ResourceGroup, ns.Name, nil)
	for topicPager.More() {
		page, err := topicPager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list topics in %s: %w", ns.Name, err)
		}
		var batch []Entity
		for _, t := range page.Value {
			if t == nil || t.Name == nil {
				continue
			}
			e := Entity{Name: *t.Name, Kind: EntityTopic}
			if t.Properties != nil && t.Properties.CountDetails != nil {
				if t.Properties.CountDetails.ActiveMessageCount != nil {
					e.ActiveMsgCount = *t.Properties.CountDetails.ActiveMessageCount
				}
				if t.Properties.CountDetails.DeadLetterMessageCount != nil {
					e.DeadLetterCount = *t.Properties.CountDetails.DeadLetterMessageCount
				}
			}
			batch = append(batch, e)
		}
		if len(batch) > 0 {
			send(batch)
		}
	}

	return nil
}

func (s *Service) ListTopicSubscriptions(ctx context.Context, ns Namespace, topicName string, send func([]TopicSubscription)) error {
	client, err := armservicebus.NewSubscriptionsClient(ns.SubscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create subscriptions client: %w", err)
	}

	pager := client.NewListByTopicPager(ns.ResourceGroup, ns.Name, topicName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list subscriptions for topic %s: %w", topicName, err)
		}
		var batch []TopicSubscription
		for _, sub := range page.Value {
			if sub == nil || sub.Name == nil {
				continue
			}
			ts := TopicSubscription{Name: *sub.Name}
			if sub.Properties != nil && sub.Properties.CountDetails != nil {
				if sub.Properties.CountDetails.ActiveMessageCount != nil {
					ts.ActiveMsgCount = *sub.Properties.CountDetails.ActiveMessageCount
				}
				if sub.Properties.CountDetails.DeadLetterMessageCount != nil {
					ts.DeadLetterCount = *sub.Properties.CountDetails.DeadLetterMessageCount
				}
			}
			batch = append(batch, ts)
		}
		if len(batch) > 0 {
			send(batch)
		}
	}

	return nil
}

func (s *Service) PeekQueueMessages(ctx context.Context, ns Namespace, queueName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return err
	}

	err = s.peekQueue(ctx, client, queueName, maxCount, deadLetter, fromSequenceNumber, send)
	if err == nil || !isAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return fmt.Errorf("peek queue %s with AAD failed: %v; connection string fallback failed: %w", queueName, err, fallbackErr)
	}

	err = s.peekQueue(ctx, fallbackClient, queueName, maxCount, deadLetter, fromSequenceNumber, send)
	if err != nil {
		return fmt.Errorf("peek queue %s with connection string fallback: %w", queueName, err)
	}
	return nil
}

func (s *Service) peekQueue(ctx context.Context, client *azservicebus.Client, queueName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	opts := &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
	}
	if deadLetter {
		opts.SubQueue = azservicebus.SubQueueDeadLetter
	}
	receiver, err := client.NewReceiverForQueue(queueName, opts)
	if err != nil {
		return fmt.Errorf("create queue receiver for %s: %w", queueName, err)
	}
	defer receiver.Close(ctx)

	return peekMessages(ctx, receiver, maxCount, fromSequenceNumber, send)
}

func (s *Service) PeekSubscriptionMessages(ctx context.Context, ns Namespace, topicName, subName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return err
	}

	err = s.peekSubscription(ctx, client, topicName, subName, maxCount, deadLetter, fromSequenceNumber, send)
	if err == nil || !isAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return fmt.Errorf("peek subscription %s/%s with AAD failed: %v; connection string fallback failed: %w", topicName, subName, err, fallbackErr)
	}

	err = s.peekSubscription(ctx, fallbackClient, topicName, subName, maxCount, deadLetter, fromSequenceNumber, send)
	if err != nil {
		return fmt.Errorf("peek subscription %s/%s with connection string fallback: %w", topicName, subName, err)
	}
	return nil
}

func (s *Service) peekSubscription(ctx context.Context, client *azservicebus.Client, topicName, subName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	opts := &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
	}
	if deadLetter {
		opts.SubQueue = azservicebus.SubQueueDeadLetter
	}
	receiver, err := client.NewReceiverForSubscription(topicName, subName, opts)
	if err != nil {
		return fmt.Errorf("create subscription receiver for %s/%s: %w", topicName, subName, err)
	}
	defer receiver.Close(ctx)

	return peekMessages(ctx, receiver, maxCount, fromSequenceNumber, send)
}

// ReceivedMessages holds the result of a receive-with-lock operation.
// The caller owns the Receiver and must Close it when done. While open,
// messages can be completed (removed) or abandoned (lock released).
type ReceivedMessages struct {
	Messages []*azservicebus.ReceivedMessage
	Receiver *azservicebus.Receiver
}

// Complete removes a locked message from the queue.
func (r *ReceivedMessages) Complete(ctx context.Context, msg *azservicebus.ReceivedMessage) error {
	return r.Receiver.CompleteMessage(ctx, msg, nil)
}

// Abandon releases the lock on a message so it becomes available again.
func (r *ReceivedMessages) Abandon(ctx context.Context, msg *azservicebus.ReceivedMessage) error {
	return r.Receiver.AbandonMessage(ctx, msg, nil)
}

// AbandonAll releases locks on all messages.
func (r *ReceivedMessages) AbandonAll(ctx context.Context) {
	for _, msg := range r.Messages {
		_ = r.Receiver.AbandonMessage(ctx, msg, nil)
	}
}

// Close abandons all messages and closes the receiver.
func (r *ReceivedMessages) Close(ctx context.Context) {
	if r == nil || r.Receiver == nil {
		return
	}
	r.AbandonAll(ctx)
	r.Receiver.Close(ctx)
}

// ReceiveFromQueue receives messages from the active queue with peek-lock.
func (s *Service) ReceiveFromQueue(ctx context.Context, ns Namespace, queueName string, maxCount int) (*ReceivedMessages, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return nil, err
	}

	result, err := s.receiveFromQueue(ctx, client, queueName, maxCount)
	if err == nil || !isAuthError(err) {
		return result, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return nil, fmt.Errorf("receive from queue %s with AAD failed: %v; fallback failed: %w", queueName, err, fallbackErr)
	}
	return s.receiveFromQueue(ctx, fallbackClient, queueName, maxCount)
}

func (s *Service) receiveFromQueue(ctx context.Context, client *azservicebus.Client, queueName string, maxCount int) (*ReceivedMessages, error) {
	receiver, err := client.NewReceiverForQueue(queueName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
	})
	if err != nil {
		return nil, fmt.Errorf("create receiver for %s: %w", queueName, err)
	}

	messages, err := receiver.ReceiveMessages(ctx, maxCount, nil)
	if err != nil {
		receiver.Close(ctx)
		return nil, fmt.Errorf("receive from queue %s: %w", queueName, err)
	}

	return &ReceivedMessages{Messages: messages, Receiver: receiver}, nil
}

// ReceiveFromSubscription receives messages from a topic subscription with peek-lock.
func (s *Service) ReceiveFromSubscription(ctx context.Context, ns Namespace, topicName, subName string, maxCount int) (*ReceivedMessages, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return nil, err
	}

	result, err := s.receiveFromSub(ctx, client, topicName, subName, maxCount)
	if err == nil || !isAuthError(err) {
		return result, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return nil, fmt.Errorf("receive from subscription %s/%s with AAD failed: %v; fallback failed: %w", topicName, subName, err, fallbackErr)
	}
	return s.receiveFromSub(ctx, fallbackClient, topicName, subName, maxCount)
}

func (s *Service) receiveFromSub(ctx context.Context, client *azservicebus.Client, topicName, subName string, maxCount int) (*ReceivedMessages, error) {
	receiver, err := client.NewReceiverForSubscription(topicName, subName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
	})
	if err != nil {
		return nil, fmt.Errorf("create receiver for %s/%s: %w", topicName, subName, err)
	}

	messages, err := receiver.ReceiveMessages(ctx, maxCount, nil)
	if err != nil {
		receiver.Close(ctx)
		return nil, fmt.Errorf("receive from subscription %s/%s: %w", topicName, subName, err)
	}

	return &ReceivedMessages{Messages: messages, Receiver: receiver}, nil
}

func (s *Service) ReceiveFromDLQ(ctx context.Context, ns Namespace, queueName string, maxCount int) (*ReceivedMessages, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return nil, err
	}

	result, err := s.receiveFromQueueDLQ(ctx, client, queueName, maxCount)
	if err == nil || !isAuthError(err) {
		return result, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return nil, fmt.Errorf("receive from DLQ %s with AAD failed: %v; fallback failed: %w", queueName, err, fallbackErr)
	}
	return s.receiveFromQueueDLQ(ctx, fallbackClient, queueName, maxCount)
}

func (s *Service) receiveFromQueueDLQ(ctx context.Context, client *azservicebus.Client, queueName string, maxCount int) (*ReceivedMessages, error) {
	receiver, err := client.NewReceiverForQueue(queueName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
		SubQueue:    azservicebus.SubQueueDeadLetter,
	})
	if err != nil {
		return nil, fmt.Errorf("create DLQ receiver for %s: %w", queueName, err)
	}

	messages, err := receiver.ReceiveMessages(ctx, maxCount, nil)
	if err != nil {
		receiver.Close(ctx)
		return nil, fmt.Errorf("receive from DLQ %s: %w", queueName, err)
	}

	return &ReceivedMessages{Messages: messages, Receiver: receiver}, nil
}

func (s *Service) ReceiveFromSubscriptionDLQ(ctx context.Context, ns Namespace, topicName, subName string, maxCount int) (*ReceivedMessages, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return nil, err
	}

	result, err := s.receiveFromSubDLQ(ctx, client, topicName, subName, maxCount)
	if err == nil || !isAuthError(err) {
		return result, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return nil, fmt.Errorf("receive from subscription DLQ %s/%s with AAD failed: %v; fallback failed: %w", topicName, subName, err, fallbackErr)
	}
	return s.receiveFromSubDLQ(ctx, fallbackClient, topicName, subName, maxCount)
}

func (s *Service) receiveFromSubDLQ(ctx context.Context, client *azservicebus.Client, topicName, subName string, maxCount int) (*ReceivedMessages, error) {
	receiver, err := client.NewReceiverForSubscription(topicName, subName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
		SubQueue:    azservicebus.SubQueueDeadLetter,
	})
	if err != nil {
		return nil, fmt.Errorf("create DLQ receiver for %s/%s: %w", topicName, subName, err)
	}

	messages, err := receiver.ReceiveMessages(ctx, maxCount, nil)
	if err != nil {
		receiver.Close(ctx)
		return nil, fmt.Errorf("receive from DLQ %s/%s: %w", topicName, subName, err)
	}

	return &ReceivedMessages{Messages: messages, Receiver: receiver}, nil
}

// toSendableMessage converts a received message to a sendable message,
// preserving properties like SessionID, ContentType, etc.
func toSendableMessage(orig *azservicebus.ReceivedMessage) *azservicebus.Message {
	msg := &azservicebus.Message{
		Body:             orig.Body,
		SessionID:        orig.SessionID,
		ContentType:      orig.ContentType,
		CorrelationID:    orig.CorrelationID,
		Subject:          orig.Subject,
		MessageID:        &orig.MessageID,
		To:               orig.To,
		ReplyTo:          orig.ReplyTo,
		ReplyToSessionID: orig.ReplyToSessionID,
	}
	if len(orig.ApplicationProperties) > 0 {
		msg.ApplicationProperties = orig.ApplicationProperties
	}
	return msg
}

// SendBatch sends multiple messages to a queue or topic using batched sends.
func (s *Service) SendBatch(ctx context.Context, ns Namespace, queueOrTopicName string, messages []*azservicebus.ReceivedMessage) error {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return err
	}

	err = s.sendBatch(ctx, client, queueOrTopicName, messages)
	if err == nil || !isAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return fmt.Errorf("send batch to %s with AAD failed: %v; fallback failed: %w", queueOrTopicName, err, fallbackErr)
	}
	return s.sendBatch(ctx, fallbackClient, queueOrTopicName, messages)
}

func (s *Service) sendBatch(ctx context.Context, client *azservicebus.Client, name string, messages []*azservicebus.ReceivedMessage) error {
	sender, err := client.NewSender(name, nil)
	if err != nil {
		return fmt.Errorf("create sender for %s: %w", name, err)
	}
	defer sender.Close(ctx)

	batch, err := sender.NewMessageBatch(ctx, nil)
	if err != nil {
		return fmt.Errorf("create message batch for %s: %w", name, err)
	}

	for _, orig := range messages {
		err := batch.AddMessage(toSendableMessage(orig), nil)
		if err == nil {
			continue
		}
		// Batch is full — send what we have and start a new one.
		if err := sender.SendMessageBatch(ctx, batch, nil); err != nil {
			return fmt.Errorf("send batch to %s: %w", name, err)
		}
		batch, err = sender.NewMessageBatch(ctx, nil)
		if err != nil {
			return fmt.Errorf("create message batch for %s: %w", name, err)
		}
		// Retry adding the message that didn't fit.
		if err := batch.AddMessage(toSendableMessage(orig), nil); err != nil {
			return fmt.Errorf("message %s too large for batch: %w", orig.MessageID, err)
		}
	}

	if batch.NumMessages() > 0 {
		if err := sender.SendMessageBatch(ctx, batch, nil); err != nil {
			return fmt.Errorf("send batch to %s: %w", name, err)
		}
	}
	return nil
}

// resendAll receives all messages from a receiver, batch-sends them to
// targetName via the given client, and completes the originals. The sender
// and receiver are kept open for the entire operation to avoid repeated
// AMQP handshakes. When maxMessages > 0 the loop stops after that many
// messages to avoid infinite cycles when a consumer immediately dead-letters
// resent messages. Returns the number of messages successfully resent.
func (s *Service) resendAll(ctx context.Context, client *azservicebus.Client, receiver *azservicebus.Receiver, targetName string, batchSize, maxMessages int) (int, error) {
	sender, err := client.NewSender(targetName, nil)
	if err != nil {
		return 0, fmt.Errorf("create sender for %s: %w", targetName, err)
	}
	defer sender.Close(ctx)

	total := 0
	for {
		receiveCtx, receiveCancel := context.WithTimeout(ctx, 15*time.Second)
		messages, err := receiver.ReceiveMessages(receiveCtx, batchSize, &azservicebus.ReceiveMessagesOptions{
			TimeAfterFirstMessage: 1 * time.Second,
		})
		receiveCancel()
		if err != nil && len(messages) == 0 {
			if ctx.Err() == nil && total > 0 {
				break
			}
			return total, fmt.Errorf("receive messages: %w", err)
		}
		if len(messages) == 0 {
			break
		}

		// Batch-send all received messages.
		batch, err := sender.NewMessageBatch(ctx, nil)
		if err != nil {
			return total, fmt.Errorf("create message batch: %w", err)
		}
		for _, msg := range messages {
			if err := batch.AddMessage(toSendableMessage(msg), nil); err != nil {
				// Batch full — flush and start a new one.
				if err := sender.SendMessageBatch(ctx, batch, nil); err != nil {
					return total, fmt.Errorf("send batch: %w", err)
				}
				batch, err = sender.NewMessageBatch(ctx, nil)
				if err != nil {
					return total, fmt.Errorf("create message batch: %w", err)
				}
				if err := batch.AddMessage(toSendableMessage(msg), nil); err != nil {
					return total, fmt.Errorf("message %s too large for batch: %w", msg.MessageID, err)
				}
			}
		}
		if batch.NumMessages() > 0 {
			if err := sender.SendMessageBatch(ctx, batch, nil); err != nil {
				return total, fmt.Errorf("send batch: %w", err)
			}
		}

		// Complete originals.
		for _, msg := range messages {
			if err := receiver.CompleteMessage(ctx, msg, nil); err != nil {
				return total, fmt.Errorf("complete message %s: %w", msg.MessageID, err)
			}
			total++
		}

		if maxMessages > 0 && total >= maxMessages {
			break
		}
	}
	return total, nil
}

// ResendAllFromDLQ receives all DLQ messages and resends them to the target
// entity using a single receiver and sender for the entire operation.
// expectedCount is used to clamp the receive batch size when known (pass 0 to use the default).
func (s *Service) ResendAllFromDLQ(ctx context.Context, ns Namespace, entityName, subName, targetName string, expectedCount int) (int, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return 0, err
	}

	n, err := s.resendAllFromDLQ(ctx, client, entityName, subName, targetName, expectedCount)
	if err == nil || !isAuthError(err) {
		return n, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return n, fmt.Errorf("resend DLQ with AAD failed: %v; fallback failed: %w", err, fallbackErr)
	}
	return s.resendAllFromDLQ(ctx, fallbackClient, entityName, subName, targetName, expectedCount)
}

func (s *Service) resendAllFromDLQ(ctx context.Context, client *azservicebus.Client, entityName, subName, targetName string, expectedCount int) (int, error) {
	var receiver *azservicebus.Receiver
	var err error
	if subName == "" {
		receiver, err = client.NewReceiverForQueue(entityName, &azservicebus.ReceiverOptions{
			ReceiveMode: azservicebus.ReceiveModePeekLock,
			SubQueue:    azservicebus.SubQueueDeadLetter,
		})
	} else {
		receiver, err = client.NewReceiverForSubscription(entityName, subName, &azservicebus.ReceiverOptions{
			ReceiveMode: azservicebus.ReceiveModePeekLock,
			SubQueue:    azservicebus.SubQueueDeadLetter,
		})
	}
	if err != nil {
		return 0, fmt.Errorf("create DLQ receiver: %w", err)
	}
	defer receiver.Close(ctx)

	batchSize := 50
	if expectedCount > 0 && expectedCount < batchSize {
		batchSize = expectedCount
	}
	return s.resendAll(ctx, client, receiver, targetName, batchSize, expectedCount)
}

// ResendAllFromSource receives all messages from a source (active or DLQ) and
// resends them to a target entity using a single receiver and sender.
func (s *Service) ResendAllFromSource(ctx context.Context, ns Namespace, entityName, subName string, deadLetter bool, targetNS Namespace, targetName string, expectedCount int) (int, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return 0, err
	}

	targetClient, err := s.getClient(targetNS.FQDN)
	if err != nil {
		return 0, err
	}

	var receiver *azservicebus.Receiver
	opts := &azservicebus.ReceiverOptions{ReceiveMode: azservicebus.ReceiveModePeekLock}
	if deadLetter {
		opts.SubQueue = azservicebus.SubQueueDeadLetter
	}
	if subName == "" {
		receiver, err = client.NewReceiverForQueue(entityName, opts)
	} else {
		receiver, err = client.NewReceiverForSubscription(entityName, subName, opts)
	}
	if err != nil {
		return 0, fmt.Errorf("create receiver: %w", err)
	}
	defer receiver.Close(ctx)

	batchSize := 50
	if expectedCount > 0 && expectedCount < batchSize {
		batchSize = expectedCount
	}
	return s.resendAll(ctx, targetClient, receiver, targetName, batchSize, expectedCount)
}

type DuplicateError struct {
	MessageID string
	Err       error
}

func (e *DuplicateError) Error() string {
	return fmt.Sprintf("message %s sent but not removed from DLQ (possible duplicate): %v", e.MessageID, e.Err)
}

func (e *DuplicateError) Unwrap() error { return e.Err }

func (s *Service) RequeueFromDLQ(ctx context.Context, ns Namespace, queueName string, messageIDs []string) (int, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return 0, err
	}

	n, err := s.requeueFromDLQ(ctx, client, queueName, messageIDs)
	if err == nil || !isAuthError(err) {
		return n, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return n, fmt.Errorf("requeue from DLQ %s with AAD failed: %v; connection string fallback failed: %w", queueName, err, fallbackErr)
	}

	return s.requeueFromDLQ(ctx, fallbackClient, queueName, messageIDs)
}

func (s *Service) requeueFromDLQ(ctx context.Context, client *azservicebus.Client, queueName string, messageIDs []string) (int, error) {
	receiver, err := client.NewReceiverForQueue(queueName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
		SubQueue:    azservicebus.SubQueueDeadLetter,
	})
	if err != nil {
		return 0, fmt.Errorf("create DLQ receiver for %s: %w", queueName, err)
	}
	defer receiver.Close(ctx)

	sender, err := client.NewSender(queueName, nil)
	if err != nil {
		return 0, fmt.Errorf("create sender for %s: %w", queueName, err)
	}
	defer sender.Close(ctx)

	return requeueMessages(ctx, receiver, sender, messageIDs)
}

func (s *Service) RequeueFromSubscriptionDLQ(ctx context.Context, ns Namespace, topicName string, subName string, messageIDs []string) (int, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return 0, err
	}

	n, err := s.requeueFromSubscriptionDLQ(ctx, client, topicName, subName, messageIDs)
	if err == nil || !isAuthError(err) {
		return n, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return n, fmt.Errorf("requeue from subscription DLQ %s/%s with AAD failed: %v; connection string fallback failed: %w", topicName, subName, err, fallbackErr)
	}

	return s.requeueFromSubscriptionDLQ(ctx, fallbackClient, topicName, subName, messageIDs)
}

func (s *Service) requeueFromSubscriptionDLQ(ctx context.Context, client *azservicebus.Client, topicName string, subName string, messageIDs []string) (int, error) {
	receiver, err := client.NewReceiverForSubscription(topicName, subName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
		SubQueue:    azservicebus.SubQueueDeadLetter,
	})
	if err != nil {
		return 0, fmt.Errorf("create DLQ receiver for %s/%s: %w", topicName, subName, err)
	}
	defer receiver.Close(ctx)

	sender, err := client.NewSender(topicName, nil)
	if err != nil {
		return 0, fmt.Errorf("create sender for %s: %w", topicName, err)
	}
	defer sender.Close(ctx)

	return requeueMessages(ctx, receiver, sender, messageIDs)
}

func requeueMessages(ctx context.Context, receiver *azservicebus.Receiver, sender *azservicebus.Sender, messageIDs []string) (int, error) {
	target := make(map[string]struct{}, len(messageIDs))
	for _, id := range messageIDs {
		target[id] = struct{}{}
	}

	requeued := 0
	seen := make(map[string]struct{})

	for len(target) > 0 {
		messages, err := receiver.ReceiveMessages(ctx, 1, nil)
		if err != nil {
			return requeued, fmt.Errorf("receive DLQ messages: %w", err)
		}
		if len(messages) == 0 {
			break
		}

		msg := messages[0]
		if _, ok := target[msg.MessageID]; ok {
			if err := sender.SendMessage(ctx, toSendableMessage(msg), nil); err != nil {
				_ = receiver.AbandonMessage(ctx, msg, nil)
				return requeued, fmt.Errorf("send message %s: %w", msg.MessageID, err)
			}
			if err := receiver.CompleteMessage(ctx, msg, nil); err != nil {
				return requeued, &DuplicateError{MessageID: msg.MessageID, Err: err}
			}
			requeued++
			delete(target, msg.MessageID)
			seen = make(map[string]struct{})
		} else {
			if _, alreadySeen := seen[msg.MessageID]; alreadySeen {
				_ = receiver.AbandonMessage(ctx, msg, nil)
				break
			}
			seen[msg.MessageID] = struct{}{}
			_ = receiver.AbandonMessage(ctx, msg, nil)
		}
	}

	if len(target) > 0 {
		return requeued, fmt.Errorf("%d message(s) not found in DLQ", len(target))
	}

	return requeued, nil
}

func (s *Service) DeleteFromDLQ(ctx context.Context, ns Namespace, queueName string, messageID string) error {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return err
	}

	err = s.deleteFromDLQ(ctx, client, queueName, messageID)
	if err == nil || !isAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return fmt.Errorf("delete from DLQ %s with AAD failed: %v; connection string fallback failed: %w", queueName, err, fallbackErr)
	}

	return s.deleteFromDLQ(ctx, fallbackClient, queueName, messageID)
}

func (s *Service) deleteFromDLQ(ctx context.Context, client *azservicebus.Client, queueName string, messageID string) error {
	receiver, err := client.NewReceiverForQueue(queueName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
		SubQueue:    azservicebus.SubQueueDeadLetter,
	})
	if err != nil {
		return fmt.Errorf("create DLQ receiver for %s: %w", queueName, err)
	}
	defer receiver.Close(ctx)

	msg, err := receiveByID(ctx, receiver, messageID)
	if err != nil {
		return err
	}

	return receiver.CompleteMessage(ctx, msg, nil)
}

func (s *Service) DeleteFromSubscriptionDLQ(ctx context.Context, ns Namespace, topicName string, subName string, messageID string) error {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return err
	}

	err = s.deleteFromSubscriptionDLQ(ctx, client, topicName, subName, messageID)
	if err == nil || !isAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return fmt.Errorf("delete from subscription DLQ %s/%s with AAD failed: %v; connection string fallback failed: %w", topicName, subName, err, fallbackErr)
	}

	return s.deleteFromSubscriptionDLQ(ctx, fallbackClient, topicName, subName, messageID)
}

func (s *Service) deleteFromSubscriptionDLQ(ctx context.Context, client *azservicebus.Client, topicName string, subName string, messageID string) error {
	receiver, err := client.NewReceiverForSubscription(topicName, subName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
		SubQueue:    azservicebus.SubQueueDeadLetter,
	})
	if err != nil {
		return fmt.Errorf("create DLQ receiver for %s/%s: %w", topicName, subName, err)
	}
	defer receiver.Close(ctx)

	msg, err := receiveByID(ctx, receiver, messageID)
	if err != nil {
		return err
	}

	return receiver.CompleteMessage(ctx, msg, nil)
}

func receiveByID(ctx context.Context, receiver *azservicebus.Receiver, messageID string) (*azservicebus.ReceivedMessage, error) {
	seen := make(map[string]struct{})
	for {
		messages, err := receiver.ReceiveMessages(ctx, 1, nil)
		if err != nil {
			return nil, fmt.Errorf("receive DLQ messages: %w", err)
		}
		if len(messages) == 0 {
			return nil, fmt.Errorf("message %s not found in DLQ", messageID)
		}
		msg := messages[0]
		if msg.MessageID == messageID {
			return msg, nil
		}
		if _, ok := seen[msg.MessageID]; ok {
			_ = receiver.AbandonMessage(ctx, msg, nil)
			return nil, fmt.Errorf("message %s not found in DLQ", messageID)
		}
		seen[msg.MessageID] = struct{}{}
		_ = receiver.AbandonMessage(ctx, msg, nil)
	}
}

func isAuthError(err error) bool {
	var sbErr *azservicebus.Error
	if errors.As(err, &sbErr) {
		return sbErr.Code == azservicebus.CodeUnauthorizedAccess
	}
	return false
}

func peekMessages(ctx context.Context, receiver *azservicebus.Receiver, maxCount int, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	var opts *azservicebus.PeekMessagesOptions
	if fromSequenceNumber > 0 {
		opts = &azservicebus.PeekMessagesOptions{
			FromSequenceNumber: &fromSequenceNumber,
		}
	}
	peeked, err := receiver.PeekMessages(ctx, maxCount, opts)
	if err != nil {
		return fmt.Errorf("peek messages: %w", err)
	}

	messages := make([]PeekedMessage, 0, len(peeked))
	for _, msg := range peeked {
		entry := PeekedMessage{
			MessageID: msg.MessageID,
		}
		if msg.SequenceNumber != nil {
			entry.SequenceNumber = *msg.SequenceNumber
		}
		if msg.EnqueuedTime != nil {
			entry.EnqueuedAt = *msg.EnqueuedTime
		}
		entry.FullBody = string(msg.Body)
		entry.BodyPreview = truncateBody(msg.Body, maxBodyPreview)
		messages = append(messages, entry)
	}

	if len(messages) > 0 {
		send(messages)
	}

	return nil
}

func truncateBody(body []byte, max int) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "..."
}

func parseResourceGroup(id *string) string {
	if id == nil {
		return ""
	}
	parts := strings.Split(*id, "/")
	for i, part := range parts {
		if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func endpointToFQDN(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimSuffix(endpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, ":443")
	return endpoint
}
