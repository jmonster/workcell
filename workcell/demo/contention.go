package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type criticalEvent struct {
	WorkerID  int   `json:"worker_id"`
	StartNS   int64 `json:"start_unix_ns"`
	EndNS     int64 `json:"end_unix_ns"`
	Collision bool  `json:"collision"`
}

type contentionResult struct {
	Workers                   int     `json:"workers"`
	Completed                 int     `json:"completed"`
	MaximumSimultaneousOwners int     `json:"maximum_simultaneous_owners"`
	Collisions                int     `json:"collisions"`
	MedianHandoffMS           float64 `json:"median_handoff_ms"`
	P95HandoffMS              float64 `json:"p95_handoff_ms"`
}

type evidenceResult struct {
	SchemaVersion     int              `json:"schema_version"`
	GeneratedAt       string           `json:"generated_at"`
	GeneratedByCommit string           `json:"generated_by_commit"`
	TestsPassed       bool             `json:"tests_passed"`
	Contention        contentionResult `json:"contention"`
}

type workcellCompletion struct {
	Decision string `json:"decision"`
	ExitCode int    `json:"exit_code"`
	Session  string `json:"session"`
}

type workerOutcome struct {
	ID     int
	Output []byte
	Err    error
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "critical" {
		criticalMain(os.Args[2:])
		return
	}
	orchestratorMain(os.Args[1:])
}

func orchestratorMain(args []string) {
	flags := flag.NewFlagSet("contention", flag.ExitOnError)
	workers := flags.Int("workers", 24, "number of simultaneous contenders")
	resource := flags.String("resource", "macos-xcode", "opaque Workcell resource name")
	workcellPath := flags.String("workcell", "./bin/workcell", "path to the Workcell binary")
	holdMS := flags.Int("hold-ms", 30, "critical-section duration per worker")
	evidence := flags.Bool("evidence", false, "emit the full evidence artifact")
	testsPassed := flags.Bool("tests-passed", false, "record that the acceptance suite passed")
	gitSHA := flags.String("git-sha", "", "Git commit used to produce the result")
	if err := flags.Parse(args); err != nil {
		fatalf("parse arguments: %v", err)
	}
	if *workers < 1 {
		fatalf("workers must be positive")
	}
	if *holdMS < 1 {
		fatalf("hold-ms must be positive")
	}
	if strings.TrimSpace(*resource) == "" {
		fatalf("resource cannot be empty")
	}

	absoluteWorkcell, err := filepath.Abs(*workcellPath)
	if err != nil {
		fatalf("resolve Workcell binary: %v", err)
	}
	if info, err := os.Stat(absoluteWorkcell); err != nil || info.IsDir() {
		fatalf("Workcell binary is unavailable at %s", absoluteWorkcell)
	}
	self, err := os.Executable()
	if err != nil {
		fatalf("resolve contention helper: %v", err)
	}
	temporaryDirectory, err := os.MkdirTemp("", "workcell-contention-*")
	if err != nil {
		fatalf("create contention directory: %v", err)
	}
	defer os.RemoveAll(temporaryDirectory)
	stateDirectory := filepath.Join(temporaryDirectory, "state")
	eventDirectory := filepath.Join(temporaryDirectory, "events")
	sentinelPath := filepath.Join(temporaryDirectory, "critical-section")
	if err := os.MkdirAll(eventDirectory, 0o700); err != nil {
		fatalf("create event directory: %v", err)
	}

	start := make(chan struct{})
	outcomes := make(chan workerOutcome, *workers)
	var workersGroup sync.WaitGroup
	for workerID := 1; workerID <= *workers; workerID++ {
		workersGroup.Add(1)
		go func(id int) {
			defer workersGroup.Done()
			<-start
			session := fmt.Sprintf("contender-%02d", id)
			eventPath := filepath.Join(eventDirectory, fmt.Sprintf("worker-%02d.json", id))
			command := exec.Command(
				absoluteWorkcell,
				"run", *resource,
				"--wait",
				"--session", session,
				"--json",
				"--",
				self,
				"critical",
				"--worker-id", fmt.Sprintf("%d", id),
				"--sentinel", sentinelPath,
				"--event", eventPath,
				"--hold-ms", fmt.Sprintf("%d", *holdMS+(id%5)),
			)
			command.Env = environmentWith("WORKCELL_STATE_DIR", stateDirectory)
			output, commandErr := command.CombinedOutput()
			outcomes <- workerOutcome{ID: id, Output: output, Err: commandErr}
		}(workerID)
	}
	close(start)
	workersGroup.Wait()
	close(outcomes)

	completed := 0
	for outcome := range outcomes {
		if outcome.Err != nil {
			fatalf("worker %d failed: %v\n%s", outcome.ID, outcome.Err, outcome.Output)
		}
		var completion workcellCompletion
		if err := json.Unmarshal(outcome.Output, &completion); err != nil {
			fatalf("worker %d returned invalid JSON: %v\n%s", outcome.ID, err, outcome.Output)
		}
		if completion.Decision != "completed" || completion.ExitCode != 0 {
			fatalf("worker %d did not complete successfully: %s", outcome.ID, outcome.Output)
		}
		completed++
	}

	events := make([]criticalEvent, 0, *workers)
	for workerID := 1; workerID <= *workers; workerID++ {
		eventPath := filepath.Join(eventDirectory, fmt.Sprintf("worker-%02d.json", workerID))
		data, err := os.ReadFile(eventPath)
		if err != nil {
			fatalf("read worker %d event: %v", workerID, err)
		}
		var event criticalEvent
		if err := json.Unmarshal(data, &event); err != nil {
			fatalf("decode worker %d event: %v", workerID, err)
		}
		events = append(events, event)
	}

	result := summarize(events, *workers, completed)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if *evidence {
		commit := strings.TrimSpace(*gitSHA)
		if commit == "" {
			commit = currentGitSHA()
		}
		if err := encoder.Encode(evidenceResult{
			SchemaVersion:     1,
			GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
			GeneratedByCommit: commit,
			TestsPassed:       *testsPassed,
			Contention:        result,
		}); err != nil {
			fatalf("write evidence: %v", err)
		}
		return
	}
	if err := encoder.Encode(result); err != nil {
		fatalf("write contention result: %v", err)
	}
}

