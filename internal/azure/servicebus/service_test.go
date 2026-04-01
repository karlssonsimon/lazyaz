package servicebus

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

func TestParseResourceGroup(t *testing.T) {
	tests := []struct {
		name string
		id   *string
		want string
	}{
		{
			name: "nil id",
			id:   nil,
			want: "",
		},
		{
			name: "valid id",
			id:   strPtr("/subscriptions/abc/resourceGroups/rg-prod/providers/Microsoft.ServiceBus/namespaces/sb-demo"),
			want: "rg-prod",
		},
		{
			name: "missing resource groups segment",
			id:   strPtr("/subscriptions/abc/providers/Microsoft.ServiceBus/namespaces/sb-demo"),
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseResourceGroup(tc.id); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEndpointToFQDN(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "https with trailing slash",
			endpoint: "https://sb-demo.servicebus.windows.net/",
			want:     "sb-demo.servicebus.windows.net",
		},
		{
			name:     "https with port",
			endpoint: "https://sb-demo.servicebus.windows.net:443",
			want:     "sb-demo.servicebus.windows.net",
		},
		{
			name:     "plain fqdn",
			endpoint: "sb-demo.servicebus.windows.net",
			want:     "sb-demo.servicebus.windows.net",
		},
		{
			name:     "empty",
			endpoint: "",
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := endpointToFQDN(tc.endpoint); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		max  int
		want string
	}{
		{
			name: "empty body",
			body: nil,
			max:  512,
			want: "",
		},
		{
			name: "short body",
			body: []byte("hello"),
			max:  512,
			want: "hello",
		},
		{
			name: "exact limit",
			body: []byte("abc"),
			max:  3,
			want: "abc",
		},
		{
			name: "truncated",
			body: []byte("abcdef"),
			max:  3,
			want: "abc...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateBody(tc.body, tc.max); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error",
			err:  fmt.Errorf("something went wrong"),
			want: false,
		},
		{
			name: "service bus unauthorized access",
			err:  &azservicebus.Error{Code: azservicebus.CodeUnauthorizedAccess},
			want: true,
		},
		{
			name: "service bus other code",
			err:  &azservicebus.Error{Code: azservicebus.CodeNotFound},
			want: false,
		},
		{
			name: "wrapped service bus unauthorized",
			err:  fmt.Errorf("peek failed: %w", &azservicebus.Error{Code: azservicebus.CodeUnauthorizedAccess}),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAuthError(tc.err); got != tc.want {
				t.Fatalf("isAuthError() = %v, want %v", got, tc.want)
			}
		})
	}
}

func strPtr(v string) *string {
	return &v
}
