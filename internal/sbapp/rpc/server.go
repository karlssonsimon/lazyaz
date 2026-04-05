package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/cache"
	sharedrpc "azure-storage/internal/rpc"
	sbcore "azure-storage/internal/sbapp/core"
)

type Server struct {
	service  *servicebus.Service
	rpc      *sharedrpc.Server
	sessions *sharedrpc.Sessions[*sessionState]
	cache    serverCache
}

type sessionState struct {
	mu   sync.Mutex
	core *sbcore.Service
}

type serverCache struct {
	subscriptions cache.Store[azure.Subscription]
	namespaces    cache.Store[servicebus.Namespace]
	entities      cache.Store[servicebus.Entity]
	topicSubs     cache.Store[servicebus.TopicSubscription]
}

func NewServer(socketPath string, svc *servicebus.Service, db *cache.DB) (*Server, error) {
	trimmed := strings.TrimSpace(socketPath)
	if trimmed == "" {
		return nil, fmt.Errorf("socket path is required")
	}
	if svc == nil {
		return nil, fmt.Errorf("service bus service is required")
	}
	server := &Server{service: svc, cache: newServerCache(db)}
	server.sessions = sharedrpc.NewSessions(func() *sessionState {
		session := sbcore.NewSession()
		return &sessionState{core: sbcore.NewService(&session)}
	})
	rpcServer, err := sharedrpc.ListenUnix(trimmed, server.handleRequest)
	if err != nil {
		return nil, err
	}
	server.rpc = rpcServer
	return server, nil
}

func (s *Server) Close() error {
	if s.rpc == nil {
		return nil
	}
	return s.rpc.Close()
}
func (s *Server) Serve() error { return s.rpc.Serve() }

func (s *Server) handleRequest(req sharedrpc.Request) sharedrpc.Response {
	switch req.Method {
	case "session.create":
		id, session, err := s.sessions.Create()
		if err != nil {
			return sharedrpc.Response{ID: req.ID, Error: err.Error()}
		}
		return sharedrpc.EncodeResult(req.ID, SessionCreateResult{Session: id, State: session.core.Snapshot()})
	case "session.close":
		if strings.TrimSpace(req.Session) == "" {
			return sharedrpc.Response{ID: req.ID, Error: "session is required"}
		}
		s.sessions.Delete(req.Session)
		return sharedrpc.EncodeResult(req.ID, map[string]bool{"closed": true})
	case "state.get":
		session, resp := s.requireSession(req)
		if session == nil {
			return resp
		}
		session.mu.Lock()
		defer session.mu.Unlock()
		return sharedrpc.EncodeResult(req.ID, session.core.Snapshot())
	case "action.invoke":
		session, resp := s.requireSession(req)
		if session == nil {
			return resp
		}
		var params sbcore.ActionRequest
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return sharedrpc.Response{ID: req.ID, Error: fmt.Sprintf("decode action params: %v", err)}
			}
		}
		result, err := s.invoke(session, params)
		if err != nil {
			return sharedrpc.Response{ID: req.ID, Error: err.Error()}
		}
		return sharedrpc.EncodeResult(req.ID, result)
	default:
		return sharedrpc.Response{ID: req.ID, Error: fmt.Sprintf("unsupported method %q", req.Method)}
	}
}

func (s *Server) requireSession(req sharedrpc.Request) (*sessionState, sharedrpc.Response) {
	trimmed := strings.TrimSpace(req.Session)
	if trimmed == "" {
		return nil, sharedrpc.Response{ID: req.ID, Error: "session is required"}
	}
	session, ok := s.sessions.Get(trimmed)
	if !ok {
		return nil, sharedrpc.Response{ID: req.ID, Error: "session not found"}
	}
	return session, sharedrpc.Response{}
}

func (s *Server) invoke(session *sessionState, req sbcore.ActionRequest) (ActionInvokeResult, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	result, err := session.core.Dispatch(req)
	if err != nil {
		return ActionInvokeResult{}, err
	}
	if result.Status != "" {
		session.core.Session().SetStatus(result.Status)
	}
	if err := s.executeLoadRequest(session.core.Session(), result.LoadRequest); err != nil {
		return ActionInvokeResult{}, err
	}
	return ActionInvokeResult{Action: result, State: session.core.Snapshot()}, nil
}

