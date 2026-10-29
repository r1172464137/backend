package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gopkg.in/yaml.v3"
)

// LockInitialization serializes setup and staged-secret cleanup across
// processes sharing the same config path. The lock file contains no secrets.
func LockInitialization(path string) (func(), error) {
	f, err := os.OpenFile(path+".init.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

// ClearStagedAdminHash removes only the exact staged value, preserving all
// other YAML keys. A crash at any point leaves either the old or cleaned file.
func ClearStagedAdminHash(path, expected string) error {
	if expected == "" {
		return nil
	}
	unlock, err := LockInitialization(path)
	if err != nil {
		return err
	}
	defer unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("invalid staged configuration")
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "Administrator" {
			continue
		}
		admin := root.Content[i+1]
		if admin.Kind != yaml.MappingNode {
			return fmt.Errorf("invalid administrator configuration")
		}
		for j := 0; j+1 < len(admin.Content); j += 2 {
			if admin.Content[j].Value != "PasswordHash" {
				continue
			}
			if admin.Content[j+1].Value != expected {
				return fmt.Errorf("staged administrator hash changed")
			}
			admin.Content = append(admin.Content[:j], admin.Content[j+2:]...)
			encoded, err := yaml.Marshal(&doc)
			if err != nil {
				return err
			}
			f, err := os.CreateTemp(filepath.Dir(path), ".ppanel-clean-*")
			if err != nil {
				return err
			}
			name := f.Name()
			defer os.Remove(name)
			if err = f.Chmod(0600); err != nil {
				f.Close()
				return err
			}
			if _, err = f.Write(encoded); err != nil {
				f.Close()
				return err
			}
			if err = f.Sync(); err != nil {
				f.Close()
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
			if err = os.Rename(name, path); err != nil {
				return err
			}
			dir, err := os.Open(filepath.Dir(path))
			if err != nil {
				return err
			}
			defer dir.Close()
			return dir.Sync()
		}
		return nil
	}
	return nil
}
