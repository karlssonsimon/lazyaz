package core

import (
	"errors"
	"fmt"
	"sort"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
)

type Action string

const (
	ActionRefresh             Action = "sb.refresh"
	ActionFocusNext           Action = "sb.focus.next"
	ActionFocusPrevious       Action = "sb.focus.previous"
	ActionNavigateLeft        Action = "sb.navigate.left"
	ActionBackspace           Action = "sb.navigate.backspace"
	ActionSelectSubscription  Action = "sb.select.subscription"
	ActionSelectNamespace     Action = "sb.select.namespace"
	ActionSelectEntity        Action = "sb.select.entity"
	ActionSelectTopicSub      Action = "sb.select.topic_sub"
	ActionOpenMessage         Action = "sb.open.message"
	ActionCloseMessage        Action = "sb.close.message"
	ActionToggleMark          Action = "sb.toggle.mark"
	ActionShowActiveQueue     Action = "sb.show.active"
	ActionShowDeadLetterQueue Action = "sb.show.dlq"
	ActionToggleDLQFilter     Action = "sb.toggle.dlq_filter"
	ActionRequeue             Action = "sb.requeue"
	ActionDeleteDuplicate     Action = "sb.delete_duplicate"
)

type ActionRequest struct {
	Action       Action
	Subscription azure.Subscription
	Namespace    servicebus.Namespace
	Entity       servicebus.Entity
	TopicSub     servicebus.TopicSubscription
	Message      servicebus.PeekedMessage
	MessageID    string
	Duplicate    bool
	MessageIDs   []string
}

type ActionResult struct {
	LoadRequest LoadRequest
	Status      string
}

type Service struct{ session *Session }

func NewService(session *Session) *Service {
	if session == nil {
		s := NewSession()
		session = &s
	}
	return &Service{session: session}
}

func (s *Service) Session() *Session { return s.session }

func (s *Service) Dispatch(req ActionRequest) (ActionResult, error) {
	session := s.session
	if session == nil {
		return ActionResult{}, fmt.Errorf("service bus service has no session")
	}
	switch req.Action {
	case ActionRefresh:
		return ActionResult{LoadRequest: session.RefreshRequest()}, nil
	case ActionFocusNext:
		session.NextFocus()
		return ActionResult{}, nil
	case ActionFocusPrevious:
		session.PreviousFocus()
		return ActionResult{}, nil
	case ActionNavigateLeft:
		return ActionResult{Status: session.NavigateLeft()}, nil
	case ActionBackspace:
		return ActionResult{Status: session.Backspace()}, nil
	case ActionSelectSubscription:
		return ActionResult{LoadRequest: session.SelectSubscriptionRequest(req.Subscription)}, nil
	case ActionSelectNamespace:
		return ActionResult{LoadRequest: session.SelectNamespaceRequest(req.Namespace)}, nil
	case ActionSelectEntity:
		return ActionResult{LoadRequest: session.SelectEntityRequest(req.Entity)}, nil
	case ActionSelectTopicSub:
		return ActionResult{LoadRequest: session.SelectTopicSubRequest(req.TopicSub)}, nil
	case ActionOpenMessage:
		return ActionResult{Status: session.OpenMessage(req.Message)}, nil
	case ActionCloseMessage:
		return ActionResult{Status: session.CloseMessage()}, nil
	case ActionToggleMark:
		return ActionResult{Status: session.ToggleMark(req.MessageID, req.Duplicate)}, nil
	case ActionShowActiveQueue:
		if session.SetDeadLetterMode(false) {
			return ActionResult{LoadRequest: session.RePeekMessagesRequest()}, nil
		}
		return ActionResult{}, nil
	case ActionShowDeadLetterQueue:
		if session.SetDeadLetterMode(true) {
			return ActionResult{LoadRequest: session.RePeekMessagesRequest()}, nil
		}
		return ActionResult{}, nil
	case ActionToggleDLQFilter:
		return ActionResult{Status: session.ToggleDLQFilter()}, nil
	case ActionRequeue:
		ids := req.MessageIDs
		if len(ids) == 0 {
			ids = session.CollectRequeueIDs(req.MessageID, req.Duplicate)
		}
		return ActionResult{LoadRequest: session.RequeueRequest(ids)}, nil
	case ActionDeleteDuplicate:
		return ActionResult{LoadRequest: session.DeleteDuplicateRequest(req.MessageID)}, nil
	default:
		return ActionResult{}, fmt.Errorf("unsupported service bus action %q", req.Action)
	}
}

type Snapshot struct {
	Focus               string               `json:"focus"`
	DetailMode          string               `json:"detail_mode"`
	HasSubscription     bool                 `json:"has_subscription"`
	CurrentSubscription *SubscriptionState   `json:"current_subscription,omitempty"`
	HasNamespace        bool                 `json:"has_namespace"`
	CurrentNamespace    *NamespaceState      `json:"current_namespace,omitempty"`
	HasEntity           bool                 `json:"has_entity"`
	CurrentEntity       *EntityState         `json:"current_entity,omitempty"`
	ViewingTopicSub     bool                 `json:"viewing_topic_sub"`
	CurrentTopicSub     *TopicSubState       `json:"current_topic_sub,omitempty"`
	DeadLetter          bool                 `json:"dead_letter"`
	DLQFilter           bool                 `json:"dlq_filter"`
	ViewingMessage      bool                 `json:"viewing_message"`
	SelectedMessage     *PeekedMessageState  `json:"selected_message,omitempty"`
	Subscriptions       []SubscriptionState  `json:"subscriptions"`
	Namespaces          []NamespaceState     `json:"namespaces"`
	Entities            []EntityState        `json:"entities"`
	TopicSubs           []TopicSubState      `json:"topic_subs"`
	PeekedMessages      []PeekedMessageState `json:"peeked_messages"`
	MarkedMessageIDs    []string             `json:"marked_message_ids"`
	DuplicateMessageIDs []string             `json:"duplicate_message_ids"`
	Loading             bool                 `json:"loading"`
	Status              string               `json:"status"`
	LastErr             string               `json:"last_err"`
}

