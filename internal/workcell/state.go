package workcell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

type statePaths struct {
	logsDir         string
	lockPath        string
	metadataPath    string
	historyPath     string
	historyLockPath string
	queueDir        string
	queueLockPath   string
}

func pathsForResource(resource string) (statePaths, error) {
	root, err := stateRoot()
	if err != nil {
		return statePaths{}, err
	}
	locks := filepath.Join(root, "locks")
	metadata := filepath.Join(root, "metadata")
	logs := filepath.Join(root, "logs")
	queues := filepath.Join(root, "queues")
	history := filepath.Join(root, "history")
	key := resourceKey(resource)
	queueDir := filepath.Join(queues, key)
	for _, dir := range []string{root, locks, metadata, logs, queues, queueDir, history} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return statePaths{}, fmt.Errorf("create state directory %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return statePaths{}, fmt.Errorf("secure state directory %s: %w", dir, err)
		}
	}
	return statePaths{
		logsDir:         logs,
		lockPath:        filepath.Join(locks, key+".lock"),
		metadataPath:    filepath.Join(metadata, key+".json"),
		historyPath:     filepath.Join(history, key+".json"),
		historyLockPath: filepath.Join(history, key+".lock"),
		queueDir:        queueDir,
		queueLockPath:   filepath.Join(queueDir, ".lock"),
	}, nil
}

func stateRoot() (string, error) {
	if dir := os.Getenv("WORKCELL_STATE_DIR"); dir != "" {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("resolve WORKCELL_STATE_DIR: %w", err)
		}
		return absolute, nil
	}
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		absolute, err := filepath.Abs(filepath.Join(dir, "workcell"))
		if err != nil {
			return "", fmt.Errorf("resolve XDG_STATE_HOME: %w", err)
		}
		return absolute, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "workcell"), nil
}

func resourceKey(resource string) string {
	sum := sha256.Sum256([]byte(resource))
	return hex.EncodeToString(sum[:])
}
