package core

import (
	"fmt"
	"sort"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"
)

type Pane int

const (
	SubscriptionsPane Pane = iota
	NamespacesPane
	EntitiesPane
	DetailPane
)

type DetailView int

const (
	DetailMessages DetailView = iota
	DetailTopicSubscriptions
)

type LoadKind int

const (
	LoadNone LoadKind = iota
	LoadSubscriptions
	LoadNamespaces
	LoadEntities
	LoadTopicSubscriptions
	LoadQueueMessages
	LoadSubscriptionMessages
	LoadRequeueMessages
	LoadDeleteDuplicate
	LoadRefreshEntities
)

type LoadRequest struct {
	Kind           LoadKind
	SubscriptionID string
	Namespace      servicebus.Namespace
	Entity         servicebus.Entity
	TopicSub       servicebus.TopicSubscription
	TopicName      string
	Source         string
	DeadLetter     bool
	MessageIDs     []string
	MessageID      string
	Status         string
}

type Session struct {
	Focus               Pane
	Subscriptions       []azure.Subscription
	Namespaces          []servicebus.Namespace
	Entities            []servicebus.Entity
	TopicSubs           []servicebus.TopicSubscription
	PeekedMessages      []servicebus.PeekedMessage
	HasSubscription     bool
	CurrentSubscription azure.Subscription
	HasNamespace        bool
	CurrentNamespace    servicebus.Namespace
	HasEntity           bool
	CurrentEntity       servicebus.Entity
	DetailMode          DetailView
	ViewingTopicSub     bool
	CurrentTopicSub     servicebus.TopicSubscription
	DeadLetter          bool
	DLQFilter           bool
	ViewingMessage      bool
	SelectedMessage     servicebus.PeekedMessage
	MarkedMessages      map[string]struct{}
	DuplicateMessages   map[string]struct{}
	Loading             bool
	Status              string
	LastErr             string
}

func NewSession() Session {
	return Session{
		Focus:             SubscriptionsPane,
		DetailMode:        DetailMessages,
		MarkedMessages:    make(map[string]struct{}),
		DuplicateMessages: make(map[string]struct{}),
	}
}

func (s *Session) BeginLoading(status string) {
	s.Loading = true
	s.LastErr = ""
	s.Status = status
}

func (s *Session) ClearError() { s.LastErr = "" }

func (s *Session) SetError(status string, err error) {
	s.Loading = false
	s.Status = status
	if err == nil {
		s.LastErr = ""
		return
	}
	s.LastErr = err.Error()
}

func (s *Session) SetStatus(status string) { s.Status = status }

func (s *Session) clearMarkedMessages() {
	for id := range s.MarkedMessages {
		delete(s.MarkedMessages, id)
	}
}

func (s *Session) clearDuplicateMessages() {
	for id := range s.DuplicateMessages {
		delete(s.DuplicateMessages, id)
	}
}

func (s *Session) ClearDetailState() {
	s.TopicSubs = nil
	s.PeekedMessages = nil
	s.ViewingTopicSub = false
	s.CurrentTopicSub = servicebus.TopicSubscription{}
	s.DetailMode = DetailMessages
	s.DeadLetter = false
	s.ViewingMessage = false
	s.SelectedMessage = servicebus.PeekedMessage{}
	s.clearMarkedMessages()
	s.clearDuplicateMessages()
}

func (s *Session) SelectSubscription(sub azure.Subscription) {
	s.CurrentSubscription = sub
	s.HasSubscription = true
	s.HasNamespace = false
	s.HasEntity = false
	s.CurrentNamespace = servicebus.Namespace{}
	s.CurrentEntity = servicebus.Entity{}
	s.Focus = NamespacesPane
	s.Namespaces = nil
	s.Entities = nil
	s.ClearDetailState()
}

func (s *Session) SelectNamespace(ns servicebus.Namespace) {
	s.CurrentNamespace = ns
	s.HasNamespace = true
	s.HasEntity = false
	s.CurrentEntity = servicebus.Entity{}
	s.Focus = EntitiesPane
	s.Entities = nil
	s.ClearDetailState()
}

func (s *Session) SelectEntity(entity servicebus.Entity) {
	s.CurrentEntity = entity
	s.HasEntity = true
	s.Focus = DetailPane
	s.ClearDetailState()
	s.DeadLetter = false
}

func (s *Session) SelectTopicSub(sub servicebus.TopicSubscription) {
	s.CurrentTopicSub = sub
	s.ViewingTopicSub = true
	s.PeekedMessages = nil
	s.ViewingMessage = false
	s.SelectedMessage = servicebus.PeekedMessage{}
	s.clearMarkedMessages()
	s.clearDuplicateMessages()
	s.DeadLetter = false
}

