package azure

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

type BlobProperties struct {
	Size        int64
	ContentType string
}

func (s *Service) GetBlobProperties(ctx context.Context, account Account, containerName, blobName string) (BlobProperties, error) {
	serviceClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return BlobProperties{}, err
	}

	props, err := s.getBlobPropertiesWithClient(ctx, serviceClient, account, containerName, blobName)
	if err == nil {
		return props, nil
	}
	if !isDataPlaneAuthError(err) {
		return BlobProperties{}, err
	}

	fallbackClient, fallbackErr := s.getSharedKeyServiceClient(ctx, account)
	if fallbackErr != nil {
		return BlobProperties{}, fmt.Errorf("get blob properties for %s/%s/%s with AAD failed: %v; shared key fallback failed: %w", account.Name, containerName, blobName, err, fallbackErr)
	}

	props, err = s.getBlobPropertiesWithClient(ctx, fallbackClient, account, containerName, blobName)
	if err != nil {
		return BlobProperties{}, fmt.Errorf("get blob properties for %s/%s/%s with shared key fallback: %w", account.Name, containerName, blobName, err)
	}

	return props, nil
}

func (s *Service) getBlobPropertiesWithClient(ctx context.Context, serviceClient *service.Client, account Account, containerName, blobName string) (BlobProperties, error) {
	containerClient := serviceClient.NewContainerClient(containerName)
	blobClient := containerClient.NewBlobClient(blobName)

	resp, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		return BlobProperties{}, fmt.Errorf("get blob properties for %s/%s/%s: %w", account.Name, containerName, blobName, err)
	}

	props := BlobProperties{}
	if resp.ContentLength != nil {
		props.Size = *resp.ContentLength
	}
	if resp.ContentType != nil {
		props.ContentType = *resp.ContentType
	}

	return props, nil
}

func (s *Service) ReadBlobRange(ctx context.Context, account Account, containerName, blobName string, offset, count int64) ([]byte, error) {
	serviceClient, err := s.getAADServiceClient(account.BlobEndpoint)
	if err != nil {
		return nil, err
	}

	data, err := s.readBlobRangeWithClient(ctx, serviceClient, account, containerName, blobName, offset, count)
	if err == nil {
		return data, nil
	}
	if !isDataPlaneAuthError(err) {
		return nil, err
	}

	fallbackClient, fallbackErr := s.getSharedKeyServiceClient(ctx, account)
	if fallbackErr != nil {
		return nil, fmt.Errorf("read blob range for %s/%s/%s with AAD failed: %v; shared key fallback failed: %w", account.Name, containerName, blobName, err, fallbackErr)
	}

	data, err = s.readBlobRangeWithClient(ctx, fallbackClient, account, containerName, blobName, offset, count)
	if err != nil {
		return nil, fmt.Errorf("read blob range for %s/%s/%s with shared key fallback: %w", account.Name, containerName, blobName, err)
	}

	return data, nil
}

func (s *Service) readBlobRangeWithClient(ctx context.Context, serviceClient *service.Client, account Account, containerName, blobName string, offset, count int64) ([]byte, error) {
	if offset < 0 {
		offset = 0
	}

	containerClient := serviceClient.NewContainerClient(containerName)
	blobClient := containerClient.NewBlobClient(blobName)

	var options *blob.DownloadStreamOptions
	if offset > 0 || count > 0 {
		rangeSpec := blob.HTTPRange{Offset: offset}
		if count > 0 {
			rangeSpec.Count = count
		}
		options = &blob.DownloadStreamOptions{Range: rangeSpec}
	}

	resp, err := blobClient.DownloadStream(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("read blob range for %s/%s/%s (offset=%d,count=%d): %w", account.Name, containerName, blobName, offset, count, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read blob stream for %s/%s/%s: %w", account.Name, containerName, blobName, err)
	}

	return data, nil
}
