package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

const hierarchyDelimiter = "/"

type Account struct {
	Name           string
	SubscriptionID string
	ResourceGroup  string
	BlobEndpoint   string
}

type ContainerInfo struct {
	Name         string
	LastModified time.Time
}

type BlobEntry struct {
	Name          string
	IsPrefix      bool
	Size          int64
	ContentType   string
	LastModified  time.Time
	AccessTier    string
	MetadataCount int
}

type BlobDownloadResult struct {
	BlobName    string
	Destination string
	Err         error
}

type Service struct {
	cred             azcore.TokenCredential
	mu               sync.Mutex
	aadClients       map[string]*service.Client
	sharedKeyClients map[string]*service.Client
}

func NewService(cred azcore.TokenCredential) *Service {
	return &Service{
		cred:             cred,
		aadClients:       make(map[string]*service.Client),
		sharedKeyClients: make(map[string]*service.Client),
	}
}

func (s *Service) ListSubscriptions(ctx context.Context, send func([]azure.Subscription)) error {
	return azure.ListSubscriptions(ctx, s.cred, send)
}

func (s *Service) DiscoverAccounts(ctx context.Context, send func([]Account)) error {
	var subscriptions []azure.Subscription
	err := s.ListSubscriptions(ctx, func(batch []azure.Subscription) {
		subscriptions = append(subscriptions, batch...)
	})
	if err != nil {
		return err
	}

	for _, subscription := range subscriptions {
		err := s.DiscoverAccountsForSubscription(ctx, subscription.ID, send)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) DiscoverAccountsForSubscription(ctx context.Context, subscriptionID string, send func([]Account)) error {
	id := strings.TrimSpace(subscriptionID)
	if id == "" {
		return fmt.Errorf("subscription ID is required")
	}

	storageAccountsClient, err := armstorage.NewAccountsClient(id, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create accounts client for subscription %s: %w", id, err)
	}

	pager := storageAccountsClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list storage accounts for subscription %s: %w", id, err)
		}

		var batch []Account
		for _, account := range page.Value {
			if account == nil || account.Name == nil || account.Properties == nil || account.Properties.PrimaryEndpoints == nil || account.Properties.PrimaryEndpoints.Blob == nil {
				continue
			}

			batch = append(batch, Account{
				Name:           *account.Name,
				SubscriptionID: id,
				ResourceGroup:  parseResourceGroup(account.ID),
				BlobEndpoint:   strings.TrimRight(*account.Properties.PrimaryEndpoints.Blob, "/"),
			})
		}
		if len(batch) > 0 {
			send(batch)
		}
	}

	return nil
}

func (s *Service) ListContainers(ctx context.Context, account Account, send func([]ContainerInfo)) error {
	serviceClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return err
	}

	err = s.listContainersWithClient(ctx, serviceClient, account, send)
	if err == nil {
		return nil
	}
	if !isDataPlaneAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getSharedKeyServiceClient(ctx, account)
	if fallbackErr != nil {
		return fmt.Errorf("list containers for %s with AAD failed: %v; shared key fallback failed: %w", account.Name, err, fallbackErr)
	}

	err = s.listContainersWithClient(ctx, fallbackClient, account, send)
	if err != nil {
		return fmt.Errorf("list containers for %s with shared key fallback: %w", account.Name, err)
	}

	return nil
}

func (s *Service) listContainersWithClient(ctx context.Context, serviceClient *service.Client, account Account, send func([]ContainerInfo)) error {
	pager := serviceClient.NewListContainersPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list containers for %s: %w", account.Name, err)
		}

		var batch []ContainerInfo
		for _, containerItem := range page.ContainerItems {
			if containerItem.Name == nil {
				continue
			}

			containerInfo := ContainerInfo{Name: *containerItem.Name}
			if containerItem.Properties != nil && containerItem.Properties.LastModified != nil {
				containerInfo.LastModified = *containerItem.Properties.LastModified
			}

			batch = append(batch, containerInfo)
		}
		if len(batch) > 0 {
			send(batch)
		}
	}

	return nil
}

