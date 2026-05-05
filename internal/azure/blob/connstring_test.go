package blob

import "testing"

func TestAccountFromConnectionString_Azurite(t *testing.T) {
	got, err := AccountFromConnectionString(AzuriteConnectionString)
	if err != nil {
		t.Fatalf("parse Azurite preset: %v", err)
	}
	if got.Name != "devstoreaccount1" {
		t.Errorf("Name = %q, want devstoreaccount1", got.Name)
	}
	if got.BlobEndpoint != "http://127.0.0.1:10000/devstoreaccount1" {
		t.Errorf("BlobEndpoint = %q, want http://127.0.0.1:10000/devstoreaccount1", got.BlobEndpoint)
	}
	if got.SharedKey == "" {
		t.Error("SharedKey is empty")
	}
	if !got.SharedKeyOnly {
		t.Error("SharedKeyOnly = false, want true")
	}
}

func TestAccountFromConnectionString_SynthesizesEndpoint(t *testing.T) {
	conn := "DefaultEndpointsProtocol=https;AccountName=mystorage;AccountKey=Zm9vYmFy;EndpointSuffix=core.windows.net"
	got, err := AccountFromConnectionString(conn)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.BlobEndpoint != "https://mystorage.blob.core.windows.net" {
		t.Errorf("BlobEndpoint = %q, want https://mystorage.blob.core.windows.net", got.BlobEndpoint)
	}
}

func TestAccountFromConnectionString_DefaultsWhenSparse(t *testing.T) {
	conn := "AccountName=acct;AccountKey=Zm9v"
	got, err := AccountFromConnectionString(conn)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.BlobEndpoint != "https://acct.blob.core.windows.net" {
		t.Errorf("BlobEndpoint = %q, want https://acct.blob.core.windows.net", got.BlobEndpoint)
	}
}

func TestAccountFromConnectionString_TrimsTrailingSlash(t *testing.T) {
	conn := "AccountName=acct;AccountKey=Zm9v;BlobEndpoint=http://localhost:10000/acct/"
	got, err := AccountFromConnectionString(conn)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.BlobEndpoint != "http://localhost:10000/acct" {
		t.Errorf("BlobEndpoint = %q, trailing slash not trimmed", got.BlobEndpoint)
	}
}

func TestAccountFromConnectionString_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"only-whitespace", "   "},
		{"missing-name", "AccountKey=Zm9v"},
		{"missing-key", "AccountName=acct"},
		{"malformed-segment", "AccountName=acct;junk;AccountKey=Zm9v"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AccountFromConnectionString(tc.in); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
