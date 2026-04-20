package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

// ExistingBlobs returns the subset of blobNames that already exist in
// the container. Missing blobs are not in the returned set; blobs that
// fail the existence check for any reason other than "not found" are
// treated as unknown (omitted from the set, silently). This is a
// best-effort pre-flight check for the conflict prompt — a transient
// network error shouldn't block the upload.
func (s *Service) ExistingBlobs(ctx context.Context, account Account, containerName string, blobNames []string) (map[string]struct{}, error) {
	if len(blobNames) == 0 {
		return map[string]struct{}{}, nil
	}
	existing := make(map[string]struct{}, len(blobNames))
	err := s.withFallback(ctx, account, fmt.Sprintf("check existing blobs in %s/%s", account.Name, containerName), func(c *service.Client) error {
		containerClient := c.NewContainerClient(containerName)
		for _, name := range blobNames {
			if err := ctx.Err(); err != nil {
				return err
			}
			blobClient := containerClient.NewBlobClient(name)
			_, err := blobClient.GetProperties(ctx, nil)
			if err == nil {
				existing[name] = struct{}{}
				continue
			}
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && (respErr.ErrorCode == string(bloberror.BlobNotFound) || respErr.StatusCode == http.StatusNotFound) {
				continue
			}
			// Any other error for a single blob is ignored (soft degrade).
		}
		return nil
	})
	return existing, err
}

// UploadBlob streams localPath to the block blob at blobName inside the
// given container. Content-Type is inferred from the file extension via
// mime.TypeByExtension; falls back to application/octet-stream. The
// progress callback is invoked with cumulative bytes read for this
// file; callers use this to update UI progress. Uses UploadStream so
// large files don't load entirely into memory.
func (s *Service) UploadBlob(ctx context.Context, account Account, containerName, blobName, localPath string, progress func(bytes int64)) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name is required")
	}
	if strings.TrimSpace(blobName) == "" {
		return fmt.Errorf("blob name is required")
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", localPath, err)
	}
	defer file.Close()

	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return s.withFallback(ctx, account, fmt.Sprintf("upload %s to %s/%s", blobName, account.Name, containerName), func(c *service.Client) error {
		// withFallback may invoke this closure twice (AAD, then shared-key on
		// 401/403). Rewind on each attempt so the retry doesn't upload an empty body.
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("rewind %s: %w", localPath, err)
		}
		bbClient := c.NewContainerClient(containerName).NewBlockBlobClient(blobName)
		reader := &progressReader{r: file, onAdvance: progress}
		_, err := bbClient.UploadStream(ctx, reader, &blockblob.UploadStreamOptions{
			BlockSize:   8 * 1024 * 1024,
			Concurrency: transferConcurrency(),
			HTTPHeaders: &blob.HTTPHeaders{
				BlobContentType: &contentType,
			},
		})
		return err
	})
}

// transferConcurrency returns the parallelism used for block-blob stream
// uploads and downloads. Scales with CPU count but floors at 8 so even
// low-core machines saturate typical home/office uplinks.
func transferConcurrency() int {
	n := runtime.NumCPU()
	if n < 8 {
		n = 8
	}
	return n
}

// progressReader wraps an io.Reader and invokes onAdvance with the
// cumulative bytes read each time Read returns new bytes.
type progressReader struct {
	r         io.Reader
	total     int64
	onAdvance func(int64)
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.total += int64(n)
		if p.onAdvance != nil {
			p.onAdvance(p.total)
		}
	}
	return n, err
}
