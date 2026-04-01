package blob

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

func TestParseResourceGroup(t *testing.T) {
	tests := []struct {
		name       string
		resourceID *string
		want       string
	}{
		{
			name:       "nil id",
			resourceID: nil,
			want:       "",
		},
		{
			name:       "valid id",
			resourceID: strPtr("/subscriptions/abc/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/demo"),
			want:       "rg-prod",
		},
		{
			name:       "missing resource groups segment",
			resourceID: strPtr("/subscriptions/abc/providers/Microsoft.Storage/storageAccounts/demo"),
			want:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseResourceGroup(tc.resourceID); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestPickAccountKey(t *testing.T) {
	readonly := armstorage.KeyPermissionRead
	full := armstorage.KeyPermissionFull

	tests := []struct {
		name    string
		keys    []*armstorage.AccountKey
		want    string
		wantErr bool
	}{
		{
			name: "prefers full permission key",
			keys: []*armstorage.AccountKey{
				{Permissions: &readonly, Value: strPtr("readonly-key")},
				{Permissions: &full, Value: strPtr("full-key")},
			},
			want: "full-key",
		},
		{
			name: "falls back to first key",
			keys: []*armstorage.AccountKey{
				{Permissions: &readonly, Value: strPtr("readonly-key")},
			},
			want: "readonly-key",
		},
		{
			name: "returns error when no usable keys",
			keys: []*armstorage.AccountKey{
				{},
				{Value: strPtr("")},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickAccountKey(tc.keys)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestIsDataPlaneAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "forbidden response",
			err:  &azcore.ResponseError{StatusCode: 403},
			want: true,
		},
		{
			name: "wrapped unauthorized response",
			err:  fmt.Errorf("outer: %w", &azcore.ResponseError{StatusCode: 401}),
			want: true,
		},
		{
			name: "non auth response",
			err:  &azcore.ResponseError{StatusCode: 404},
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("oops"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDataPlaneAuthError(tc.err); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestDedupeAndSortBlobNames(t *testing.T) {
	input := []string{"b/file.txt", "a/file.txt", "", "a/file.txt", "  c/file.txt  "}
	got := dedupeAndSortBlobNames(input)
	want := []string{"a/file.txt", "b/file.txt", "c/file.txt"}

	if len(got) != len(want) {
		t.Fatalf("expected %d names, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %q at index %d, got %q", want[i], i, got[i])
		}
	}
}

func TestDestinationPathForBlob(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		blob    string
		want    string
		wantErr bool
	}{
		{
			name: "nested path",
			root: "downloads/account/container",
			blob: "foo/bar.txt",
			want: "downloads/account/container/foo/bar.txt",
		},
		{
			name: "leading slash trimmed",
			root: "downloads",
			blob: "/foo.txt",
			want: "downloads/foo.txt",
		},
		{
			name:    "reject traversal",
			root:    "downloads",
			blob:    "../secret.txt",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := destinationPathForBlob(tc.root, tc.blob)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func strPtr(v string) *string {
	return &v
}
