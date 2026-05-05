package blob

import (
	"fmt"
	"strings"
)

// AzuriteConnectionString is the well-known default connection string
// for the Azurite local emulator. The account name and key are public
// and identical for every Azurite install — they're not secrets, just
// the dev-time identifier baked into the emulator. The default Blob
// endpoint listens on 127.0.0.1:10000.
const AzuriteConnectionString = "DefaultEndpointsProtocol=http;" +
	"AccountName=devstoreaccount1;" +
	"AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;" +
	"BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"

// AccountFromConnectionString parses an Azure Storage connection string
// into an Account configured for shared-key (data-plane only) access.
// The returned Account has SharedKeyOnly=true so blob.Service skips the
// AAD path entirely — connection-string tabs have no Azure identity to
// fall back to.
//
// Required fields: AccountName, AccountKey. BlobEndpoint is used when
// present; otherwise it's synthesized from DefaultEndpointsProtocol and
// EndpointSuffix in the same form the SDK uses.
func AccountFromConnectionString(connectionString string) (Account, error) {
	fields, err := parseConnectionString(connectionString)
	if err != nil {
		return Account{}, err
	}

	name := fields["AccountName"]
	key := fields["AccountKey"]
	if name == "" {
		return Account{}, fmt.Errorf("connection string missing AccountName")
	}
	if key == "" {
		return Account{}, fmt.Errorf("connection string missing AccountKey")
	}

	endpoint := strings.TrimRight(fields["BlobEndpoint"], "/")
	if endpoint == "" {
		protocol := fields["DefaultEndpointsProtocol"]
		if protocol == "" {
			protocol = "https"
		}
		suffix := fields["EndpointSuffix"]
		if suffix == "" {
			suffix = "core.windows.net"
		}
		endpoint = fmt.Sprintf("%s://%s.blob.%s", protocol, name, suffix)
	}

	return Account{
		Name:          name,
		BlobEndpoint:  endpoint,
		SharedKey:     key,
		SharedKeyOnly: true,
	}, nil
}

// parseConnectionString splits a `key=value;key=value` connection string
// into a map. Whitespace around keys/values is trimmed; empty segments
// (e.g. trailing semicolons) are skipped. Values may contain `=` (URLs
// usually do not, but be tolerant) — only the first `=` separates key
// from value.
func parseConnectionString(s string) (map[string]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("connection string is empty")
	}

	out := make(map[string]string)
	for _, segment := range strings.Split(s, ";") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		idx := strings.IndexByte(segment, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("malformed connection string segment: %q", segment)
		}
		key := strings.TrimSpace(segment[:idx])
		value := strings.TrimSpace(segment[idx+1:])
		out[key] = value
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("connection string contained no fields")
	}
	return out, nil
}
