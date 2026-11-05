package conf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gopkg.in/yaml.v3"
)

// LockInitialization serializes setup and staged-secret cleanup across processes.
func LockInitialization(path string) (func() error, error) {
	f, err := os.OpenFile(path+".init.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return func() error { return errors.Join(syscall.Flock(int(f.Fd()), syscall.LOCK_UN), f.Close()) }, nil
}

// ClearStagedAdminHash removes only the exact staged value, preserving YAML.
func ClearStagedAdminHash(path, expected string) (err error) {
	if expected == "" {
		return nil
	}
	unlock, err := LockInitialization(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	changed, err := removeStagedHash(&doc, expected)
	if err != nil || !changed {
		return err
	}
	encoded, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return WritePrivateAtomic(path, encoded)
}

func removeStagedHash(doc *yaml.Node, expected string) (bool, error) {
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return false, fmt.Errorf("invalid staged configuration")
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "Administrator" {
			continue
		}
		admin := root.Content[i+1]
		if admin.Kind != yaml.MappingNode {
			return false, fmt.Errorf("invalid administrator configuration")
		}
		for j := 0; j+1 < len(admin.Content); j += 2 {
			if admin.Content[j].Value != "PasswordHash" {
				continue
			}
			if admin.Content[j+1].Value != expected {
				return false, fmt.Errorf("staged administrator hash changed")
			}
			admin.Content = append(admin.Content[:j], admin.Content[j+2:]...)
			return true, nil
		}
		return false, nil
	}
	return false, nil
}

// WritePrivateAtomic persists a private file and its directory; cleanup and
// close errors are reported alongside, never instead of, the primary error.
func WritePrivateAtomic(path string, data []byte) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".ppanel-private-*")
	if err != nil {
		return err
	}
	name := file.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, file.Close())
		}
		if removeErr := os.Remove(name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if err = file.Chmod(0600); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	err = file.Close()
	closed = true
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, dir.Close()) }()
	return dir.Sync()
}
