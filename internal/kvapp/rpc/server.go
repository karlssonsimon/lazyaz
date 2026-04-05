package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/cache"
	kvcore "azure-storage/internal/kvapp/core"
	sharedrpc "azure-storage/internal/rpc"
)

type Server struct {
	service  *keyvault.Service
	rpc      *sharedrpc.Server
	sessions *sharedrpc.Sessions[*sessionState]
	cache    serverCache
}

type sessionState struct {
	mu   sync.Mutex
	core *kvcore.Service
}

type serverCache struct {
	subscriptions cache.Store[azure.Subscription]
	vaults        cache.Store[keyvault.Vault]
	secrets       cache.Store[keyvault.Secret]
	versions      cache.Store[keyvault.SecretVersion]
}

func NewServer(socketPath string, svc *keyvault.Service, db *cache.DB) (*Server, error) {
	trimmed := strings.TrimSpace(socketPath)
	if trimmed == "" {
		return nil, fmt.Errorf("socket path is required")
	}
	if svc == nil {
		return nil, fmt.Errorf("key vault service is required")
	}
	server := &Server{service: svc, cache: newServerCache(db)}
	server.sessions = sharedrpc.NewSessions(func() *sessionState {
		session := kvcore.NewSession()
		return &sessionState{core: kvcore.NewService(&session)}
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
		var params kvcore.ActionRequest
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

func (s *Server) invoke(session *sessionState, req kvcore.ActionRequest) (ActionInvokeResult, error) {
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

func (s *Server) executeLoadRequest(session *kvcore.Session, req kvcore.LoadRequest) error {
	if req.Kind == kvcore.LoadNone {
		return nil
	}
	session.BeginLoading(req.Status)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	switch req.Kind {
	case kvcore.LoadSubscriptions:
		if items, ok := s.cache.subscriptions.Get(""); ok {
			session.ApplySubscriptionsResult(items, false, nil)
		}
		items, err := collectSubscriptions(ctx, s.service)
		if err == nil {
			s.cache.subscriptions.Set("", items)
		}
		session.ApplySubscriptionsResult(items, true, err)
		return nil
	case kvcore.LoadVaults:
		if items, ok := s.cache.vaults.Get(req.SubscriptionID); ok {
			session.ApplyVaultsResult(req.SubscriptionID, items, false, nil)
		}
		items, err := collectVaults(ctx, s.service, req.SubscriptionID)
		if err == nil {
			s.cache.vaults.Set(req.SubscriptionID, items)
		}
		session.ApplyVaultsResult(req.SubscriptionID, items, true, err)
		return nil
	case kvcore.LoadSecrets:
		key := cache.Key(req.Vault.SubscriptionID, req.Vault.Name)
		if items, ok := s.cache.secrets.Get(key); ok {
			session.ApplySecretsResult(req.Vault, items, false, nil)
		}
		items, err := collectSecrets(ctx, s.service, req.Vault)
		if err == nil {
			s.cache.secrets.Set(key, items)
		}
		session.ApplySecretsResult(req.Vault, items, true, err)
		return nil
	case kvcore.LoadVersions:
		key := cache.Key(req.Vault.SubscriptionID, req.Vault.Name, req.SecretName)
		if items, ok := s.cache.versions.Get(key); ok {
			session.ApplyVersionsResult(req.Vault, req.SecretName, items, false, nil)
		}
		items, err := collectVersions(ctx, s.service, req.Vault, req.SecretName)
		if err == nil {
			s.cache.versions.Set(key, items)
		}
		session.ApplyVersionsResult(req.Vault, req.SecretName, items, true, err)
		return nil
	case kvcore.LoadPreviewSecret:
		value, err := s.service.GetSecretValue(ctx, req.Vault, req.SecretName, req.Version)
		session.ApplyPreviewResult(req.SecretName, req.Version, value, err)
		return nil
	default:
		return fmt.Errorf("unsupported key vault load request %v", req.Kind)
	}
}

func newServerCache(db *cache.DB) serverCache {
	if db != nil {
		return serverCache{
			subscriptions: cache.NewStore[azure.Subscription](db, "subscriptions"),
			vaults:        cache.NewStore[keyvault.Vault](db, "kv_vaults"),
			secrets:       cache.NewStore[keyvault.Secret](db, "kv_secrets"),
			versions:      cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions"),
		}
	}
	return serverCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		vaults:        cache.NewMap[keyvault.Vault](),
		secrets:       cache.NewMap[keyvault.Secret](),
		versions:      cache.NewMap[keyvault.SecretVersion](),
	}
}

func collectSubscriptions(ctx context.Context, svc *keyvault.Service) ([]azure.Subscription, error) {
	var items []azure.Subscription
	err := svc.ListSubscriptions(ctx, func(batch []azure.Subscription) { items = append(items, batch...) })
	return items, err
}

func collectVaults(ctx context.Context, svc *keyvault.Service, subscriptionID string) ([]keyvault.Vault, error) {
	var items []keyvault.Vault
	err := svc.ListVaults(ctx, subscriptionID, func(batch []keyvault.Vault) { items = append(items, batch...) })
	return items, err
}

func collectSecrets(ctx context.Context, svc *keyvault.Service, vault keyvault.Vault) ([]keyvault.Secret, error) {
	var items []keyvault.Secret
	err := svc.ListSecrets(ctx, vault, func(batch []keyvault.Secret) { items = append(items, batch...) })
	return items, err
}

func collectVersions(ctx context.Context, svc *keyvault.Service, vault keyvault.Vault, secretName string) ([]keyvault.SecretVersion, error) {
	var items []keyvault.SecretVersion
	err := svc.ListSecretVersions(ctx, vault, secretName, func(batch []keyvault.SecretVersion) { items = append(items, batch...) })
	return items, err
}
