package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	bak := path + ".bak"

	// no-op when the file does not exist
	if err := backupFile(path); err != nil {
		t.Fatalf("backupFile on missing file: %v", err)
	}
	if _, err := os.Stat(bak); !os.IsNotExist(err) {
		t.Errorf("expected no %s, err = %v", bak, err)
	}

	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := backupFile(path); err != nil {
		t.Fatalf("backupFile: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to still exist, err = %v", path, err)
	}
	data, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(data) != "old" {
		t.Errorf("backup content = %q, want %q", data, "old")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.txt")

	if err := writeFileAtomic(path, []byte("one")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	if err := writeFileAtomic(path, []byte("two")); err != nil {
		t.Fatalf("writeFileAtomic overwrite: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil || string(data) != "two" {
		t.Fatalf("content = %q, err = %v, want %q", data, err, "two")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("expected only the output file, got %d entries", len(entries))
	}
}

func TestExpandHome(t *testing.T) {
	home := os.Getenv("HOME")
	tests := map[string]string{
		"~/.config/foo": home + "/.config/foo",
		"~/":            home + "/",
		"~":             home,
		"/abs/path":     "/abs/path",
		"relative/path": "relative/path",
		"":              "",
	}
	for input, want := range tests {
		got := expandHome(input)
		if got != want {
			t.Errorf("expandHome(%q) = %q, want %q", input, got, want)
		}
	}
}