type SubscriptionState struct{ ID, Name, State string }
type NamespaceState struct{ Name, SubscriptionID, ResourceGroup, FQDN string }
type EntityState struct {
	Name            string
	Kind            servicebus.EntityKind
	ActiveMsgCount  int64
	DeadLetterCount int64
}
type TopicSubState struct{ Name string }
type PeekedMessageState struct {
	MessageID string
	FullBody  string
}

func (s *Service) Snapshot() Snapshot {
	session := s.session
	snapshot := Snapshot{Focus: paneName(session.Focus), DetailMode: detailModeName(session.DetailMode), HasSubscription: session.HasSubscription, HasNamespace: session.HasNamespace, HasEntity: session.HasEntity, ViewingTopicSub: session.ViewingTopicSub, DeadLetter: session.DeadLetter, DLQFilter: session.DLQFilter, ViewingMessage: session.ViewingMessage, MarkedMessageIDs: session.SortedMarkedMessageIDs(), DuplicateMessageIDs: sortedDuplicateIDs(session.DuplicateMessages), Loading: session.Loading, Status: session.Status, LastErr: session.LastErr}
	if session.HasSubscription {
		snapshot.CurrentSubscription = &SubscriptionState{ID: session.CurrentSubscription.ID, Name: session.CurrentSubscription.Name, State: session.CurrentSubscription.State}
	}
	if session.HasNamespace {
		snapshot.CurrentNamespace = &NamespaceState{Name: session.CurrentNamespace.Name, SubscriptionID: session.CurrentNamespace.SubscriptionID, ResourceGroup: session.CurrentNamespace.ResourceGroup, FQDN: session.CurrentNamespace.FQDN}
	}
	if session.HasEntity {
		snapshot.CurrentEntity = &EntityState{Name: session.CurrentEntity.Name, Kind: session.CurrentEntity.Kind, ActiveMsgCount: session.CurrentEntity.ActiveMsgCount, DeadLetterCount: session.CurrentEntity.DeadLetterCount}
	}
	if session.ViewingTopicSub {
		snapshot.CurrentTopicSub = &TopicSubState{Name: session.CurrentTopicSub.Name}
	}
	if session.ViewingMessage {
		snapshot.SelectedMessage = &PeekedMessageState{MessageID: session.SelectedMessage.MessageID, FullBody: session.SelectedMessage.FullBody}
	}
	for _, sub := range session.Subscriptions {
		snapshot.Subscriptions = append(snapshot.Subscriptions, SubscriptionState{ID: sub.ID, Name: sub.Name, State: sub.State})
	}
	for _, ns := range session.Namespaces {
		snapshot.Namespaces = append(snapshot.Namespaces, NamespaceState{Name: ns.Name, SubscriptionID: ns.SubscriptionID, ResourceGroup: ns.ResourceGroup, FQDN: ns.FQDN})
	}
	for _, entity := range session.Entities {
		snapshot.Entities = append(snapshot.Entities, EntityState{Name: entity.Name, Kind: entity.Kind, ActiveMsgCount: entity.ActiveMsgCount, DeadLetterCount: entity.DeadLetterCount})
	}
	for _, sub := range session.TopicSubs {
		snapshot.TopicSubs = append(snapshot.TopicSubs, TopicSubState{Name: sub.Name})
	}
	for _, msg := range session.PeekedMessages {
		snapshot.PeekedMessages = append(snapshot.PeekedMessages, PeekedMessageState{MessageID: msg.MessageID, FullBody: msg.FullBody})
	}
	return snapshot
}

func paneName(pane Pane) string {
	switch pane {
	case SubscriptionsPane:
		return "subscriptions"
	case NamespacesPane:
		return "namespaces"
	case EntitiesPane:
		return "entities"
	case DetailPane:
		return "detail"
	default:
		return "subscriptions"
	}
}

func PaneFromName(name string) Pane {
	switch name {
	case "subscriptions":
		return SubscriptionsPane
	case "entities":
		return EntitiesPane
	case "detail":
		return DetailPane
	default:
		return SubscriptionsPane
	}
}

func detailModeName(mode DetailView) string {
	if mode == DetailTopicSubscriptions {
		return "topic_subscriptions"
	}
	return "messages"
}

func DetailModeFromName(name string) DetailView {
	if name == "topic_subscriptions" {
		return DetailTopicSubscriptions
	}
	return DetailMessages
}

func sortedDuplicateIDs(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func ErrorFromResponse(message string) error {
	if message == "" {
		return nil
	}
	return errors.New(message)
}
