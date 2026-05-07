package sbapp

import "testing"

const blobCreatedSample = `{
  "topic": "/subscriptions/8af26010-a963-4ea9-84b7-c2791c322404/resourceGroups/rg-htg-stage/providers/Microsoft.Storage/storageAccounts/sthtgdevarc",
  "subject": "/blobServices/default/containers/picking-list-incoming/blobs/PL-PX165.xml",
  "eventType": "Microsoft.Storage.BlobCreated",
  "data": { "url": "https://sthtgdevarc.blob.core.windows.net/picking-list-incoming/PL-PX165.xml" }
}`

const blobDeletedSampleNested = `{
  "topic": "/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/acct",
  "subject": "/blobServices/default/containers/raw/blobs/year=2026/month=05/file.csv",
  "eventType": "Microsoft.Storage.BlobDeleted"
}`

func TestParseBlobReferenceCreated(t *testing.T) {
	got, ok := parseBlobReference(blobCreatedSample)
	if !ok {
		t.Fatal("expected parse to succeed for BlobCreated event")
	}
	if got.SubscriptionID != "8af26010-a963-4ea9-84b7-c2791c322404" {
		t.Errorf("SubscriptionID = %q", got.SubscriptionID)
	}
	if got.AccountName != "sthtgdevarc" {
		t.Errorf("AccountName = %q", got.AccountName)
	}
	if got.ContainerName != "picking-list-incoming" {
		t.Errorf("ContainerName = %q", got.ContainerName)
	}
	if got.Prefix != "" {
		t.Errorf("Prefix = %q, want empty (root-level blob)", got.Prefix)
	}
	if got.BlobName != "PL-PX165.xml" {
		t.Errorf("BlobName = %q", got.BlobName)
	}
}

func TestParseBlobReferenceNestedPath(t *testing.T) {
	got, ok := parseBlobReference(blobDeletedSampleNested)
	if !ok {
		t.Fatal("expected parse to succeed for nested-path blob event")
	}
	if got.Prefix != "year=2026/month=05/" {
		t.Errorf("Prefix = %q, want trailing-slash form", got.Prefix)
	}
	if got.BlobName != "file.csv" {
		t.Errorf("BlobName = %q", got.BlobName)
	}
	if got.LeafPath() != "year=2026/month=05/file.csv" {
		t.Errorf("LeafPath = %q", got.LeafPath())
	}
}

func TestParseBlobReferenceRejectsNonBlobEvents(t *testing.T) {
	cases := map[string]string{
		"masstransit-style": `{"messageId":"abc","message":{"x":1}}`,
		"wrong eventType":   `{"topic":"/subscriptions/x/resourceGroups/y/providers/Microsoft.Storage/storageAccounts/z","subject":"/blobServices/default/containers/c/blobs/x","eventType":"Microsoft.EventHub.CaptureFileCreated"}`,
		"missing subject":   `{"topic":"/subscriptions/x/resourceGroups/y/providers/Microsoft.Storage/storageAccounts/z","eventType":"Microsoft.Storage.BlobCreated"}`,
		"malformed json":    `{not json`,
		"empty body":        ``,
		"non-json":          `hello world`,
		"non-storage topic": `{"topic":"/subscriptions/x/resourceGroups/y/providers/Microsoft.EventGrid/topics/foo","subject":"/blobServices/default/containers/c/blobs/x","eventType":"Microsoft.Storage.BlobCreated"}`,
	}
	for name, body := range cases {
		if _, ok := parseBlobReference(body); ok {
			t.Errorf("%s: expected parse to fail, but it succeeded", name)
		}
	}
}
