package workcell

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

func discoverRepository(cwd string) (string, string) {
	root, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return "", ""
	}
	repository := filepath.Base(root)
	branch, err := gitOutput(cwd, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil && branch != "" {
		return repository, branch
	}
	commit, err := gitOutput(cwd, "rev-parse", "--short", "HEAD")
	if err != nil {
		return repository, ""
	}
	return repository, "detached@" + commit
}

func gitOutput(cwd string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func resolveSession(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	for _, key := range []string{"WORKCELL_SESSION", "ACE_SESSION_ID"} {
		if value := os.Getenv(key); value != "" {
			if err := validateField("session", value); err != nil {
				return "", fmt.Errorf("%s: %w", key, err)
			}
			return value, nil
		}
	}
	username := os.Getenv("USER")
	if current, err := user.Current(); err == nil && current.Username != "" {
		username = current.Username
	}
	if username == "" {
		username = "unknown"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	session := fmt.Sprintf("%s@%s:%d", username, host, os.Getpid())
	if err := validateField("session", session); err != nil {
		return "", fmt.Errorf("generated session: %w", err)
	}
	return session, nil
}
