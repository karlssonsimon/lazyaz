package blob

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azdatalake/directory"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azdatalake/file"
)

// DeleteBlob removes a single blob. Returns nil if the delete succeeds
// or if the blob was already gone; any other SDK error is surfaced.
func (s *Service) DeleteBlob(ctx context.Context, account Account, containerName, blobName string) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name is required")
	}
	if strings.TrimSpace(blobName) == "" {
		return fmt.Errorf("blob name is required")
	}
	return s.withFallback(ctx, account, fmt.Sprintf("delete %s from %s/%s", blobName, account.Name, containerName), func(c *service.Client) error {
		bc := c.NewContainerClient(containerName).NewBlobClient(blobName)
		_, err := bc.Delete(ctx, nil)
		return err
	})
}

// BlobDeleteResult mirrors BlobDownloadResult: one entry per requested
// blob, carrying either a nil Err (success) or the SDK error.
type BlobDeleteResult struct {
	BlobName string
	Err      error
}

// DeleteBlobs removes a batch of blobs sequentially, one withFallback
// per blob (so per-blob auth fallback behaves correctly). Returns a
// slice with one result per input name, in the same order. An error
// on any one blob does not abort the batch.
func (s *Service) DeleteBlobs(ctx context.Context, account Account, containerName string, blobNames []string) ([]BlobDeleteResult, error) {
	if strings.TrimSpace(containerName) == "" {
		return nil, fmt.Errorf("container name is required")
	}
	results := make([]BlobDeleteResult, 0, len(blobNames))
	for _, name := range blobNames {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		err := s.DeleteBlob(ctx, account, containerName, name)
		results = append(results, BlobDeleteResult{BlobName: name, Err: err})
	}
	return results, nil
}

// RenameBlob renames a blob within the same container. On HNS-enabled
// accounts it uses the atomic Data Lake path rename API (O(1), preserves
// metadata/versions/permissions). On flat-namespace accounts it falls
// back to the async server-side copy + delete path.
func (s *Service) RenameBlob(ctx context.Context, account Account, containerName, oldName, newName string) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name is required")
	}
	if strings.TrimSpace(oldName) == "" {
		return fmt.Errorf("source blob name is required")
	}
	if strings.TrimSpace(newName) == "" {
		return fmt.Errorf("destination blob name is required")
	}
	if oldName == newName {
		return fmt.Errorf("source and destination blob names are identical")
	}
	if account.IsHnsEnabled {
		return s.renameBlobHNS(ctx, account, containerName, oldName, newName)
	}
	return s.renameBlobCopyDelete(ctx, account, containerName, oldName, newName)
}

// renameBlobHNS uses the Data Lake path rename API for an atomic,
// metadata-preserving rename on HNS accounts. O(1) instead of O(bytes).
func (s *Service) renameBlobHNS(ctx context.Context, account Account, containerName, oldName, newName string) error {
	dfs := dfsEndpoint(account)
	if dfs == "" {
		return fmt.Errorf("cannot derive DFS endpoint for account %s", account.Name)
	}
	srcURL := fmt.Sprintf("%s/%s/%s", dfs, containerName, strings.TrimPrefix(oldName, "/"))
	dest := containerName + "/" + strings.TrimPrefix(newName, "/")
	label := fmt.Sprintf("rename %s → %s in %s/%s", oldName, newName, account.Name, containerName)
	return s.withHNSFileFallback(ctx, account, srcURL, label, func(c *file.Client) error {
		_, err := c.Rename(ctx, dest, nil)
		return err
	})
}