func (s *Session) OpenMessage(msg servicebus.PeekedMessage) string {
	s.SelectedMessage = msg
	s.ViewingMessage = true
	return fmt.Sprintf("Viewing message %s (Esc/h to go back)", ui.EmptyToDash(msg.MessageID))
}

func (s *Session) CloseMessage() string {
	s.ViewingMessage = false
	s.SelectedMessage = servicebus.PeekedMessage{}
	return "Back to messages"
}

func (s *Session) NextFocus() { s.Focus = (s.Focus + 1) % 4 }

func (s *Session) PreviousFocus() {
	s.Focus--
	if s.Focus >= 0 {
		return
	}
	s.Focus = DetailPane
}

func (s *Session) NavigateLeft() string {
	switch s.Focus {
	case DetailPane:
		if s.ViewingTopicSub {
			s.ViewingTopicSub = false
			s.CurrentTopicSub = servicebus.TopicSubscription{}
			s.PeekedMessages = nil
			s.DetailMode = DetailTopicSubscriptions
			return "Back to topic subscriptions"
		}
		s.Focus = EntitiesPane
		return "Focus: entities"
	case EntitiesPane:
		s.Focus = NamespacesPane
		return "Focus: namespaces"
	case NamespacesPane:
		s.Focus = SubscriptionsPane
		return "Focus: subscriptions"
	default:
		return ""
	}
}

func (s *Session) Backspace() string {
	if s.Focus == DetailPane {
		return s.NavigateLeft()
	}
	return ""
}

func (s *Session) RefreshRequest() LoadRequest {
	if !s.HasSubscription {
		return LoadRequest{Kind: LoadSubscriptions, Status: "Refreshing subscriptions..."}
	}
	if s.Focus == SubscriptionsPane {
		return LoadRequest{Kind: LoadSubscriptions, Status: "Refreshing subscriptions..."}
	}
	if !s.HasNamespace || s.Focus == NamespacesPane {
		return LoadRequest{Kind: LoadNamespaces, SubscriptionID: s.CurrentSubscription.ID, Status: fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(s.CurrentSubscription))}
	}
	if s.Focus == EntitiesPane || !s.HasEntity {
		return LoadRequest{Kind: LoadEntities, Namespace: s.CurrentNamespace, Status: fmt.Sprintf("Loading entities in %s", s.CurrentNamespace.Name)}
	}
	return s.refreshDetailRequest()
}

func (s *Session) refreshDetailRequest() LoadRequest {
	if s.CurrentEntity.Kind == servicebus.EntityQueue {
		return LoadRequest{Kind: LoadQueueMessages, Namespace: s.CurrentNamespace, Entity: s.CurrentEntity, DeadLetter: s.DeadLetter, Status: fmt.Sprintf("Peeking messages from queue %s", s.CurrentEntity.Name), Source: s.CurrentEntity.Name}
	}
	if s.ViewingTopicSub {
		return LoadRequest{Kind: LoadSubscriptionMessages, Namespace: s.CurrentNamespace, Entity: s.CurrentEntity, TopicSub: s.CurrentTopicSub, DeadLetter: s.DeadLetter, Status: fmt.Sprintf("Peeking messages from %s/%s", s.CurrentEntity.Name, s.CurrentTopicSub.Name), Source: s.CurrentEntity.Name + "/" + s.CurrentTopicSub.Name}
	}
	return LoadRequest{Kind: LoadTopicSubscriptions, Namespace: s.CurrentNamespace, TopicName: s.CurrentEntity.Name, Status: fmt.Sprintf("Loading subscriptions for topic %s", s.CurrentEntity.Name)}
}

func (s *Session) SelectSubscriptionRequest(sub azure.Subscription) LoadRequest {
	s.SelectSubscription(sub)
	return LoadRequest{Kind: LoadNamespaces, SubscriptionID: sub.ID, Status: fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(sub))}
}

func (s *Session) SelectNamespaceRequest(ns servicebus.Namespace) LoadRequest {
	s.SelectNamespace(ns)
	return LoadRequest{Kind: LoadEntities, Namespace: ns, Status: fmt.Sprintf("Loading entities in %s", ns.Name)}
}

func (s *Session) SelectEntityRequest(entity servicebus.Entity) LoadRequest {
	s.SelectEntity(entity)
	if entity.Kind == servicebus.EntityTopic {
		return LoadRequest{Kind: LoadTopicSubscriptions, Namespace: s.CurrentNamespace, TopicName: entity.Name, Status: fmt.Sprintf("Loading subscriptions for topic %s", entity.Name)}
	}
	return LoadRequest{Kind: LoadQueueMessages, Namespace: s.CurrentNamespace, Entity: entity, DeadLetter: s.DeadLetter, Status: fmt.Sprintf("Peeking messages from queue %s", entity.Name), Source: entity.Name}
}

