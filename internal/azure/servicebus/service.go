package servicebus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"azure-storage/internal/azure"

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

type PeekedMessage struct {
	MessageID   string
	EnqueuedAt  time.Time
	BodyPreview string
	FullBody    string
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
	s.connStrClients[ns.FQDN] = c
	s.mu.Unlock()

	return c, nil
}

func (s *Service) ListSubscriptions(ctx context.Context) ([]azure.Subscription, error) {
	return azure.ListSubscriptions(ctx, s.cred)
}

func (s *Service) ListNamespaces(ctx context.Context, subscriptionID string) ([]Namespace, error) {
	client, err := armservicebus.NewNamespacesClient(subscriptionID, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create namespaces client: %w", err)
	}

	namespaces := make([]Namespace, 0)
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}

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

			namespaces = append(namespaces, entry)
		}
	}

	sort.Slice(namespaces, func(i, j int) bool {
		return strings.ToLower(namespaces[i].Name) < strings.ToLower(namespaces[j].Name)
	})

	return namespaces, nil
}

func (s *Service) ListEntities(ctx context.Context, ns Namespace) ([]Entity, error) {
	client, err := armservicebus.NewQueuesClient(ns.SubscriptionID, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create queues client: %w", err)
	}

	entities := make([]Entity, 0)

	queuePager := client.NewListByNamespacePager(ns.ResourceGroup, ns.Name, nil)
	for queuePager.More() {
		page, err := queuePager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list queues in %s: %w", ns.Name, err)
		}
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
			entities = append(entities, e)
		}
	}

	topicsClient, err := armservicebus.NewTopicsClient(ns.SubscriptionID, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create topics client: %w", err)
	}

	topicPager := topicsClient.NewListByNamespacePager(ns.ResourceGroup, ns.Name, nil)
	for topicPager.More() {
		page, err := topicPager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list topics in %s: %w", ns.Name, err)
		}
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
			entities = append(entities, e)
		}
	}

	sort.Slice(entities, func(i, j int) bool {
		if entities[i].Kind != entities[j].Kind {
			return entities[i].Kind < entities[j].Kind
		}
		return strings.ToLower(entities[i].Name) < strings.ToLower(entities[j].Name)
	})

	return entities, nil
}

func (s *Service) ListTopicSubscriptions(ctx context.Context, ns Namespace, topicName string) ([]TopicSubscription, error) {
	client, err := armservicebus.NewSubscriptionsClient(ns.SubscriptionID, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create subscriptions client: %w", err)
	}

	subs := make([]TopicSubscription, 0)
	pager := client.NewListByTopicPager(ns.ResourceGroup, ns.Name, topicName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list subscriptions for topic %s: %w", topicName, err)
		}
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
			subs = append(subs, ts)
		}
	}

	sort.Slice(subs, func(i, j int) bool {
		return strings.ToLower(subs[i].Name) < strings.ToLower(subs[j].Name)
	})

	return subs, nil
}

func (s *Service) PeekQueueMessages(ctx context.Context, ns Namespace, queueName string, maxCount int, deadLetter bool) ([]PeekedMessage, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return nil, err
	}

	messages, err := s.peekQueue(ctx, client, queueName, maxCount, deadLetter)
	if err == nil || !isAuthError(err) {
		return messages, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return nil, fmt.Errorf("peek queue %s with AAD failed: %v; connection string fallback failed: %w", queueName, err, fallbackErr)
	}

	messages, err = s.peekQueue(ctx, fallbackClient, queueName, maxCount, deadLetter)
	if err != nil {
		return nil, fmt.Errorf("peek queue %s with connection string fallback: %w", queueName, err)
	}
	return messages, nil
}

func (s *Service) peekQueue(ctx context.Context, client *azservicebus.Client, queueName string, maxCount int, deadLetter bool) ([]PeekedMessage, error) {
	opts := &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
	}
	if deadLetter {
		opts.SubQueue = azservicebus.SubQueueDeadLetter
	}
	receiver, err := client.NewReceiverForQueue(queueName, opts)
	if err != nil {
		return nil, fmt.Errorf("create queue receiver for %s: %w", queueName, err)
	}
	defer receiver.Close(ctx)

	return peekMessages(ctx, receiver, maxCount)
}

func (s *Service) PeekSubscriptionMessages(ctx context.Context, ns Namespace, topicName, subName string, maxCount int, deadLetter bool) ([]PeekedMessage, error) {
	client, err := s.getClient(ns.FQDN)
	if err != nil {
		return nil, err
	}

	messages, err := s.peekSubscription(ctx, client, topicName, subName, maxCount, deadLetter)
	if err == nil || !isAuthError(err) {
		return messages, err
	}

	fallbackClient, fallbackErr := s.getConnStrClient(ctx, ns)
	if fallbackErr != nil {
		return nil, fmt.Errorf("peek subscription %s/%s with AAD failed: %v; connection string fallback failed: %w", topicName, subName, err, fallbackErr)
	}

	messages, err = s.peekSubscription(ctx, fallbackClient, topicName, subName, maxCount, deadLetter)
	if err != nil {
		return nil, fmt.Errorf("peek subscription %s/%s with connection string fallback: %w", topicName, subName, err)
	}
	return messages, nil
}

func (s *Service) peekSubscription(ctx context.Context, client *azservicebus.Client, topicName, subName string, maxCount int, deadLetter bool) ([]PeekedMessage, error) {
	opts := &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
	}
	if deadLetter {
		opts.SubQueue = azservicebus.SubQueueDeadLetter
	}
	receiver, err := client.NewReceiverForSubscription(topicName, subName, opts)
	if err != nil {
		return nil, fmt.Errorf("create subscription receiver for %s/%s: %w", topicName, subName, err)
	}
	defer receiver.Close(ctx)

	return peekMessages(ctx, receiver, maxCount)
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
			if err := sender.SendMessage(ctx, &azservicebus.Message{Body: msg.Body}, nil); err != nil {
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

func peekMessages(ctx context.Context, receiver *azservicebus.Receiver, maxCount int) ([]PeekedMessage, error) {
	peeked, err := receiver.PeekMessages(ctx, maxCount, nil)
	if err != nil {
		return nil, fmt.Errorf("peek messages: %w", err)
	}

	messages := make([]PeekedMessage, 0, len(peeked))
	for _, msg := range peeked {
		entry := PeekedMessage{
			MessageID: msg.MessageID,
		}
		if msg.EnqueuedTime != nil {
			entry.EnqueuedAt = *msg.EnqueuedTime
		}
		entry.FullBody = string(msg.Body)
		entry.BodyPreview = truncateBody(msg.Body, maxBodyPreview)
		messages = append(messages, entry)
	}

	return messages, nil
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
