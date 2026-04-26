package servicebus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/servicebus/armservicebus"
)

const maxBodyPreview = 512

var lockedMessageSessionCounter uint64

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
func NamespaceKey(ns Namespace) string                { return ns.Name }
func EntityKey(e Entity) string                       { return e.Name }
func TopicSubscriptionKey(s TopicSubscription) string { return s.Name }

type PeekedMessage struct {
	MessageID      string
	LockID         string
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
	metricsClient  *azquery.MetricsClient
	generation     uint64
}

func NewService(cred azcore.TokenCredential) *Service {
	return &Service{
		cred:           cred,
		clients:        make(map[string]*azservicebus.Client),
		connStrClients: make(map[string]*azservicebus.Client),
	}
}

func (s *Service) Credential() azcore.TokenCredential {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cred
}

// SetCredential swaps the credential and clears all cached clients so
// they are re-created with the new identity on next use.
func (s *Service) SetCredential(cred azcore.TokenCredential) {
	s.mu.Lock()
	oldClients := s.clients
	oldConnStrClients := s.connStrClients
	s.cred = cred
	s.clients = make(map[string]*azservicebus.Client)
	s.connStrClients = make(map[string]*azservicebus.Client)
	s.metricsClient = nil
	s.generation++
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, c := range oldClients {
		_ = c.Close(ctx)
	}
	for _, c := range oldConnStrClients {
		_ = c.Close(ctx)
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
	cred := s.cred
	generation := s.generation
	s.mu.Unlock()

	if strings.TrimSpace(ns.SubscriptionID) == "" || strings.TrimSpace(ns.ResourceGroup) == "" || strings.TrimSpace(ns.Name) == "" {
		return nil, fmt.Errorf("insufficient namespace metadata for connection string fallback")
	}

	nsClient, err := armservicebus.NewNamespacesClient(ns.SubscriptionID, cred, nil)
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
	if s.generation != generation {
		s.mu.Unlock()
		_ = c.Close(ctx)
		return s.getConnStrClient(ctx, ns)
	}
	// Re-check under lock: another goroutine may have raced us.
	if existing, ok := s.connStrClients[ns.FQDN]; ok {
		s.mu.Unlock()
		_ = c.Close(ctx)
		return existing, nil
	}
	s.connStrClients[ns.FQDN] = c
	s.mu.Unlock()

	return c, nil
}

func (s *Service) ListSubscriptions(ctx context.Context, send func([]azure.Subscription)) error {
	return azure.ListSubscriptions(ctx, s.Credential(), send)
}

func (s *Service) ListNamespaces(ctx context.Context, subscriptionID string, send func([]Namespace)) error {
	client, err := armservicebus.NewNamespacesClient(subscriptionID, s.Credential(), nil)
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
	cred := s.Credential()
	client, err := armservicebus.NewQueuesClient(ns.SubscriptionID, cred, nil)
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

	topicsClient, err := armservicebus.NewTopicsClient(ns.SubscriptionID, cred, nil)
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
	client, err := armservicebus.NewSubscriptionsClient(ns.SubscriptionID, s.Credential(), nil)
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

// withFallback runs fn with an AAD client. If fn returns an auth error,
// it retries with a connection-string client (fetched via the
// control-plane ListKeys API). The label is used to contextualize the
// final error if both paths fail.
func (s *Service) withFallback(ctx context.Context, ns Namespace, label string, fn func(*azservicebus.Client) error) error {
	aad, err := s.getClient(ns.FQDN)
	if err != nil {
		return err
	}
	err = fn(aad)
	if err == nil || !isAuthError(err) {
		return err
	}

	fb, fbErr := s.getConnStrClient(ctx, ns)
	if fbErr != nil {
		return fmt.Errorf("%s with AAD failed: %v; connection string fallback failed: %w", label, err, fbErr)
	}
	if err := fn(fb); err != nil {
		return fmt.Errorf("%s with connection string fallback: %w", label, err)
	}
	return nil
}

func (s *Service) PeekQueueMessages(ctx context.Context, ns Namespace, queueName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	return s.withFallback(ctx, ns, "peek queue "+queueName, func(c *azservicebus.Client) error {
		return s.peekQueue(ctx, c, queueName, maxCount, deadLetter, fromSequenceNumber, send)
	})
}

func (s *Service) peekQueue(ctx context.Context, client *azservicebus.Client, queueName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	receiver, err := newReceiver(client, queueName, "", deadLetter)
	if err != nil {
		return fmt.Errorf("create queue receiver for %s: %w", queueName, err)
	}
	defer receiver.Close(ctx)
	return peekMessages(ctx, receiver, maxCount, fromSequenceNumber, send)
}

func (s *Service) PeekSubscriptionMessages(ctx context.Context, ns Namespace, topicName, subName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	return s.withFallback(ctx, ns, fmt.Sprintf("peek subscription %s/%s", topicName, subName), func(c *azservicebus.Client) error {
		return s.peekSubscription(ctx, c, topicName, subName, maxCount, deadLetter, fromSequenceNumber, send)
	})
}

func (s *Service) peekSubscription(ctx context.Context, client *azservicebus.Client, topicName, subName string, maxCount int, deadLetter bool, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	receiver, err := newReceiver(client, topicName, subName, deadLetter)
	if err != nil {
		return fmt.Errorf("create subscription receiver for %s/%s: %w", topicName, subName, err)
	}
	defer receiver.Close(ctx)
	return peekMessages(ctx, receiver, maxCount, fromSequenceNumber, send)
}

// ReceivedMessages holds the result of a receive-with-lock operation.
// The caller owns the receiver and must Close it when done. While open,
// messages can be completed (removed) or abandoned (lock released).
type LockedMessage struct {
	ID     string
	LockID string
	raw    *azservicebus.ReceivedMessage
}

type ReceivedMessages struct {
	mu       sync.Mutex
	messages []LockedMessage
	receiver *azservicebus.Receiver
}

func lockReceivedMessages(messages []*azservicebus.ReceivedMessage) []LockedMessage {
	session := atomic.AddUint64(&lockedMessageSessionCounter, 1)
	locked := make([]LockedMessage, len(messages))
	for i, msg := range messages {
		locked[i] = LockedMessage{ID: msg.MessageID, LockID: fmt.Sprintf("%d:%d", session, i), raw: msg}
	}
	return locked
}

func (r *ReceivedMessages) CompleteByID(ctx context.Context, id string) error {
	if r == nil {
		return fmt.Errorf("complete locked message %q: no locked messages", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasAvailableMessageLocked(id) {
		return fmt.Errorf("complete locked message %q: message ID not found", id)
	}
	if r.receiver == nil {
		return fmt.Errorf("complete locked message %q: receiver is nil", id)
	}
	return r.completeByIDLocked(ctx, id, func(ctx context.Context, msg *azservicebus.ReceivedMessage) error {
		return r.receiver.CompleteMessage(ctx, msg, nil)
	})
}

func (r *ReceivedMessages) hasAvailableMessageLocked(id string) bool {
	for _, msg := range r.messages {
		if msg.matchesOperationID(id) && msg.raw != nil {
			return true
		}
	}
	return false
}

func (r *ReceivedMessages) completeByID(ctx context.Context, id string, complete func(context.Context, *azservicebus.ReceivedMessage) error) error {
	if r == nil {
		return fmt.Errorf("complete locked message %q: no locked messages", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.completeByIDLocked(ctx, id, complete)
}

func (r *ReceivedMessages) completeByIDLocked(ctx context.Context, id string, complete func(context.Context, *azservicebus.ReceivedMessage) error) error {
	for i := range r.messages {
		msg := &r.messages[i]
		if !msg.matchesOperationID(id) || msg.raw == nil {
			continue
		}
		raw := msg.raw
		if err := complete(ctx, raw); err != nil {
			return err
		}
		msg.raw = nil
		return nil
	}
	return fmt.Errorf("complete locked message %q: message ID not found", id)
}

func (m LockedMessage) matchesOperationID(id string) bool {
	if m.LockID != "" {
		return m.LockID == id
	}
	return m.ID == id
}

func (m LockedMessage) operationID() string {
	if m.LockID != "" {
		return m.LockID
	}
	return m.ID
}

func (m LockedMessage) OperationID() string {
	return m.operationID()
}

func (r *ReceivedMessages) MessagesSnapshot() []LockedMessage {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LockedMessage, len(r.messages))
	copy(out, r.messages)
	return out
}

func (r *ReceivedMessages) RemoveByID(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, msg := range r.messages {
		if !msg.matchesOperationID(id) {
			continue
		}
		copy(r.messages[i:], r.messages[i+1:])
		r.messages[len(r.messages)-1] = LockedMessage{}
		r.messages = r.messages[:len(r.messages)-1]
		return true
	}
	return false
}

func (r *ReceivedMessages) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

// AbandonAll releases locks on all messages.
func (r *ReceivedMessages) AbandonAll(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.abandonAllLocked(ctx)
}

func (r *ReceivedMessages) abandonAllLocked(ctx context.Context) {
	if r.receiver == nil {
		return
	}
	for i := range r.messages {
		msg := &r.messages[i]
		if msg.raw != nil {
			_ = r.receiver.AbandonMessage(ctx, msg.raw, nil)
			msg.raw = nil
		}
	}
}

// Close abandons all messages and closes the receiver.
func (r *ReceivedMessages) Close(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.receiver == nil {
		return
	}
	r.abandonAllLocked(ctx)
	r.receiver.Close(ctx)
	r.receiver = nil
}

func (r *ReceivedMessages) PeekedMessages() []PeekedMessage {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	peeked := make([]PeekedMessage, 0, len(r.messages))
	for _, locked := range r.messages {
		if locked.raw == nil {
			peeked = append(peeked, PeekedMessage{MessageID: locked.ID, LockID: locked.LockID})
			continue
		}
		msg := locked.raw
		entry := PeekedMessage{
			MessageID:   locked.ID,
			LockID:      locked.LockID,
			FullBody:    string(msg.Body),
			BodyPreview: truncateBody(msg.Body, maxBodyPreview),
		}
		if msg.SequenceNumber != nil {
			entry.SequenceNumber = *msg.SequenceNumber
		}
		if msg.EnqueuedTime != nil {
			entry.EnqueuedAt = *msg.EnqueuedTime
		}
		peeked = append(peeked, entry)
	}
	return peeked
}

// newReceiver creates a receiver for a queue (subName == "") or topic
// subscription (subName != ""), optionally targeting the dead-letter
// sub-queue.
func newReceiver(client *azservicebus.Client, entityName, subName string, deadLetter bool) (*azservicebus.Receiver, error) {
	opts := &azservicebus.ReceiverOptions{ReceiveMode: azservicebus.ReceiveModePeekLock}
	if deadLetter {
		opts.SubQueue = azservicebus.SubQueueDeadLetter
	}
	if subName == "" {
		return client.NewReceiverForQueue(entityName, opts)
	}
	return client.NewReceiverForSubscription(entityName, subName, opts)
}

// Receive locks up to maxCount messages from a queue or subscription
// with peek-lock semantics. Pass subName == "" to receive from a queue;
// pass deadLetter == true to target the dead-letter sub-queue. The
// caller owns the returned Receiver and must Close it when done.
func (s *Service) Receive(ctx context.Context, ns Namespace, entityName, subName string, deadLetter bool, maxCount int) (*ReceivedMessages, error) {
	var result *ReceivedMessages
	err := s.withFallback(ctx, ns, receiveLabel(entityName, subName, deadLetter), func(c *azservicebus.Client) error {
		receiver, err := newReceiver(c, entityName, subName, deadLetter)
		if err != nil {
			return fmt.Errorf("create receiver: %w", err)
		}
		messages, err := receiver.ReceiveMessages(ctx, maxCount, nil)
		if err != nil {
			receiver.Close(ctx)
			return fmt.Errorf("receive messages: %w", err)
		}
		result = &ReceivedMessages{messages: lockReceivedMessages(messages), receiver: receiver}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func receiveLabel(entityName, subName string, deadLetter bool) string {
	target := entityName
	if subName != "" {
		target = entityName + "/" + subName
	}
	kind := "receive from"
	if deadLetter {
		kind = "receive from DLQ"
	}
	return fmt.Sprintf("%s %s", kind, target)
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
	return s.withFallback(ctx, ns, "send batch to "+queueOrTopicName, func(c *azservicebus.Client) error {
		return s.sendBatch(ctx, c, queueOrTopicName, messages)
	})
}

func (s *Service) RequeueLockedByID(ctx context.Context, ns Namespace, queueOrTopicName string, locked *ReceivedMessages, ids map[string]struct{}) ([]string, error) {
	if locked == nil {
		return nil, fmt.Errorf("requeue locked messages: no locked messages")
	}
	locked.mu.Lock()
	defer locked.mu.Unlock()

	toRequeue := make([]*azservicebus.ReceivedMessage, 0, len(ids))
	selectedIDs := make([]string, 0, len(ids))
	for _, msg := range locked.messages {
		operationID := msg.operationID()
		if _, ok := ids[operationID]; !ok {
			continue
		}
		if msg.raw == nil {
			return nil, fmt.Errorf("requeue locked message %q: raw message is nil", operationID)
		}
		toRequeue = append(toRequeue, msg.raw)
		selectedIDs = append(selectedIDs, operationID)
	}
	if len(selectedIDs) > 0 && locked.receiver == nil {
		return nil, fmt.Errorf("complete locked message %q: receiver is nil", selectedIDs[0])
	}

	if err := s.SendBatch(ctx, ns, queueOrTopicName, toRequeue); err != nil {
		return nil, err
	}

	completed := make([]string, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		if err := locked.completeByIDLocked(ctx, id, func(ctx context.Context, msg *azservicebus.ReceivedMessage) error {
			return locked.receiver.CompleteMessage(ctx, msg, nil)
		}); err != nil {
			return completed, err
		}
		completed = append(completed, id)
	}
	return completed, nil
}

func (s *Service) sendBatch(ctx context.Context, client *azservicebus.Client, name string, messages []*azservicebus.ReceivedMessage) error {
	sender, err := client.NewSender(name, nil)
	if err != nil {
		return fmt.Errorf("create sender for %s: %w", name, err)
	}
	defer sender.Close(ctx)
	return sendMessagesBatched(ctx, sender, messages)
}

// sendMessagesBatched sends messages through sender using message
// batching. When a batch fills up it's flushed and a new one started.
// A single message that's too large for an empty batch is returned as
// an error rather than silently dropped.
func sendMessagesBatched(ctx context.Context, sender *azservicebus.Sender, messages []*azservicebus.ReceivedMessage) error {
	batch, err := sender.NewMessageBatch(ctx, nil)
	if err != nil {
		return fmt.Errorf("create message batch: %w", err)
	}

	for _, orig := range messages {
		if err := batch.AddMessage(toSendableMessage(orig), nil); err == nil {
			continue
		}
		// Batch full — flush and start a new one, then retry.
		if err := sender.SendMessageBatch(ctx, batch, nil); err != nil {
			return fmt.Errorf("send batch: %w", err)
		}
		batch, err = sender.NewMessageBatch(ctx, nil)
		if err != nil {
			return fmt.Errorf("create message batch: %w", err)
		}
		if err := batch.AddMessage(toSendableMessage(orig), nil); err != nil {
			return fmt.Errorf("message %s too large for batch: %w", orig.MessageID, err)
		}
	}

	if batch.NumMessages() > 0 {
		if err := sender.SendMessageBatch(ctx, batch, nil); err != nil {
			return fmt.Errorf("send batch: %w", err)
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

	// Service Bus receivers can silently stall — returning zero messages while
	// the entity still holds more. Retry a few times with growing wait windows
	// before concluding the source is drained.
	const maxConsecutiveZero = 3
	consecutiveZero := 0
	total := 0
	for {
		waitTime := 15 * time.Second
		if consecutiveZero > 0 {
			waitTime = time.Duration(20*(consecutiveZero+1)) * time.Second
		}
		receiveCtx, receiveCancel := context.WithTimeout(ctx, waitTime)
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
			consecutiveZero++
			if consecutiveZero >= maxConsecutiveZero {
				break
			}
			continue
		}
		consecutiveZero = 0

		if err := sendMessagesBatched(ctx, sender, messages); err != nil {
			return total, err
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
	var total int
	err := s.withFallback(ctx, ns, "resend DLQ", func(c *azservicebus.Client) error {
		n, err := s.resendAllFromDLQ(ctx, c, entityName, subName, targetName, expectedCount)
		total = n
		return err
	})
	return total, err
}

func (s *Service) resendAllFromDLQ(ctx context.Context, client *azservicebus.Client, entityName, subName, targetName string, expectedCount int) (int, error) {
	receiver, err := newReceiver(client, entityName, subName, true)
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
//
// The send side needs its own credential. For same-namespace moves
// (the dominant DLQ→main-queue case) we reuse whichever client the
// receiver successfully authenticated with — the conn-string fallback
// uses RootManageSharedAccessKey, which carries Send rights, so a source
// that authenticated via fallback can also send. AAD-only target clients
// silently fail with "Send claim required" when the caller has data-plane
// Receiver but not Sender role on the target.
//
// Cross-namespace moves still build the target client via AAD only; if
// the target rejects on auth, the operation fails. A second-level
// fallback for cross-NS would need a per-target conn-string lookup —
// added when there's a real use case for it.
func (s *Service) ResendAllFromSource(ctx context.Context, ns Namespace, entityName, subName string, deadLetter bool, targetNS Namespace, targetName string, expectedCount int) (int, error) {
	sameNS := strings.EqualFold(ns.FQDN, targetNS.FQDN)

	var crossNSTarget *azservicebus.Client
	if !sameNS {
		c, err := s.getClient(targetNS.FQDN)
		if err != nil {
			return 0, err
		}
		crossNSTarget = c
	}

	var total int
	err := s.withFallback(ctx, ns, "resend from source", func(c *azservicebus.Client) error {
		receiver, err := newReceiver(c, entityName, subName, deadLetter)
		if err != nil {
			return fmt.Errorf("create receiver: %w", err)
		}
		defer receiver.Close(ctx)

		batchSize := 50
		if expectedCount > 0 && expectedCount < batchSize {
			batchSize = expectedCount
		}
		targetClient := crossNSTarget
		if sameNS {
			targetClient = c
		}
		n, err := s.resendAll(ctx, targetClient, receiver, targetName, batchSize, expectedCount)
		total = n
		return err
	})
	return total, err
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