func (s *Service) ListBlobs(ctx context.Context, account Account, containerName, prefix string, send func([]BlobEntry)) error {
	return s.ListBlobsLimited(ctx, account, containerName, prefix, 0, send)
}

func (s *Service) ListBlobsLimited(ctx context.Context, account Account, containerName, prefix string, limit int, send func([]BlobEntry)) error {
	serviceClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return err
	}

	err = s.listBlobsWithClient(ctx, serviceClient, account, containerName, prefix, limit, send)
	if err == nil {
		return nil
	}
	if !isDataPlaneAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getSharedKeyServiceClient(ctx, account)
	if fallbackErr != nil {
		return fmt.Errorf("list blobs for %s/%s with AAD failed: %v; shared key fallback failed: %w", account.Name, containerName, err, fallbackErr)
	}

	err = s.listBlobsWithClient(ctx, fallbackClient, account, containerName, prefix, limit, send)
	if err != nil {
		return fmt.Errorf("list blobs for %s/%s with shared key fallback: %w", account.Name, containerName, err)
	}

	return nil
}

func (s *Service) listBlobsWithClient(ctx context.Context, serviceClient *service.Client, account Account, containerName, prefix string, limit int, send func([]BlobEntry)) error {
	containerClient := serviceClient.NewContainerClient(containerName)

	options := &container.ListBlobsHierarchyOptions{}
	if prefix != "" {
		options.Prefix = &prefix
	}

	total := 0
	pager := containerClient.NewListBlobsHierarchyPager(hierarchyDelimiter, options)
	for pager.More() {
		if limit > 0 && total >= limit {
			break
		}

		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list blobs for %s/%s with prefix %q: %w", account.Name, containerName, prefix, err)
		}

		var batch []BlobEntry
		for _, blobPrefix := range page.Segment.BlobPrefixes {
			if limit > 0 && total+len(batch) >= limit {
				break
			}
			if blobPrefix.Name == nil {
				continue
			}

			batch = append(batch, BlobEntry{
				Name:     *blobPrefix.Name,
				IsPrefix: true,
			})
		}

		for _, blobItem := range page.Segment.BlobItems {
			if limit > 0 && total+len(batch) >= limit {
				break
			}
			if blobItem.Name == nil {
				continue
			}

			entry := BlobEntry{
				Name: *blobItem.Name,
			}
			if blobItem.Properties != nil {
				if blobItem.Properties.ContentLength != nil {
					entry.Size = *blobItem.Properties.ContentLength
				}
				if blobItem.Properties.ContentType != nil {
					entry.ContentType = *blobItem.Properties.ContentType
				}
				if blobItem.Properties.LastModified != nil {
					entry.LastModified = *blobItem.Properties.LastModified
				}
				if blobItem.Properties.AccessTier != nil {
					entry.AccessTier = string(*blobItem.Properties.AccessTier)
				}
			}
			entry.MetadataCount = len(blobItem.Metadata)

			batch = append(batch, entry)
		}

		if len(batch) > 0 {
			send(batch)
			total += len(batch)
		}
	}

	return nil
}

func (s *Service) ListAllBlobs(ctx context.Context, account Account, containerName string, send func([]BlobEntry)) error {
	serviceClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return err
	}

	err = s.listBlobsFlatWithClient(ctx, serviceClient, account, containerName, "", 0, send)
	if err == nil {
		return nil
	}
	if !isDataPlaneAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getSharedKeyServiceClient(ctx, account)
	if fallbackErr != nil {
		return fmt.Errorf("list all blobs for %s/%s with AAD failed: %v; shared key fallback failed: %w", account.Name, containerName, err, fallbackErr)
	}

	err = s.listBlobsFlatWithClient(ctx, fallbackClient, account, containerName, "", 0, send)
	if err != nil {
		return fmt.Errorf("list all blobs for %s/%s with shared key fallback: %w", account.Name, containerName, err)
	}

	return nil
}

