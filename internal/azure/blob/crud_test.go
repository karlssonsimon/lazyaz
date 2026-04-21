package blob

import (
	"context"
	"testing"
)

// Compile-time signature assertions — Azure-backed behavior isn't tested.
var _ func(*Service, context.Context, Account, string, string) error = (*Service).DeleteBlob
var _ func(*Service, context.Context, Account, string, []string) ([]BlobDeleteResult, error) = (*Service).DeleteBlobs
var _ func(*Service, context.Context, Account, string, string, string) error = (*Service).RenameBlob
var _ func(*Service, context.Context, Account, string) error = (*Service).CreateContainer
var _ func(*Service, context.Context, Account, string) error = (*Service).DeleteContainer
var _ func(*Service, context.Context, Account, string, string) error = (*Service).CreateDirectory
var _ func(*Service, context.Context, Account, string, string) error = (*Service).DeleteDirectory
var _ func(*Service, context.Context, Account, string, string, string) error = (*Service).RenameDirectory

func TestValidateContainerNameRejectsShort(t *testing.T) {
	if ValidateContainerName("ab") == "" {
		t.Fatalf("expected error for 2-char name")
	}
}

func TestValidateContainerNameRejectsLong(t *testing.T) {
	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if ValidateContainerName(string(long)) == "" {
		t.Fatalf("expected error for 64-char name")
	}
}

func TestValidateContainerNameRejectsUppercase(t *testing.T) {
	if ValidateContainerName("MyContainer") == "" {
		t.Fatalf("expected error for uppercase")
	}
}

func TestValidateContainerNameRejectsUnderscore(t *testing.T) {
	if ValidateContainerName("my_container") == "" {
		t.Fatalf("expected error for underscore")
	}
}

func TestValidateContainerNameRejectsLeadingHyphen(t *testing.T) {
	if ValidateContainerName("-leading") == "" {
		t.Fatalf("expected error for leading hyphen")
	}
}

func TestValidateContainerNameRejectsTrailingHyphen(t *testing.T) {
	if ValidateContainerName("trailing-") == "" {
		t.Fatalf("expected error for trailing hyphen")
	}
}

func TestValidateContainerNameRejectsConsecutiveHyphens(t *testing.T) {
	if ValidateContainerName("foo--bar") == "" {
		t.Fatalf("expected error for consecutive hyphens")
	}
}

func TestValidateContainerNameAcceptsValid(t *testing.T) {
	for _, name := range []string{
		"abc",
		"my-container",
		"logs2026",
		"a-b-c",
		"0-starts-with-digit",
	} {
		if msg := ValidateContainerName(name); msg != "" {
			t.Fatalf("name %q should be valid: %s", name, msg)
		}
	}
}
