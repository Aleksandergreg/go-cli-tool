package kubelab

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/aleksandergregersen/opsquest/internal/ownerproc"
)

// A durable private record precedes all engine operations. Match the local
// host and endpoint again before attempting orphan cleanup.
type owner struct {
	PID      int
	Hostname string
	Endpoint string
}

var attemptName = regexp.MustCompile("^opsquest-[a-f0-9]{32}$")

func newAttemptDir(ctx context.Context, name, endpoint string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(cache, "opsquest", "kind")
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	current := owner{os.Getpid(), hostname, endpoint}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if err = recoverAttempts(ctx, root, current); err != nil {
		return "", err
	}
	dir := filepath.Join(root, name)
	if err = os.Mkdir(dir, 0700); err != nil {
		return "", err
	}
	data, err := json.Marshal(current)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "owner.json"), data, 0600)
	}
	if err != nil {
		return "", errors.Join(err, os.RemoveAll(dir))
	}
	return dir, nil
}

func recoverAttempts(ctx context.Context, root string, current owner) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() || !attemptName.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "owner.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var recorded owner
		if json.Unmarshal(data, &recorded) != nil || recorded.Hostname != current.Hostname || recorded.Endpoint != current.Endpoint || recorded.PID <= 0 || ownerproc.Alive(recorded.PID) {
			continue
		}
		e := environment{name: entry.Name(), dir: dir, host: current.Endpoint}
		if err = e.Close(); err != nil {
			return err
		}
	}
	return nil
}
