package credstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) Store {
	t.Helper()
	return Store{Root: filepath.Join(t.TempDir(), ".tmact", ".cred")}
}

func TestWriteReadRoundTripIsPrivate(t *testing.T) {
	s := newStore(t)
	path, err := s.Write("github-pat", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(s.Root, "github-pat") {
		t.Fatalf("path = %q", path)
	}
	for p, want := range map[string]os.FileMode{s.Root: 0o700, path: 0o600} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s mode = %04o, want %04o", p, info.Mode().Perm(), want)
		}
	}
	got, err := s.Read("github-pat")
	if err != nil || string(got) != "secret-value" {
		t.Fatalf("Read = %q, %v", got, err)
	}
	if _, err := s.Write("github-pat", []byte("rotated")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read("github-pat"); string(got) != "rotated" {
		t.Fatalf("after overwrite Read = %q", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(s.Root, ".tmp-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left: %v", leftovers)
	}
}

func TestInvalidNamesAreRejected(t *testing.T) {
	s := newStore(t)
	for _, name := range []string{"", ".hidden", "-x", "a/b", "..", "../escape", "a b", strings.Repeat("a", 65)} {
		if _, err := s.Write(name, []byte("v")); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Write(%q) err = %v, want ErrInvalidName", name, err)
		}
	}
	for _, name := range []string{"a", "GITHUB_TOKEN", "npm.registry-token", strings.Repeat("a", 64)} {
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false", name)
		}
	}
}

func TestWriteRejectsEmptyAndOversizedValues(t *testing.T) {
	s := newStore(t)
	if _, err := s.Write("x", nil); !errors.Is(err, ErrEmptyValue) {
		t.Fatalf("empty err = %v", err)
	}
	if _, err := s.Write("x", bytes.Repeat([]byte("a"), MaxValueSize+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized err = %v", err)
	}
}

func TestSymlinkedRootAndFileAreRefused(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: link}).Write("x", []byte("v")); err == nil {
		t.Fatal("write through symlinked root succeeded")
	}

	s := Store{Root: real}
	target := filepath.Join(base, "elsewhere")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(real, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read("linked"); err == nil {
		t.Fatal("read through symlinked file succeeded")
	}
	if _, err := s.Write("linked", []byte("v")); err == nil {
		t.Fatal("write over symlinked file succeeded")
	}
	if got, _ := os.ReadFile(target); string(got) != "outside" {
		t.Fatalf("symlink target changed to %q", got)
	}
	if ok, err := s.Exists("linked"); ok || err == nil {
		t.Fatalf("Exists(linked) = %v, %v", ok, err)
	}
}

func TestReadRefusesGroupReadableFile(t *testing.T) {
	s := newStore(t)
	path, err := s.Write("x", []byte("v"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read("x"); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("Read err = %v", err)
	}
}

func TestRootModeIsTightened(t *testing.T) {
	s := newStore(t)
	if _, err := s.Write("x", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read("x"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(s.Root)
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("root mode = %04o", info.Mode().Perm())
	}
}

func TestListExistsRemove(t *testing.T) {
	s := newStore(t)
	if entries, err := s.List(); err != nil || len(entries) != 0 {
		t.Fatalf("List on missing root = %v, %v", entries, err)
	}
	if _, err := s.Read("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read missing err = %v", err)
	}
	for _, name := range []string{"b", "a"} {
		if _, err := s.Write(name, []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.Root, ".tmp-stray"), []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List()
	if err != nil || len(entries) != 2 || entries[0].Name != "a" || entries[1].Name != "b" {
		t.Fatalf("List = %+v, %v", entries, err)
	}
	if ok, err := s.Exists("a"); !ok || err != nil {
		t.Fatalf("Exists(a) = %v, %v", ok, err)
	}
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Exists("a"); ok {
		t.Fatal("a still exists")
	}
	if err := s.Remove("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Remove err = %v", err)
	}
}