func (s *Service) SearchBlobsByPrefix(ctx context.Context, account Account, containerName, prefix string, limit int, send func([]BlobEntry)) error {
	if strings.TrimSpace(prefix) == "" {
		return nil
	}

	serviceClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return err
	}

	err = s.listBlobsFlatWithClient(ctx, serviceClient, account, containerName, prefix, limit, send)
	if err == nil {
		return nil
	}
	if !isDataPlaneAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getSharedKeyServiceClient(ctx, account)
	if fallbackErr != nil {
		return fmt.Errorf("search blobs in %s/%s with AAD failed: %v; shared key fallback failed: %w", account.Name, containerName, err, fallbackErr)
	}

	err = s.listBlobsFlatWithClient(ctx, fallbackClient, account, containerName, prefix, limit, send)
	if err != nil {
		return fmt.Errorf("search blobs in %s/%s with shared key fallback: %w", account.Name, containerName, err)
	}

	return nil
}

func (s *Service) listBlobsFlatWithClient(ctx context.Context, serviceClient *service.Client, account Account, containerName, prefix string, limit int, send func([]BlobEntry)) error {
	containerClient := serviceClient.NewContainerClient(containerName)

	options := &container.ListBlobsFlatOptions{}
	if strings.TrimSpace(prefix) != "" {
		normalizedPrefix := prefix
		options.Prefix = &normalizedPrefix
	}

	total := 0
	pager := containerClient.NewListBlobsFlatPager(options)
	for pager.More() {
		if limit > 0 && total >= limit {
			break
		}

		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list flat blobs for %s/%s with prefix %q: %w", account.Name, containerName, prefix, err)
		}

		var batch []BlobEntry
		for _, blobItem := range page.Segment.BlobItems {
			if blobItem == nil || blobItem.Name == nil {
				continue
			}

			entry := BlobEntry{Name: *blobItem.Name}
			if blobItem.Properties != nil {
				if blobItem.Properties.ContentLength != nil {
					entry.Size = *blobItem.Properties.ContentLength
				}
				if blobItem.Properties.ContentType != nil {
					entry.ContentType = *blobItem.Properties.ContentType
				}
				if blobItem.Properties.LastModified != nil {
					entry.LastModified = *blobItem.Properties.LastModified
				}
				if blobItem.Properties.AccessTier != nil {
					entry.AccessTier = string(*blobItem.Properties.AccessTier)
				}
			}
			entry.MetadataCount = len(blobItem.Metadata)

			batch = append(batch, entry)
			if limit > 0 && total+len(batch) >= limit {
				break
			}
		}

		if len(batch) > 0 {
			send(batch)
			total += len(batch)
		}
	}

	return nil
}

func (s *Service) SearchBlobsContains(ctx context.Context, account Account, containerName, query string, limit int, send func([]BlobEntry)) error {
	if strings.TrimSpace(query) == "" {
		return nil
	}

	serviceClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return err
	}

	err = s.searchBlobsContainsWithClient(ctx, serviceClient, account, containerName, query, limit, send)
	if err == nil {
		return nil
	}
	if !isDataPlaneAuthError(err) {
		return err
	}

	fallbackClient, fallbackErr := s.getSharedKeyServiceClient(ctx, account)
	if fallbackErr != nil {
		return fmt.Errorf("search blobs in %s/%s with AAD failed: %v; shared key fallback failed: %w", account.Name, containerName, err, fallbackErr)
	}

	err = s.searchBlobsContainsWithClient(ctx, fallbackClient, account, containerName, query, limit, send)
	if err != nil {
		return fmt.Errorf("search blobs in %s/%s with shared key fallback: %w", account.Name, containerName, err)
	}

	return nil
}