func (s *Server) executeLoadRequest(session *sbcore.Session, req sbcore.LoadRequest) error {
	if req.Kind == sbcore.LoadNone {
		return nil
	}
	session.BeginLoading(req.Status)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	switch req.Kind {
	case sbcore.LoadSubscriptions:
		if items, ok := s.cache.subscriptions.Get(""); ok {
			session.ApplySubscriptionsResult(items, false, nil)
		}
		items, err := collectSubscriptions(ctx, s.service)
		if err == nil {
			s.cache.subscriptions.Set("", items)
		}
		session.ApplySubscriptionsResult(items, true, err)
		return nil
	case sbcore.LoadNamespaces:
		if items, ok := s.cache.namespaces.Get(req.SubscriptionID); ok {
			session.ApplyNamespacesResult(req.SubscriptionID, items, false, nil)
		}
		items, err := collectNamespaces(ctx, s.service, req.SubscriptionID)
		if err == nil {
			s.cache.namespaces.Set(req.SubscriptionID, items)
		}
		session.ApplyNamespacesResult(req.SubscriptionID, items, true, err)
		return nil
	case sbcore.LoadEntities:
		key := cache.Key(req.Namespace.SubscriptionID, req.Namespace.Name)
		if items, ok := s.cache.entities.Get(key); ok {
			session.ApplyEntitiesResult(req.Namespace, items, false, nil)
		}
		items, err := collectEntities(ctx, s.service, req.Namespace)
		if err == nil {
			s.cache.entities.Set(key, items)
		}
		session.ApplyEntitiesResult(req.Namespace, items, true, err)
		return nil
	case sbcore.LoadTopicSubscriptions:
		key := cache.Key(req.Namespace.SubscriptionID, req.Namespace.Name, req.TopicName)
		if items, ok := s.cache.topicSubs.Get(key); ok {
			session.ApplyTopicSubscriptionsResult(req.Namespace, req.TopicName, items, false, nil)
		}
		items, err := collectTopicSubscriptions(ctx, s.service, req.Namespace, req.TopicName)
		if err == nil {
			s.cache.topicSubs.Set(key, items)
		}
		session.ApplyTopicSubscriptionsResult(req.Namespace, req.TopicName, items, true, err)
		return nil
	case sbcore.LoadQueueMessages:
		items, err := collectQueueMessages(ctx, s.service, req.Namespace, req.Entity.Name, req.DeadLetter)
		session.ApplyMessagesResult(req.Source, items, err)
		return nil
	case sbcore.LoadSubscriptionMessages:
		items, err := collectSubscriptionMessages(ctx, s.service, req.Namespace, req.Entity.Name, req.TopicSub.Name, req.DeadLetter)
		session.ApplyMessagesResult(req.Source, items, err)
		return nil
	case sbcore.LoadRequeueMessages:
		requeued, dupID, err := requeueMessages(ctx, s.service, req.Namespace, req.Entity, req.TopicSub, req.MessageIDs)
		session.ApplyRequeueResult(requeued, len(req.MessageIDs), dupID, err)
		entities, refreshErr := collectEntities(ctx, s.service, req.Namespace)
		session.ApplyEntitiesRefreshed(entities, refreshErr)
		if repeek := session.RePeekMessagesRequest(); repeek.Kind != sbcore.LoadNone {
			return s.executeLoadRequest(session, repeek)
		}
		return nil
	case sbcore.LoadDeleteDuplicate:
		err := deleteDuplicate(ctx, s.service, req.Namespace, req.Entity, req.TopicSub, req.MessageID)
		session.ApplyDeleteDuplicateResult(req.MessageID, err)
		entities, refreshErr := collectEntities(ctx, s.service, req.Namespace)
		session.ApplyEntitiesRefreshed(entities, refreshErr)
		if repeek := session.RePeekMessagesRequest(); repeek.Kind != sbcore.LoadNone {
			return s.executeLoadRequest(session, repeek)
		}
		return nil
	case sbcore.LoadRefreshEntities:
		entities, err := collectEntities(ctx, s.service, req.Namespace)
		session.ApplyEntitiesRefreshed(entities, err)
		if err != nil {
			return err
		}
		session.Loading = false
		return nil
	default:
		return fmt.Errorf("unsupported service bus load request %v", req.Kind)
	}
}

