package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearStagedAdminHashAtomicAndExact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("Host: 127.0.0.1\nAdministrator:\n  Email: admin@example.invalid\n  Password: ''\n  PasswordHash: staged-hash\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ClearStagedAdminHash(path, "wrong"); err == nil {
		t.Fatal("unexpected hash accepted")
	}
	if got, _ := os.ReadFile(path); string(got) != string(original) {
		t.Fatal("failed cleanup changed file")
	}
	if err := ClearStagedAdminHash(path, "staged-hash"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "PasswordHash") || strings.Contains(string(got), "staged-hash") || !strings.Contains(string(got), "admin@example.invalid") {
		t.Fatalf("unexpected cleaned config %s", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("bad permissions %v %v", info, err)
	}
	if err := ClearStagedAdminHash(path, "staged-hash"); err != nil {
		t.Fatalf("cleanup is not idempotent: %v", err)
	}
}
