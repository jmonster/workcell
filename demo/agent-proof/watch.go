package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type workcellStatus struct {
	Decision   string `json:"decision"`
	QueueAhead int    `json:"queue_ahead"`
	Owner      *struct {
		Session string `json:"session"`
	} `json:"owner,omitempty"`
}

type proofEventSink interface {
	deployed(deployEvent)
	testResult(attemptResult)
}

func watchProofEvents(ctx context.Context, sharedDir string, sink proofEventSink) {
	seenDeploys := make(map[string]bool)
	seenResults := make(map[string]bool)
	scan := func() {
		type visualEvent struct {
			at      time.Time
			path    string
			deploy  *deployEvent
			attempt *attemptResult
		}
		pending := make([]visualEvent, 0)
		deployPaths, _ := filepath.Glob(filepath.Join(sharedDir, "events", "deploy-*.json"))
		for _, filePath := range sortedPaths(deployPaths) {
			if seenDeploys[filePath] {
				continue
			}
			var event deployEvent
			if readJSON(filePath, &event) == nil {
				copyValue := event
				pending = append(pending, visualEvent{at: event.DeployedAt, path: filePath, deploy: &copyValue})
			}
		}
		resultPaths, _ := filepath.Glob(filepath.Join(sharedDir, "results", "*.json"))
		for _, filePath := range sortedPaths(resultPaths) {
			if seenResults[filePath] {
				continue
			}
			var result attemptResult
			if readJSON(filePath, &result) == nil {
				copyValue := result
				pending = append(pending, visualEvent{at: result.FinishedAt, path: filePath, attempt: &copyValue})
			}
		}
		sort.Slice(pending, func(i, j int) bool {
			if pending[i].at.Equal(pending[j].at) {
				return pending[i].path < pending[j].path
			}
			return pending[i].at.Before(pending[j].at)
		})
		for _, event := range pending {
			if event.deploy != nil {
				seenDeploys[event.path] = true
				sink.deployed(*event.deploy)
				continue
			}
			seenResults[event.path] = true
			sink.testResult(*event.attempt)
		}
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			scan()
		case <-ctx.Done():
			scan()
			return
		}
	}
}

func waitForCandidatesLive(ctx context.Context, sharedDir, subdirectory string, ids []string, timeout time.Duration, onReady func(string)) error {
	seen := make(map[string]bool, len(ids))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			if _, err := os.Stat(filepath.Join(sharedDir, subdirectory, id)); err == nil {
				seen[id] = true
				onReady(id)
			}
		}
		if len(seen) == len(ids) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return waitForCandidates(ctx, sharedDir, subdirectory, ids, 0)
}

func waitForWorkcellQueue(ctx context.Context, workcellPath, stateDir, resource string, wanted int, timeout time.Duration) (string, int, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		command := exec.CommandContext(ctx, workcellPath, "status", resource, "--json")
		command.Env = environmentSet(os.Environ(), "WORKCELL_STATE_DIR", stateDir)
		output, err := command.Output()
		if err == nil {
			var status workcellStatus
			if decodeErr := json.Unmarshal(output, &status); decodeErr == nil {
				owner := ""
				if status.Owner != nil {
					owner = status.Owner.Session
				}
				if status.Decision == "owned" && status.QueueAhead >= wanted {
					return owner, status.QueueAhead, nil
				}
			}
		} else {
			lastErr = err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("queue did not reach %d waiter(s)", wanted)
	}
	return "", 0, lastErr
}