func (s *Session) SelectTopicSubRequest(sub servicebus.TopicSubscription) LoadRequest {
	s.SelectTopicSub(sub)
	return LoadRequest{Kind: LoadSubscriptionMessages, Namespace: s.CurrentNamespace, Entity: s.CurrentEntity, TopicSub: sub, DeadLetter: s.DeadLetter, Status: fmt.Sprintf("Peeking messages from %s/%s", s.CurrentEntity.Name, sub.Name), Source: s.CurrentEntity.Name + "/" + sub.Name}
}

func (s *Session) RePeekMessagesRequest() LoadRequest {
	dlqLabel := "active"
	if s.DeadLetter {
		dlqLabel = "DLQ"
	}
	if s.CurrentEntity.Kind == servicebus.EntityQueue {
		return LoadRequest{Kind: LoadQueueMessages, Namespace: s.CurrentNamespace, Entity: s.CurrentEntity, DeadLetter: s.DeadLetter, Status: fmt.Sprintf("Peeking %s messages from queue %s", dlqLabel, s.CurrentEntity.Name), Source: s.CurrentEntity.Name}
	}
	if s.ViewingTopicSub {
		return LoadRequest{Kind: LoadSubscriptionMessages, Namespace: s.CurrentNamespace, Entity: s.CurrentEntity, TopicSub: s.CurrentTopicSub, DeadLetter: s.DeadLetter, Status: fmt.Sprintf("Peeking %s messages from %s/%s", dlqLabel, s.CurrentEntity.Name, s.CurrentTopicSub.Name), Source: s.CurrentEntity.Name + "/" + s.CurrentTopicSub.Name}
	}
	return LoadRequest{}
}

func (s *Session) RefreshEntitiesRequest() LoadRequest {
	return LoadRequest{Kind: LoadRefreshEntities, Namespace: s.CurrentNamespace}
}

func (s *Session) ToggleMark(messageID string, duplicate bool) string {
	if duplicate || messageID == "" {
		return ""
	}
	if _, marked := s.MarkedMessages[messageID]; marked {
		delete(s.MarkedMessages, messageID)
		return fmt.Sprintf("Unmarked %s (%d marked)", messageID, len(s.MarkedMessages))
	}
	s.MarkedMessages[messageID] = struct{}{}
	return fmt.Sprintf("Marked %s (%d marked)", messageID, len(s.MarkedMessages))
}

func (s *Session) SetDeadLetterMode(on bool) bool {
	if s.DeadLetter == on {
		return false
	}
	s.DeadLetter = on
	s.clearMarkedMessages()
	s.clearDuplicateMessages()
	return true
}

func (s *Session) ToggleDLQFilter() string {
	s.DLQFilter = !s.DLQFilter
	if s.DLQFilter {
		return "DLQ filter enabled - showing only entities with dead-letter messages"
	}
	return "DLQ filter disabled - showing all entities"
}

func (s Session) CollectRequeueIDs(selectedMessageID string, selectedIsDup bool) []string {
	if len(s.MarkedMessages) > 0 {
		var ids []string
		for _, msg := range s.PeekedMessages {
			if _, ok := s.MarkedMessages[msg.MessageID]; !ok {
				continue
			}
			if _, isDup := s.DuplicateMessages[msg.MessageID]; isDup {
				continue
			}
			ids = append(ids, msg.MessageID)
		}
		return ids
	}
	if selectedMessageID == "" || selectedIsDup {
		return nil
	}
	return []string{selectedMessageID}
}

func (s *Session) RequeueRequest(messageIDs []string) LoadRequest {
	if len(messageIDs) == 0 {
		return LoadRequest{}
	}
	return LoadRequest{Kind: LoadRequeueMessages, Namespace: s.CurrentNamespace, Entity: s.CurrentEntity, TopicSub: s.CurrentTopicSub, MessageIDs: messageIDs, Status: fmt.Sprintf("Requeuing %d message(s)...", len(messageIDs))}
}

func (s *Session) DeleteDuplicateRequest(messageID string) LoadRequest {
	if messageID == "" {
		return LoadRequest{}
	}
	return LoadRequest{Kind: LoadDeleteDuplicate, Namespace: s.CurrentNamespace, Entity: s.CurrentEntity, TopicSub: s.CurrentTopicSub, MessageID: messageID, Status: "Deleting duplicate message..."}
}

func (s Session) AcceptNamespacesResult(subscriptionID string) bool {
	return s.HasSubscription && s.CurrentSubscription.ID == subscriptionID
}

func (s Session) AcceptEntitiesResult(ns servicebus.Namespace) bool {
	return s.HasNamespace && s.CurrentNamespace.Name == ns.Name
}