func newServerCache(db *cache.DB) serverCache {
	if db != nil {
		return serverCache{
			subscriptions: cache.NewStore[azure.Subscription](db, "subscriptions"),
			namespaces:    cache.NewStore[servicebus.Namespace](db, "sb_namespaces"),
			entities:      cache.NewStore[servicebus.Entity](db, "sb_entities"),
			topicSubs:     cache.NewStore[servicebus.TopicSubscription](db, "sb_topic_subs"),
		}
	}
	return serverCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		namespaces:    cache.NewMap[servicebus.Namespace](),
		entities:      cache.NewMap[servicebus.Entity](),
		topicSubs:     cache.NewMap[servicebus.TopicSubscription](),
	}
}

func collectSubscriptions(ctx context.Context, svc *servicebus.Service) ([]azure.Subscription, error) {
	var items []azure.Subscription
	err := svc.ListSubscriptions(ctx, func(batch []azure.Subscription) { items = append(items, batch...) })
	return items, err
}

func collectNamespaces(ctx context.Context, svc *servicebus.Service, subscriptionID string) ([]servicebus.Namespace, error) {
	var items []servicebus.Namespace
	err := svc.ListNamespaces(ctx, subscriptionID, func(batch []servicebus.Namespace) { items = append(items, batch...) })
	return items, err
}

func collectEntities(ctx context.Context, svc *servicebus.Service, ns servicebus.Namespace) ([]servicebus.Entity, error) {
	var items []servicebus.Entity
	err := svc.ListEntities(ctx, ns, func(batch []servicebus.Entity) { items = append(items, batch...) })
	return items, err
}

func collectTopicSubscriptions(ctx context.Context, svc *servicebus.Service, ns servicebus.Namespace, topicName string) ([]servicebus.TopicSubscription, error) {
	var items []servicebus.TopicSubscription
	err := svc.ListTopicSubscriptions(ctx, ns, topicName, func(batch []servicebus.TopicSubscription) { items = append(items, batch...) })
	return items, err
}

func collectQueueMessages(ctx context.Context, svc *servicebus.Service, ns servicebus.Namespace, queueName string, deadLetter bool) ([]servicebus.PeekedMessage, error) {
	var items []servicebus.PeekedMessage
	err := svc.PeekQueueMessages(ctx, ns, queueName, 50, deadLetter, func(batch []servicebus.PeekedMessage) { items = append(items, batch...) })
	return items, err
}

func collectSubscriptionMessages(ctx context.Context, svc *servicebus.Service, ns servicebus.Namespace, topicName, subName string, deadLetter bool) ([]servicebus.PeekedMessage, error) {
	var items []servicebus.PeekedMessage
	err := svc.PeekSubscriptionMessages(ctx, ns, topicName, subName, 50, deadLetter, func(batch []servicebus.PeekedMessage) { items = append(items, batch...) })
	return items, err
}

func requeueMessages(ctx context.Context, svc *servicebus.Service, ns servicebus.Namespace, entity servicebus.Entity, topicSub servicebus.TopicSubscription, messageIDs []string) (int, string, error) {
	if entity.Kind == servicebus.EntityQueue {
		requeued, err := svc.RequeueFromDLQ(ctx, ns, entity.Name, messageIDs)
		var dupErr *servicebus.DuplicateError
		if errors.As(err, &dupErr) {
			return requeued, dupErr.MessageID, nil
		}
		return requeued, "", err
	}
	requeued, err := svc.RequeueFromSubscriptionDLQ(ctx, ns, entity.Name, topicSub.Name, messageIDs)
	var dupErr *servicebus.DuplicateError
	if errors.As(err, &dupErr) {
		return requeued, dupErr.MessageID, nil
	}
	return requeued, "", err
}

func deleteDuplicate(ctx context.Context, svc *servicebus.Service, ns servicebus.Namespace, entity servicebus.Entity, topicSub servicebus.TopicSubscription, messageID string) error {
	if entity.Kind == servicebus.EntityQueue {
		return svc.DeleteFromDLQ(ctx, ns, entity.Name, messageID)
	}
	return svc.DeleteFromSubscriptionDLQ(ctx, ns, entity.Name, topicSub.Name, messageID)
}