// renameBlobCopyDelete copies the source blob to newName within the same
// container, polls until the server-side copy completes, then deletes
// the source. Works for any blob size because it uses the async copy
// API. Same-account / same-container rename only — cross-container or
// cross-account rename would need a wizard in the UI.
func (s *Service) renameBlobCopyDelete(ctx context.Context, account Account, containerName, oldName, newName string) error {
	if account.BlobEndpoint == "" {
		return fmt.Errorf("blob endpoint not set for account %s", account.Name)
	}
	return s.withFallback(ctx, account, fmt.Sprintf("rename %s → %s in %s/%s", oldName, newName, account.Name, containerName), func(c *service.Client) error {
		srcURL := fmt.Sprintf("%s/%s/%s", account.BlobEndpoint, containerName, oldName)
		dst := c.NewContainerClient(containerName).NewBlobClient(newName)
		if _, err := dst.StartCopyFromURL(ctx, srcURL, nil); err != nil {
			return fmt.Errorf("start copy: %w", err)
		}
		// Poll destination properties until CopyStatus is Success.
		for {
			props, err := dst.GetProperties(ctx, nil)
			if err != nil {
				return fmt.Errorf("poll copy: %w", err)
			}
			if props.CopyStatus == nil {
				break // no copy metadata on properties → finished
			}
			switch *props.CopyStatus {
			case blob.CopyStatusTypeSuccess:
				// Delete source.
				src := c.NewContainerClient(containerName).NewBlobClient(oldName)
				if _, err := src.Delete(ctx, nil); err != nil {
					return fmt.Errorf("delete source: %w", err)
				}
				return nil
			case blob.CopyStatusTypeFailed:
				desc := ""
				if props.CopyStatusDescription != nil {
					desc = *props.CopyStatusDescription
				}
				return fmt.Errorf("copy failed: %s", desc)
			case blob.CopyStatusTypeAborted:
				return fmt.Errorf("copy aborted")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
		// CopyStatus nil means no copy was in flight — source might
		// already be copied or SDK fast-path. Just delete source.
		src := c.NewContainerClient(containerName).NewBlobClient(oldName)
		if _, err := src.Delete(ctx, nil); err != nil {
			return fmt.Errorf("delete source: %w", err)
		}
		return nil
	})
}

// CreateContainer creates a new container in the given storage account.
// Returns an error if the container already exists or the name is invalid
// per ValidateContainerName.
func (s *Service) CreateContainer(ctx context.Context, account Account, containerName string) error {
	if msg := ValidateContainerName(containerName); msg != "" {
		return fmt.Errorf("invalid container name: %s", msg)
	}
	return s.withFallback(ctx, account, fmt.Sprintf("create container %s in %s", containerName, account.Name), func(c *service.Client) error {
		cc := c.NewContainerClient(containerName)
		_, err := cc.Create(ctx, nil)
		return err
	})
}

// DeleteContainer removes a container and every blob it holds. This is
// destructive — the caller (UI layer) must confirm with the user first.
func (s *Service) DeleteContainer(ctx context.Context, account Account, containerName string) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name is required")
	}
	return s.withFallback(ctx, account, fmt.Sprintf("delete container %s from %s", containerName, account.Name), func(c *service.Client) error {
		cc := c.NewContainerClient(containerName)
		_, err := cc.Delete(ctx, nil)
		return err
	})
}

// ValidateContainerName enforces Azure's container naming rules.
// Returns an empty string for valid names; otherwise a human-readable
// error message suitable for display in a text-input overlay.
//
// Rules per Azure docs:
//   - 3 to 63 characters
//   - lowercase letters, digits, and single hyphens only
//   - must start and end with letter or digit
//   - no consecutive hyphens
func ValidateContainerName(name string) string {
	if len(name) < 3 {
		return "must be at least 3 characters"
	}
	if len(name) > 63 {
		return "must be at most 63 characters"
	}
	if !containerNameRe.MatchString(name) {
		return "lowercase letters, digits, and single hyphens only"
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return "cannot start or end with a hyphen"
	}
	if strings.Contains(name, "--") {
		return "cannot contain consecutive hyphens"
	}
	return ""
}

var containerNameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// DeleteDirectory recursively deletes a directory and all its contents.
// HNS accounts only — caller must gate on account.IsHnsEnabled. The
// operation is server-side atomic (one API call), unlike a walk-and-
// delete loop against a flat-namespace account.
func (s *Service) DeleteDirectory(ctx context.Context, account Account, containerName, directoryPath string) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name is required")
	}
	directoryPath = strings.Trim(directoryPath, "/")
	if directoryPath == "" {
		return fmt.Errorf("directory path is required")
	}
	if !account.IsHnsEnabled {
		return fmt.Errorf("directory deletion requires an HNS-enabled (Data Lake Gen2) account")
	}
	dfs := dfsEndpoint(account)
	if dfs == "" {
		return fmt.Errorf("cannot derive DFS endpoint for account %s", account.Name)
	}
	dirURL := fmt.Sprintf("%s/%s/%s", dfs, containerName, directoryPath)
	label := fmt.Sprintf("delete directory %s in %s/%s", directoryPath, account.Name, containerName)
	// directory.Client.Delete always recurses (the SDK hardcodes recursive=true internally).
	return s.withHNSDirFallback(ctx, account, dirURL, label, func(c *directory.Client) error {
		_, err := c.Delete(ctx, nil)
		return err
	})
}

