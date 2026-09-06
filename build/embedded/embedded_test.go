package embedded

import (
	"io/fs"
	"testing"
)

// The desktop application reads the staged files straight from the root of
// this file system, so the prebuild output must be visible there.
func TestStagedFilesAreExposed(t *testing.T) {
	entries, err := fs.ReadDir(Files, ".")
	if err != nil {
		t.Fatalf("ReadDir(.) error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the staging directory is empty; run build/helper-prebuild.go")
	}
	if _, err := fs.ReadFile(Files, "README.md"); err != nil {
		t.Fatalf("ReadFile(README.md) error = %v", err)
	}
}