func (s *Service) searchBlobsContainsWithClient(ctx context.Context, serviceClient *service.Client, account Account, containerName, query string, limit int, send func([]BlobEntry)) error {
	containerClient := serviceClient.NewContainerClient(containerName)
	needle := strings.ToLower(query)

	total := 0
	pager := containerClient.NewListBlobsFlatPager(nil)
	for pager.More() {
		if limit > 0 && total >= limit {
			break
		}

		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("search blobs in %s/%s for %q: %w", account.Name, containerName, query, err)
		}

		var batch []BlobEntry
		for _, blobItem := range page.Segment.BlobItems {
			if blobItem.Name == nil {
				continue
			}

			if !strings.Contains(strings.ToLower(*blobItem.Name), needle) {
				continue
			}

			if blobItem == nil || blobItem.Name == nil {
				continue
			}

			entry := BlobEntry{Name: *blobItem.Name}
			if blobItem.Properties != nil {
				if blobItem.Properties.ContentLength != nil {
					entry.Size = *blobItem.Properties.ContentLength
				}
				if blobItem.Properties.ContentType != nil {
					entry.ContentType = *blobItem.Properties.ContentType
				}
				if blobItem.Properties.LastModified != nil {
					entry.LastModified = *blobItem.Properties.LastModified
				}
				if blobItem.Properties.AccessTier != nil {
					entry.AccessTier = string(*blobItem.Properties.AccessTier)
				}
			}
			entry.MetadataCount = len(blobItem.Metadata)

			batch = append(batch, entry)

			if limit > 0 && total+len(batch) >= limit {
				break
			}
		}

		if len(batch) > 0 {
			send(batch)
			total += len(batch)
		}
	}

	return nil
}

func (s *Service) DownloadBlobs(ctx context.Context, account Account, containerName string, blobNames []string, destinationRoot string) ([]BlobDownloadResult, error) {
	if strings.TrimSpace(containerName) == "" {
		return nil, fmt.Errorf("container name is required")
	}
	if strings.TrimSpace(destinationRoot) == "" {
		return nil, fmt.Errorf("destination root is required")
	}

	names := dedupeAndSortBlobNames(blobNames)
	if len(names) == 0 {
		return nil, fmt.Errorf("no blob names provided")
	}

	root := filepath.Clean(destinationRoot)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create destination root %s: %w", root, err)
	}

	aadClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return nil, err
	}

	results := make([]BlobDownloadResult, 0, len(names))
	var sharedKeyClient *service.Client
	for _, blobName := range names {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}

		destinationPath, pathErr := destinationPathForBlob(root, blobName)
		if pathErr != nil {
			results = append(results, BlobDownloadResult{BlobName: blobName, Err: pathErr})
			continue
		}

		downloadErr := s.downloadBlobWithClient(ctx, aadClient, containerName, blobName, destinationPath)
		if downloadErr != nil && isDataPlaneAuthError(downloadErr) {
			if sharedKeyClient == nil {
				var sharedErr error
				sharedKeyClient, sharedErr = s.getSharedKeyServiceClient(ctx, account)
				if sharedErr != nil {
					downloadErr = fmt.Errorf("aad auth failed: %v; shared key fallback failed: %w", downloadErr, sharedErr)
				}
			}
			if sharedKeyClient != nil {
				downloadErr = s.downloadBlobWithClient(ctx, sharedKeyClient, containerName, blobName, destinationPath)
			}
		}

		results = append(results, BlobDownloadResult{
			BlobName:    blobName,
			Destination: destinationPath,
			Err:         downloadErr,
		})
	}

	return results, nil
}

func (s *Service) downloadBlobWithClient(ctx context.Context, serviceClient *service.Client, containerName, blobName, destinationPath string) error {
	containerClient := serviceClient.NewContainerClient(containerName)
	blobClient := containerClient.NewBlobClient(blobName)

	downloadResponse, err := blobClient.DownloadStream(ctx, nil)
	if err != nil {
		return fmt.Errorf("download blob %s: %w", blobName, err)
	}
	defer downloadResponse.Body.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory for %s: %w", destinationPath, err)
	}

	file, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("create destination file %s: %w", destinationPath, err)
	}

	if _, err := io.Copy(file, downloadResponse.Body); err != nil {
		file.Close()
		_ = os.Remove(destinationPath)
		return fmt.Errorf("write blob %s to %s: %w", blobName, destinationPath, err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return fmt.Errorf("close destination file %s: %w", destinationPath, err)
	}

	return nil
}