// RenameDirectory atomically renames (or moves) a directory on an
// HNS-enabled account. newPath is relative to the container root
// (forward slashes, no leading/trailing slash). HNS only.
func (s *Service) RenameDirectory(ctx context.Context, account Account, containerName, oldPath, newPath string) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name is required")
	}
	oldPath = strings.Trim(oldPath, "/")
	newPath = strings.Trim(newPath, "/")
	if oldPath == "" || newPath == "" {
		return fmt.Errorf("both source and destination paths are required")
	}
	if oldPath == newPath {
		return fmt.Errorf("source and destination are identical")
	}
	if !account.IsHnsEnabled {
		return fmt.Errorf("directory rename requires an HNS-enabled (Data Lake Gen2) account")
	}
	dfs := dfsEndpoint(account)
	if dfs == "" {
		return fmt.Errorf("cannot derive DFS endpoint for account %s", account.Name)
	}
	srcURL := fmt.Sprintf("%s/%s/%s", dfs, containerName, oldPath)
	// destinationPath is filesystem-relative (container/path).
	dest := containerName + "/" + newPath
	label := fmt.Sprintf("rename directory %s → %s in %s", oldPath, newPath, account.Name)
	return s.withHNSDirFallback(ctx, account, srcURL, label, func(c *directory.Client) error {
		_, err := c.Rename(ctx, dest, nil)
		return err
	})
}

// dfsEndpoint returns the Data Lake (DFS) endpoint derived from the
// blob endpoint. Blob endpoint looks like https://<acct>.blob.core.windows.net;
// the DFS endpoint replaces "blob" with "dfs".
func dfsEndpoint(account Account) string {
	if account.BlobEndpoint == "" {
		return ""
	}
	return strings.Replace(account.BlobEndpoint, ".blob.", ".dfs.", 1)
}

// CreateDirectory creates an empty first-class directory via the
// Data Lake Storage Gen2 API. Only meaningful when account.IsHnsEnabled;
// the caller (UI layer) must gate this action on that flag.
//
// directoryPath is relative to the container root — forward-slash
// separators, no leading slash, no trailing slash.
func (s *Service) CreateDirectory(ctx context.Context, account Account, containerName, directoryPath string) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name is required")
	}
	directoryPath = strings.Trim(directoryPath, "/")
	if directoryPath == "" {
		return fmt.Errorf("directory path is required")
	}
	if !account.IsHnsEnabled {
		return fmt.Errorf("directory creation requires an HNS-enabled (Data Lake Gen2) account")
	}
	dfs := dfsEndpoint(account)
	if dfs == "" {
		return fmt.Errorf("cannot derive DFS endpoint for account %s", account.Name)
	}

	dirURL := fmt.Sprintf("%s/%s/%s", dfs, containerName, directoryPath)
	label := fmt.Sprintf("create directory %s in %s/%s", directoryPath, account.Name, containerName)
	return s.withHNSDirFallback(ctx, account, dirURL, label, func(c *directory.Client) error {
		_, err := c.Create(ctx, nil)
		return err
	})
}

// withHNSFileFallback runs op with an AAD-authenticated file.Client;
// on a data-plane auth error it retries with a shared-key-authenticated
// client whose credential is fetched via ARM ListKeys. Mirrors the
// flat-namespace withFallback but for azdatalake file clients, which
// don't share a common type with azblob service clients.
func (s *Service) withHNSFileFallback(ctx context.Context, account Account, url, label string, op func(*file.Client) error) error {
	aad, err := file.NewClient(url, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create file client: %w", err)
	}
	err = op(aad)
	if err == nil || !isDataPlaneAuthError(err) {
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		return nil
	}

	sk, fbErr := s.getDFSSharedKeyCredential(ctx, account)
	if fbErr != nil {
		return fmt.Errorf("%s with AAD failed: %v; shared key fallback failed: %w", label, err, fbErr)
	}
	shared, skErr := file.NewClientWithSharedKeyCredential(url, sk, nil)
	if skErr != nil {
		return fmt.Errorf("create shared-key file client: %w", skErr)
	}
	if err := op(shared); err != nil {
		return fmt.Errorf("%s with shared key fallback: %w", label, err)
	}
	return nil
}

// withHNSDirFallback is the directory.Client analogue of
// withHNSFileFallback.
func (s *Service) withHNSDirFallback(ctx context.Context, account Account, url, label string, op func(*directory.Client) error) error {
	aad, err := directory.NewClient(url, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create directory client: %w", err)
	}
	err = op(aad)
	if err == nil || !isDataPlaneAuthError(err) {
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		return nil
	}

	sk, fbErr := s.getDFSSharedKeyCredential(ctx, account)
	if fbErr != nil {
		return fmt.Errorf("%s with AAD failed: %v; shared key fallback failed: %w", label, err, fbErr)
	}
	shared, skErr := directory.NewClientWithSharedKeyCredential(url, sk, nil)
	if skErr != nil {
		return fmt.Errorf("create shared-key directory client: %w", skErr)
	}
	if err := op(shared); err != nil {
		return fmt.Errorf("%s with shared key fallback: %w", label, err)
	}
	return nil
}
