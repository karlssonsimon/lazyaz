package sbapp

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
)

// BlobRef is a parsed Event Grid Microsoft.Storage.Blob* reference,
// pointing at a specific blob in a specific account. Empty SubscriptionID
// or AccountName means parse failed.
type BlobRef struct {
	SubscriptionID string
	AccountName    string
	ContainerName  string
	Prefix         string // parent folder, "" or trailing slash form ("a/b/")
	BlobName       string // leaf name (the blob itself)
}

// LeafPath returns the path component as it appears in the subject —
// "<prefix><blob>". Used for menu labels.
func (r BlobRef) LeafPath() string { return r.Prefix + r.BlobName }

// eventGridEnvelope is a partial schema for the fields we need. Other
// fields in the payload are ignored.
type eventGridEnvelope struct {
	Topic     string `json:"topic"`
	Subject   string `json:"subject"`
	EventType string `json:"eventType"`
}

// topicRe matches an ARM resource ID for a storage account. The capture
// groups are subscriptionId, resourceGroup (unused but kept so the regex
// reflects the full path), accountName.
var topicRe = regexp.MustCompile(`^/subscriptions/([0-9a-fA-F-]+)/resourceGroups/([^/]+)/providers/Microsoft\.Storage/storageAccounts/([^/]+)$`)

// subjectRe matches the Event Grid subject path. The blob portion is
// captured as the rest of the string so paths with slashes survive.
var subjectRe = regexp.MustCompile(`^/blobServices/default/containers/([^/]+)/blobs/(.+)$`)

// parseBlobReference attempts to parse a service bus message body as an
// Event Grid Microsoft.Storage.Blob* event. Returns ok=false on any
// shape mismatch — callers should treat that as "not a blob event"
// silently (most messages aren't).
func parseBlobReference(body string) (BlobRef, bool) {
	body = strings.TrimSpace(body)
	if body == "" || body[0] != '{' {
		return BlobRef{}, false
	}
	var env eventGridEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return BlobRef{}, false
	}
	if !strings.HasPrefix(env.EventType, "Microsoft.Storage.Blob") {
		return BlobRef{}, false
	}

	tm := topicRe.FindStringSubmatch(env.Topic)
	if tm == nil {
		return BlobRef{}, false
	}
	sm := subjectRe.FindStringSubmatch(env.Subject)
	if sm == nil {
		return BlobRef{}, false
	}

	prefix, leaf := splitBlobPath(sm[2])
	return BlobRef{
		SubscriptionID: tm[1],
		AccountName:    tm[3],
		ContainerName:  sm[1],
		Prefix:         prefix,
		BlobName:       leaf,
	}, true
}

// splitBlobPath breaks a blob path into (prefix, leaf). The prefix
// retains its trailing slash so it matches blobapp.m.prefix exactly.
// "a/b/c.xml" -> ("a/b/", "c.xml"); "c.xml" -> ("", "c.xml").
func splitBlobPath(p string) (prefix, leaf string) {
	dir, base := path.Split(p)
	return dir, base
}
