package logger

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadLastNLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, accessFilename), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := readLastNLinesFromFile(filepath.Join(dir, accessFilename), 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"two", "three"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
}

func TestReadLastNLogLinesIncludesErrorLevels(t *testing.T) {
	dir := t.TempDir()
	for filename, content := range map[string]string{
		accessFilename: "access-1\naccess-2\n",
		errorFilename:  "error-1\n",
		slowFilename:   "slow-1\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := ReadLastNLogLines(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"access-2", "error-1", "slow-1"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
}
