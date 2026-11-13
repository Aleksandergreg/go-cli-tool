package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	path string
	name func() string
}

func DefaultStore() (Store, error) {
	root := strings.TrimSpace(os.Getenv("OPSQUEST_HOME"))
	if root == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return Store{}, fmt.Errorf("locate config directory: %w", err)
		}
		root = filepath.Join(configDir, "opsquest")
	}
	return Store{
		path: filepath.Join(root, "profile.json"),
		name: func() string {
			if value := strings.TrimSpace(os.Getenv("OPSQUEST_PLAYER")); value != "" {
				return value
			}
			if value := strings.TrimSpace(os.Getenv("USER")); value != "" {
				return value
			}
			return "operator"
		},
	}, nil
}

func NewStore(path, playerName string) Store {
	return Store{path: path, name: func() string { return playerName }}
}

func (s Store) Path() string { return s.path }

func (s Store) Load() (Profile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return New(s.name()), nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("read profile %s: %w", s.path, err)
	}
	var player Profile
	if err := json.Unmarshal(data, &player); err != nil {
		return Profile{}, fmt.Errorf("decode profile %s: %w; move the file aside to keep a copy, or run 'opsquest reset' to start over", s.path, err)
	}
	if player.Version > currentVersion {
		return Profile{}, fmt.Errorf("profile version %d at %s is newer than this OpsQuest build; upgrade OpsQuest to keep this progress", player.Version, s.path)
	}
	player.Normalize()
	return player, nil
}

func (s Store) Save(player Profile) error {
	// Validate before Normalize so controls and oversized user input cannot be
	// silently converted into a different persisted display name. Load and New
	// have already normalized compatibility/default values at their boundaries.
	if err := ValidateName(player.Name); err != nil {
		return fmt.Errorf("invalid profile name: %w", err)
	}
	// Profile is passed by value, but its maps still alias the caller. Clone
	// them before compatibility normalization so saving never mutates the live
	// session (notably when completed-mission replay hints are omitted on disk).
	player = player.clone()
	player.Normalize()
	data, err := json.MarshalIndent(player, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".profile-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary profile: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary profile: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync profile: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close profile: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace profile: %w", err)
	}
	removeTemporary = false
	return nil
}

func (s Store) Reset() (bool, error) {
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("remove profile: %w", err)
	}
	return true, nil
}