func (s Session) AcceptTopicSubscriptionsResult(ns servicebus.Namespace, topicName string) bool {
	return s.HasEntity && s.CurrentEntity.Kind == servicebus.EntityTopic && s.CurrentNamespace.Name == ns.Name && s.CurrentEntity.Name == topicName
}

func (s Session) MessagesSourceMatches(source string) bool {
	if s.CurrentEntity.Kind == servicebus.EntityQueue {
		return source == s.CurrentEntity.Name
	}
	if s.ViewingTopicSub {
		return source == s.CurrentEntity.Name+"/"+s.CurrentTopicSub.Name
	}
	return false
}

func (s *Session) ApplySubscriptionsResult(subscriptions []azure.Subscription, done bool, err error) {
	if err != nil {
		s.SetError("Failed to load subscriptions", err)
		return
	}
	s.ClearError()
	s.Subscriptions = subscriptions
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d subscriptions.", len(subscriptions))
	}
}

func (s *Session) ApplyNamespacesResult(subscriptionID string, namespaces []servicebus.Namespace, done bool, err error) bool {
	if !s.AcceptNamespacesResult(subscriptionID) {
		return false
	}
	if err != nil {
		s.SetError(fmt.Sprintf("Failed to load namespaces in %s", ui.SubscriptionDisplayName(s.CurrentSubscription)), err)
		return true
	}
	s.ClearError()
	s.Namespaces = namespaces
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d namespaces from %s.", len(namespaces), ui.SubscriptionDisplayName(s.CurrentSubscription))
	}
	return true
}

func (s *Session) ApplyEntitiesResult(ns servicebus.Namespace, entities []servicebus.Entity, done bool, err error) bool {
	if !s.AcceptEntitiesResult(ns) {
		return false
	}
	if err != nil {
		s.SetError(fmt.Sprintf("Failed to load entities in %s", ns.Name), err)
		return true
	}
	s.ClearError()
	s.Entities = entities
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d entities from %s.", len(entities), ns.Name)
	}
	return true
}

func (s *Session) ApplyTopicSubscriptionsResult(ns servicebus.Namespace, topicName string, subs []servicebus.TopicSubscription, done bool, err error) bool {
	if !s.AcceptTopicSubscriptionsResult(ns, topicName) {
		return false
	}
	if err != nil {
		s.SetError(fmt.Sprintf("Failed to load subscriptions for topic %s", topicName), err)
		return true
	}
	s.ClearError()
	s.TopicSubs = subs
	s.DetailMode = DetailTopicSubscriptions
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d subscriptions for topic %s", len(subs), topicName)
	}
	return true
}

func (s *Session) ApplyMessagesResult(source string, messages []servicebus.PeekedMessage, err error) bool {
	if !s.HasEntity || !s.MessagesSourceMatches(source) {
		return false
	}
	s.Loading = false
	if err != nil {
		s.LastErr = err.Error()
		s.Status = fmt.Sprintf("Failed to peek messages from %s", source)
		return true
	}
	s.ClearError()
	s.PeekedMessages = messages
	s.DetailMode = DetailMessages
	s.ViewingMessage = false
	s.SelectedMessage = servicebus.PeekedMessage{}
	s.Status = fmt.Sprintf("Peeked %d messages from %s", len(messages), source)
	return true
}

func (s *Session) ApplyRequeueResult(requeued, total int, duplicateID string, err error) {
	s.Loading = false
	s.clearMarkedMessages()
	if duplicateID != "" {
		s.DuplicateMessages[duplicateID] = struct{}{}
		s.LastErr = fmt.Sprintf("message %s sent but not removed from DLQ (possible duplicate)", duplicateID)
	} else if err != nil {
		s.LastErr = err.Error()
	} else {
		s.LastErr = ""
	}
	if requeued > 0 {
		s.Status = fmt.Sprintf("%d of %d message(s) requeued", requeued, total)
	} else {
		s.Status = "Failed to requeue messages"
	}
}

func (s *Session) ApplyDeleteDuplicateResult(messageID string, err error) {
	s.Loading = false
	if err != nil {
		s.LastErr = err.Error()
		s.Status = "Failed to delete duplicate message"
		return
	}
	s.ClearError()
	delete(s.DuplicateMessages, messageID)
	s.Status = "Duplicate message deleted"
}

func (s *Session) ApplyEntitiesRefreshed(entities []servicebus.Entity, err error) bool {
	if err != nil {
		return false
	}
	s.Entities = entities
	return true
}

func (s Session) SortedMarkedMessageIDs() []string {
	if len(s.MarkedMessages) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.MarkedMessages))
	for id := range s.MarkedMessages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