func criticalMain(args []string) {
	flags := flag.NewFlagSet("critical", flag.ExitOnError)
	workerID := flags.Int("worker-id", 0, "worker identifier")
	sentinel := flags.String("sentinel", "", "atomic sentinel directory")
	eventPath := flags.String("event", "", "event output path")
	holdMS := flags.Int("hold-ms", 30, "critical-section duration")
	if err := flags.Parse(args); err != nil {
		fatalf("parse critical arguments: %v", err)
	}
	if *workerID < 1 || *sentinel == "" || *eventPath == "" || *holdMS < 1 {
		fatalf("invalid critical-section arguments")
	}

	ownedSentinel := false
	collision := false
	if err := os.Mkdir(*sentinel, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			collision = true
		} else {
			fatalf("create atomic sentinel: %v", err)
		}
	} else {
		ownedSentinel = true
	}
	started := time.Now()
	time.Sleep(time.Duration(*holdMS) * time.Millisecond)
	ended := time.Now()
	if ownedSentinel {
		if err := os.Remove(*sentinel); err != nil {
			fatalf("remove atomic sentinel: %v", err)
		}
	}

	data, err := json.Marshal(criticalEvent{
		WorkerID:  *workerID,
		StartNS:   started.UnixNano(),
		EndNS:     ended.UnixNano(),
		Collision: collision,
	})
	if err != nil {
		fatalf("encode critical event: %v", err)
	}
	if err := os.WriteFile(*eventPath, data, 0o600); err != nil {
		fatalf("write critical event: %v", err)
	}
}

func summarize(events []criticalEvent, workers, completed int) contentionResult {
	type boundary struct {
		at    int64
		delta int
	}
	boundaries := make([]boundary, 0, len(events)*2)
	collisions := 0
	for _, event := range events {
		boundaries = append(boundaries, boundary{at: event.StartNS, delta: 1})
		boundaries = append(boundaries, boundary{at: event.EndNS, delta: -1})
		if event.Collision {
			collisions++
		}
	}
	sort.Slice(boundaries, func(i, j int) bool {
		if boundaries[i].at == boundaries[j].at {
			return boundaries[i].delta < boundaries[j].delta
		}
		return boundaries[i].at < boundaries[j].at
	})
	currentOwners := 0
	maximumOwners := 0
	for _, point := range boundaries {
		currentOwners += point.delta
		if currentOwners > maximumOwners {
			maximumOwners = currentOwners
		}
	}

	sort.Slice(events, func(i, j int) bool { return events[i].StartNS < events[j].StartNS })
	handoffs := make([]float64, 0, max(0, len(events)-1))
	for index := 1; index < len(events); index++ {
		gap := float64(events[index].StartNS-events[index-1].EndNS) / float64(time.Millisecond)
		if gap < 0 {
			gap = 0
		}
		handoffs = append(handoffs, gap)
	}
	sort.Float64s(handoffs)
	return contentionResult{
		Workers:                   workers,
		Completed:                 completed,
		MaximumSimultaneousOwners: maximumOwners,
		Collisions:                collisions,
		MedianHandoffMS:           roundHundredth(percentile(handoffs, 0.5)),
		P95HandoffMS:              roundHundredth(percentile(handoffs, 0.95)),
	}
}

func percentile(sorted []float64, percentile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if percentile == 0.5 && len(sorted)%2 == 0 {
		middle := len(sorted) / 2
		return (sorted[middle-1] + sorted[middle]) / 2
	}
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func roundHundredth(value float64) float64 {
	return math.Round(value*100) / 100
}

func currentGitSHA() string {
	command := exec.Command("git", "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func environmentWith(key, value string) []string {
	prefix := key + "="
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, prefix) {
			environment = append(environment, entry)
		}
	}
	return append(environment, prefix+value)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "contention: "+format+"\n", args...)
	os.Exit(1)
}
