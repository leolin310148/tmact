// Package credstore keeps named credentials as private files under
// ~/.tmact/.cred/. Values are only ever read in-process (for `tmact cred
// run`); nothing in this package formats a value for output.
package credstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"syscall"
	"time"
)

// MaxValueSize bounds one credential value.
const MaxValueSize = 64 << 10

var (
	ErrInvalidName = errors.New("invalid credential name")
	ErrNotFound    = errors.New("credential not found")
	ErrEmptyValue  = errors.New("credential value is empty")
	ErrTooLarge    = fmt.Errorf("credential value exceeds %d bytes", MaxValueSize)

	namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// Store is a directory of credential files.
type Store struct {
	Root string
}

// Entry describes a stored credential without its value.
type Entry struct {
	Name    string    `json:"name"`
	ModTime time.Time `json:"mod_time"`
}

// Default returns the store at ~/.tmact/.cred.
func Default() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, err
	}
	return Store{Root: filepath.Join(home, ".tmact", ".cred")}, nil
}

// ValidName reports whether name can be used as a credential name.
func ValidName(name string) bool {
	return namePattern.MatchString(name)
}

// Path returns where name is (or would be) stored.
func (s Store) Path(name string) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("%w %q: use 1-64 letters, digits, '.', '_' or '-', not starting with a symbol", ErrInvalidName, name)
	}
	return filepath.Join(s.Root, name), nil
}

// Exists reports whether a regular credential file exists for name.
func (s Store) Exists(name string) (bool, error) {
	path, err := s.Path(name)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := checkFile(path, info); err != nil {
		return false, err
	}
	return true, nil
}

// Write atomically stores value as name with mode 0600.
func (s Store) Write(name string, value []byte) (string, error) {
	path, err := s.Path(name)
	if err != nil {
		return "", err
	}
	if len(value) == 0 {
		return "", ErrEmptyValue
	}
	if len(value) > MaxValueSize {
		return "", ErrTooLarge
	}
	if err := s.ensureRoot(); err != nil {
		return "", err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	tmp, err := os.CreateTemp(s.Root, ".tmp-"+name+"-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(value); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}
	if dir, err := os.Open(s.Root); err == nil {
		_ = dir.Sync()
		dir.Close()
	}
	return path, nil
}

// Read returns the value of name. Callers must not print it.
func (s Store) Read(name string) ([]byte, error) {
	path, err := s.Path(name)
	if err != nil {
		return nil, err
	}
	if err := s.checkRoot(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("open credential %s: %w", name, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if err := checkFile(path, info); err != nil {
		return nil, err
	}
	value, err := io.ReadAll(io.LimitReader(f, MaxValueSize+1))
	if err != nil {
		return nil, err
	}
	if len(value) > MaxValueSize {
		return nil, ErrTooLarge
	}
	return value, nil
}

// List returns stored credential names and modification times.
func (s Store) List() ([]Entry, error) {
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.checkRoot(); err != nil {
		return nil, err
	}
	var out []Entry
	for _, entry := range entries {
		if !ValidName(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{Name: entry.Name(), ModTime: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Remove deletes name.
func (s Store) Remove(name string) error {
	path, err := s.Path(name)
	if err != nil {
		return err
	}
	if err := s.checkRoot(); err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	} else if err != nil {
		return err
	}
	return nil
}

func (s Store) ensureRoot() error {
	if err := os.MkdirAll(filepath.Dir(s.Root), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(s.Root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return s.checkRoot()
}

// checkRoot requires the root to be a real directory owned by the current
// user and tightens its mode to 0700.
func (s Store) checkRoot() error {
	info, err := os.Lstat(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s does not exist", ErrNotFound, s.Root)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s is not a real directory", s.Root)
	}
	if !ownedByCurrentUser(info) {
		return fmt.Errorf("%s is not owned by the current user", s.Root)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(s.Root, 0o700); err != nil {
			return fmt.Errorf("secure %s: %w", s.Root, err)
		}
	}
	return nil
}

func checkFile(path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if !ownedByCurrentUser(info) {
		return fmt.Errorf("%s is not owned by the current user", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is readable by other users (mode %04o); run chmod 600", path, info.Mode().Perm())
	}
	return nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}