func dedupeAndSortBlobNames(blobNames []string) []string {
	if len(blobNames) == 0 {
		return nil
	}

	uniqueNames := make(map[string]struct{}, len(blobNames))
	for _, name := range blobNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		uniqueNames[trimmed] = struct{}{}
	}

	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func destinationPathForBlob(root, blobName string) (string, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(blobName, "\\", "/"))
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "" {
		return "", fmt.Errorf("blob name is empty")
	}

	relativePath := filepath.Clean(filepath.FromSlash(normalized))
	if relativePath == "." || relativePath == ".." {
		return "", fmt.Errorf("invalid blob name %q", blobName)
	}
	if strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("blob name escapes destination root: %q", blobName)
	}

	cleanRoot := filepath.Clean(root)
	destinationPath := filepath.Join(cleanRoot, relativePath)
	relativeToRoot, err := filepath.Rel(cleanRoot, destinationPath)
	if err != nil {
		return "", fmt.Errorf("compute destination path for %q: %w", blobName, err)
	}
	if relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("blob name escapes destination root: %q", blobName)
	}

	return destinationPath, nil
}

func (s *Service) getAADServiceClient(blobEndpoint string) (*service.Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(blobEndpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("empty blob endpoint")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if client, ok := s.aadClients[endpoint]; ok {
		return client, nil
	}

	client, err := service.NewClient(endpoint, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create blob service client for endpoint %s: %w", endpoint, err)
	}

	s.aadClients[endpoint] = client
	return client, nil
}

func (s *Service) getSharedKeyServiceClient(ctx context.Context, account Account) (*service.Client, error) {
	if strings.TrimSpace(account.SubscriptionID) == "" || strings.TrimSpace(account.ResourceGroup) == "" || strings.TrimSpace(account.Name) == "" {
		return nil, fmt.Errorf("insufficient account metadata for shared key fallback")
	}

	endpoint := strings.TrimRight(strings.TrimSpace(account.BlobEndpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("empty blob endpoint")
	}

	s.mu.Lock()
	if client, ok := s.sharedKeyClients[endpoint]; ok {
		s.mu.Unlock()
		return client, nil
	}
	s.mu.Unlock()

	accountsClient, err := armstorage.NewAccountsClient(account.SubscriptionID, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create ARM accounts client for subscription %s: %w", account.SubscriptionID, err)
	}

	listKeysResponse, err := accountsClient.ListKeys(ctx, account.ResourceGroup, account.Name, nil)
	if err != nil {
		return nil, fmt.Errorf("list keys for %s/%s: %w", account.ResourceGroup, account.Name, err)
	}

	keyValue, err := pickAccountKey(listKeysResponse.Keys)
	if err != nil {
		return nil, fmt.Errorf("select key for %s: %w", account.Name, err)
	}

	sharedKeyCredential, err := service.NewSharedKeyCredential(account.Name, keyValue)
	if err != nil {
		return nil, fmt.Errorf("create shared key credential for %s: %w", account.Name, err)
	}

	sharedKeyClient, err := service.NewClientWithSharedKeyCredential(endpoint, sharedKeyCredential, nil)
	if err != nil {
		return nil, fmt.Errorf("create shared key service client for endpoint %s: %w", endpoint, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.sharedKeyClients[endpoint]; ok {
		return existing, nil
	}

	s.sharedKeyClients[endpoint] = sharedKeyClient
	return sharedKeyClient, nil
}

func pickAccountKey(keys []*armstorage.AccountKey) (string, error) {
	fallback := ""
	for _, key := range keys {
		if key == nil || key.Value == nil || *key.Value == "" {
			continue
		}

		value := *key.Value
		if key.Permissions != nil && *key.Permissions == armstorage.KeyPermissionFull {
			return value, nil
		}

		if fallback == "" {
			fallback = value
		}
	}

	if fallback == "" {
		return "", fmt.Errorf("no usable storage account key returned")
	}

	return fallback, nil
}

func isDataPlaneAuthError(err error) bool {
	var responseErr *azcore.ResponseError
	if !errors.As(err, &responseErr) {
		return false
	}

	return responseErr.StatusCode == http.StatusUnauthorized || responseErr.StatusCode == http.StatusForbidden
}

func parseResourceGroup(resourceID *string) string {
	if resourceID == nil {
		return ""
	}

	segments := strings.Split(*resourceID, "/")
	for i := 0; i < len(segments)-1; i++ {
		if strings.EqualFold(segments[i], "resourceGroups") {
			return segments[i+1]
		}
	}

	return ""
}
