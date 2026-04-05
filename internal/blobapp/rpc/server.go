package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	blobcore "azure-storage/internal/blobapp/core"
	"azure-storage/internal/cache"
	sharedrpc "azure-storage/internal/rpc"
	"azure-storage/internal/ui"
)

type Server struct {
	service  *blob.Service
	rpc      *sharedrpc.Server
	sessions *sharedrpc.Sessions[*sessionState]
	cache    serverCache
}

type sessionState struct {
	mu   sync.Mutex
	core *blobcore.Service
}

type serverCache struct {
	subscriptions cache.Store[azure.Subscription]
	accounts      cache.Store[blob.Account]
	containers    cache.Store[blob.ContainerInfo]
	blobs         cache.Store[blob.BlobEntry]
}

func NewServer(socketPath string, svc *blob.Service, db *cache.DB) (*Server, error) {
	trimmed := strings.TrimSpace(socketPath)
	if trimmed == "" {
		return nil, fmt.Errorf("socket path is required")
	}
	if svc == nil {
		return nil, fmt.Errorf("blob service is required")
	}
	server := &Server{service: svc, cache: newServerCache(db)}
	server.sessions = sharedrpc.NewSessions(func() *sessionState {
		session := blobcore.NewSession()
		return &sessionState{core: blobcore.NewService(&session)}
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
		return sharedrpc.Response{ID: req.ID, OK: true, Result: SessionCreateResult{Session: id, State: session.core.Snapshot()}}
	case "session.close":
		if strings.TrimSpace(req.Session) == "" {
			return sharedrpc.Response{ID: req.ID, Error: "session is required"}
		}
		s.sessions.Delete(req.Session)
		return sharedrpc.Response{ID: req.ID, OK: true, Result: map[string]bool{"closed": true}}
	case "state.get":
		session, resp := s.requireSession(req)
		if session == nil {
			return resp
		}
		session.mu.Lock()
		defer session.mu.Unlock()
		return sharedrpc.Response{ID: req.ID, OK: true, Result: session.core.Snapshot()}
	case "action.invoke":
		session, resp := s.requireSession(req)
		if session == nil {
			return resp
		}
		var params blobcore.ActionRequest
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return sharedrpc.Response{ID: req.ID, Error: fmt.Sprintf("decode action params: %v", err)}
			}
		}
		result, err := s.invoke(session, params)
		if err != nil {
			return sharedrpc.Response{ID: req.ID, Error: err.Error()}
		}
		return sharedrpc.Response{ID: req.ID, OK: true, Result: result}
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

func (s *Server) invoke(session *sessionState, req blobcore.ActionRequest) (ActionInvokeResult, error) {
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

func (s *Server) executeLoadRequest(session *blobcore.Session, req blobcore.LoadRequest) error {
	if req.Kind == blobcore.LoadNone {
		return nil
	}
	session.BeginLoading(req.Status)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch req.Kind {
	case blobcore.LoadSubscriptions:
		items, ok := s.cache.subscriptions.Get("")
		if ok {
			session.ApplySubscriptionsResult(items, true, nil)
			if !req.Force {
				return nil
			}
		}
		items, err := collectSubscriptions(ctx, s.service)
		if err == nil {
			s.cache.subscriptions.Set("", items)
		}
		session.ApplySubscriptionsResult(items, true, err)
		return nil
	case blobcore.LoadAccounts:
		if items, ok := s.cache.accounts.Get(req.SubscriptionID); ok {
			session.ApplyAccountsResult(req.SubscriptionID, items, true, nil)
			if !req.Force {
				return nil
			}
		}
		items, err := collectAccounts(ctx, s.service, req.SubscriptionID)
		if err == nil {
			s.cache.accounts.Set(req.SubscriptionID, items)
		}
		session.ApplyAccountsResult(req.SubscriptionID, items, true, err)
		return nil
	case blobcore.LoadContainers:
		key := cache.Key(req.Account.SubscriptionID, req.Account.Name)
		if items, ok := s.cache.containers.Get(key); ok {
			session.ApplyContainersResult(req.Account, items, true, nil)
			if !req.Force {
				return nil
			}
		}
		items, err := collectContainers(ctx, s.service, req.Account)
		if err == nil {
			s.cache.containers.Set(key, items)
		}
		session.ApplyContainersResult(req.Account, items, true, err)
		return nil
	case blobcore.LoadHierarchyBlobs:
		key := blobsCacheKey(req.Account.SubscriptionID, req.Account.Name, req.ContainerName, req.Prefix, false)
		if items, ok := s.cache.blobs.Get(key); ok {
			session.ApplyBlobsResult(req.Account, req.ContainerName, req.Prefix, false, "", items, true, nil, req.Limit)
			if !req.Force {
				return nil
			}
		}
		items, err := collectHierarchyBlobs(ctx, s.service, req.Account, req.ContainerName, req.Prefix, req.Limit)
		if err == nil {
			s.cache.blobs.Set(key, items)
		}
		session.ApplyBlobsResult(req.Account, req.ContainerName, req.Prefix, false, "", items, true, err, req.Limit)
		return nil
	case blobcore.LoadAllBlobs:
		key := blobsCacheKey(req.Account.SubscriptionID, req.Account.Name, req.ContainerName, req.Prefix, true)
		if items, ok := s.cache.blobs.Get(key); ok {
			session.ApplyBlobsResult(req.Account, req.ContainerName, req.Prefix, true, "", items, true, nil, req.Limit)
			if !req.Force {
				return nil
			}
		}
		items, err := collectAllBlobs(ctx, s.service, req.Account, req.ContainerName)
		if err == nil {
			s.cache.blobs.Set(key, items)
		}
		session.ApplyBlobsResult(req.Account, req.ContainerName, req.Prefix, true, "", items, true, err, req.Limit)
		return nil
	case blobcore.LoadSearchBlobs:
		items, err := collectSearchBlobs(ctx, s.service, req.Account, req.ContainerName, req.Prefix, req.Query, req.Limit)
		session.ApplyBlobsResult(req.Account, req.ContainerName, req.Prefix, false, req.Query, items, true, err, req.Limit)
		return nil
	case blobcore.LoadDownload:
		results, err := s.service.DownloadBlobs(ctx, req.Account, req.ContainerName, req.BlobNames, req.DestinationRoot)
		var downloaded, failed int
		var failures []string
		if err == nil {
			for _, result := range results {
				if result.Err != nil {
					failed++
					if len(failures) < 3 {
						failures = append(failures, fmt.Sprintf("%s: %v", result.BlobName, result.Err))
					}
					continue
				}
				downloaded++
			}
			if failed > 3 {
				failures = append(failures, fmt.Sprintf("... and %d more", failed-3))
			}
		}
		session.ApplyDownloadResult(req.DestinationRoot, len(req.BlobNames), downloaded, failed, failures, err)
		return nil
	case blobcore.LoadPreviewWindow:
		return s.loadPreviewWindow(ctx, session, req)
	default:
		return fmt.Errorf("unsupported blob load request %v", req.Kind)
	}
}

func (s *Server) loadPreviewWindow(ctx context.Context, session *blobcore.Session, req blobcore.LoadRequest) error {
	size := req.KnownSize
	contentType := req.KnownContentType
	if size <= 0 || strings.TrimSpace(contentType) == "" {
		props, err := s.service.GetBlobProperties(ctx, req.Account, req.ContainerName, req.BlobName)
		if err != nil {
			session.SetError("Failed to load preview for "+req.BlobName, err)
			return nil
		}
		size = props.Size
		if strings.TrimSpace(contentType) == "" {
			contentType = props.ContentType
		}
	}
	windowStart, windowCount := computePreviewWindow(size, req.Cursor, req.VisibleLines)
	data, err := s.service.ReadBlobRange(ctx, req.Account, req.ContainerName, req.BlobName, windowStart, windowCount)
	binary := ui.IsProbablyBinary(data)
	session.ApplyPreviewResult(req.Account, req.ContainerName, req.BlobName, session.Preview.RequestID, size, contentType, req.Cursor, windowStart, data, binary)
	if err != nil {
		session.SetError("Failed to load preview for "+req.BlobName, err)
	}
	return nil
}

func newServerCache(db *cache.DB) serverCache {
	if db != nil {
		return serverCache{
			subscriptions: cache.NewStore[azure.Subscription](db, "subscriptions"),
			accounts:      cache.NewStore[blob.Account](db, "blob_accounts"),
			containers:    cache.NewStore[blob.ContainerInfo](db, "blob_containers"),
			blobs:         cache.NewStore[blob.BlobEntry](db, "blobs"),
		}
	}
	return serverCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		accounts:      cache.NewMap[blob.Account](),
		containers:    cache.NewMap[blob.ContainerInfo](),
		blobs:         cache.NewMap[blob.BlobEntry](),
	}
}

func blobsCacheKey(subscriptionID, accountName, container, prefix string, loadAll bool) string {
	allStr := "0"
	if loadAll {
		allStr = "1"
	}
	return cache.Key(subscriptionID, accountName, container, prefix, allStr)
}

func collectSubscriptions(ctx context.Context, svc *blob.Service) ([]azure.Subscription, error) {
	var items []azure.Subscription
	err := svc.ListSubscriptions(ctx, func(batch []azure.Subscription) { items = append(items, batch...) })
	return items, err
}

func collectAccounts(ctx context.Context, svc *blob.Service, subscriptionID string) ([]blob.Account, error) {
	var items []blob.Account
	err := svc.DiscoverAccountsForSubscription(ctx, subscriptionID, func(batch []blob.Account) { items = append(items, batch...) })
	return items, err
}

func collectContainers(ctx context.Context, svc *blob.Service, account blob.Account) ([]blob.ContainerInfo, error) {
	var items []blob.ContainerInfo
	err := svc.ListContainers(ctx, account, func(batch []blob.ContainerInfo) { items = append(items, batch...) })
	return items, err
}

func collectHierarchyBlobs(ctx context.Context, svc *blob.Service, account blob.Account, containerName, prefix string, limit int) ([]blob.BlobEntry, error) {
	var items []blob.BlobEntry
	err := svc.ListBlobsLimited(ctx, account, containerName, prefix, limit, func(batch []blob.BlobEntry) { items = append(items, batch...) })
	return items, err
}

func collectAllBlobs(ctx context.Context, svc *blob.Service, account blob.Account, containerName string) ([]blob.BlobEntry, error) {
	var items []blob.BlobEntry
	err := svc.ListAllBlobs(ctx, account, containerName, func(batch []blob.BlobEntry) { items = append(items, batch...) })
	return items, err
}

func collectSearchBlobs(ctx context.Context, svc *blob.Service, account blob.Account, containerName, currentPrefix, query string, limit int) ([]blob.BlobEntry, error) {
	var items []blob.BlobEntry
	effectivePrefix := blobcore.BlobSearchPrefix(currentPrefix, query)
	err := svc.SearchBlobsByPrefix(ctx, account, containerName, effectivePrefix, limit, func(batch []blob.BlobEntry) { items = append(items, batch...) })
	return items, err
}

const (
	previewApproxLineBytes = 96
	previewMinWindowBytes  = int64(64 * 1024)
	previewMaxWindowBytes  = int64(2 * 1024 * 1024)
)

func computePreviewWindow(totalSize, cursor int64, visibleLines int) (int64, int64) {
	if totalSize <= 0 {
		return 0, 0
	}
	visibleBytes := int64(max(1, visibleLines) * previewApproxLineBytes)
	bufferBytes := visibleBytes * 10
	windowSize := visibleBytes + 2*bufferBytes
	if windowSize < previewMinWindowBytes {
		windowSize = previewMinWindowBytes
	}
	if windowSize > previewMaxWindowBytes {
		windowSize = previewMaxWindowBytes
	}
	if windowSize > totalSize {
		windowSize = totalSize
	}
	anchored := clampInt64(cursor, 0, maxInt64(0, totalSize-1))
	start := anchored - bufferBytes
	if start < 0 {
		start = 0
	}
	if start+windowSize > totalSize {
		start = maxInt64(0, totalSize-windowSize)
	}
	return start, windowSize
}

func clampInt64(v, minVal, maxVal int64) int64 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
