package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	regressionCandidateID = "wait-regression"
	baselineCandidateID   = "baseline"
	replicaCandidateID    = "baseline-replica"
	proofResource         = "proof:shared-integration-binary"
	acceptanceTest        = "^TestWaitAcquiresAfterOwnerExits$"
)

var candidateIDs = []string{regressionCandidateID, baselineCandidateID, replicaCandidateID}

type proofConfig struct {
	Mode         string `json:"mode"`
	CandidateID  string `json:"candidate_id"`
	Worktree     string `json:"worktree"`
	SharedDir    string `json:"shared_dir"`
	OracleRepo   string `json:"oracle_repo"`
	HarnessPath  string `json:"harness_path"`
	WorkcellPath string `json:"workcell_path"`
	Resource     string `json:"resource"`
}

type attemptResult struct {
	CandidateID  string    `json:"candidate_id"`
	Mode         string    `json:"mode"`
	ExpectedSHA  string    `json:"expected_sha256"`
	ObservedSHA  string    `json:"observed_sha256"`
	StartedAt    time.Time `json:"started_at"`
	DeployedAt   time.Time `json:"deployed_at"`
	TestStarted  time.Time `json:"test_started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	TestExitCode int       `json:"test_exit_code"`
}

type deployEvent struct {
	CandidateID string    `json:"candidate_id"`
	SHA         string    `json:"sha256"`
	DeployedAt  time.Time `json:"deployed_at"`
}

type workcellCompletion struct {
	SchemaVersion int     `json:"schema_version"`
	Resource      string  `json:"resource"`
	Decision      string  `json:"decision"`
	ReservationID string  `json:"reservation_id"`
	Session       string  `json:"session"`
	ExitCode      int     `json:"exit_code"`
	WaitedSeconds float64 `json:"waited_seconds"`
	RanSeconds    float64 `json:"ran_seconds"`
	LogPath       string  `json:"log_path"`
	EvidencePath  string  `json:"evidence_path,omitempty"`
}

type agentOutcome struct {
	CandidateID string `json:"candidate_id"`
	ExitCode    int    `json:"exit_code"`
	EventsPath  string `json:"events_path"`
	StderrPath  string `json:"stderr_path"`
	FinalPath   string `json:"final_message_path"`
}

type oracleResult struct {
	CandidateID string `json:"candidate_id"`
	Passed      bool   `json:"passed"`
	ExitCode    int    `json:"exit_code"`
	LogPath     string `json:"log_path"`
	DiffPath    string `json:"diff_path"`
}

type armResult struct {
	Mode                       string               `json:"mode"`
	Agents                     int                  `json:"agents"`
	AgentsCompleted            int                  `json:"agents_completed"`
	Attempts                   int                  `json:"attempts"`
	GreenAttempts              int                  `json:"green_attempts"`
	FalseGreenAttempts         int                  `json:"false_green_attempts"`
	MaximumConcurrentCriticals int                  `json:"maximum_concurrent_critical_sections"`
	FinalCandidatesCorrect     int                  `json:"final_candidates_correct"`
	TimeToAllCorrectSeconds    *float64             `json:"time_to_all_correct_seconds,omitempty"`
	ElapsedSeconds             float64              `json:"elapsed_seconds"`
	AgentOutcomes              []agentOutcome       `json:"agent_outcomes"`
	AttemptResults             []attemptResult      `json:"attempt_results"`
	OracleResults              []oracleResult       `json:"oracle_results"`
	WorkcellRuns               []workcellCompletion `json:"workcell_runs,omitempty"`
}

type proofResult struct {
	SchemaVersion int         `json:"schema_version"`
	GeneratedAt   time.Time   `json:"generated_at"`
	Repository    string      `json:"repository"`
	Commit        string      `json:"commit"`
	Model         string      `json:"model"`
	Reasoning     string      `json:"reasoning_effort"`
	ArtifactDir   string      `json:"artifact_dir"`
	Passed        bool        `json:"passed"`
	Arms          []armResult `json:"arms"`
}

type candidateWorkspace struct {
	ID       string
	Worktree string
}

type runningAgent struct {
	ID         string
	EventsPath string
	StderrPath string
	FinalPath  string
	Done       <-chan agentOutcome
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "verify":
			os.Exit(verifyMain(os.Args[2:]))
		case "critical":
			os.Exit(criticalMain(os.Args[2:]))
		}
	}
	os.Exit(runMain(os.Args[1:]))
}

func runMain(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer cancel()

	flags := flag.NewFlagSet("agent-proof", flag.ContinueOnError)
	mode := flags.String("mode", "compare", "without, workcell, or compare")
	model := flags.String("model", "gpt-5.6-luna", "Codex model for every agent")
	reasoning := flags.String("reasoning", "medium", "Codex reasoning effort")
	codexPath := flags.String("codex", "codex", "Codex CLI path")
	workcellPath := flags.String("workcell", "./bin/workcell", "canonical Workcell binary")
	artifactDir := flags.String("output", "", "artifact directory; defaults to a retained temporary directory")
	timeout := flags.Duration("timeout", 8*time.Minute, "maximum duration per arm")
	plain := flags.Bool("plain", false, "disable the live terminal dashboard")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *mode != "without" && *mode != "workcell" && *mode != "compare" {
		fmt.Fprintln(os.Stderr, "mode must be without, workcell, or compare")
		return 2
	}

	repository, err := repositoryRoot()
	if err != nil {
		return fail(err)
	}
	commit, err := commandOutput(repository, "git", "rev-parse", "HEAD")
	if err != nil {
		return fail(err)
	}
	codexExecutable, err := exec.LookPath(*codexPath)
	if err != nil {
		return fail(fmt.Errorf("find Codex CLI: %w", err))
	}
	workcellExecutable, err := filepath.Abs(*workcellPath)
	if err != nil {
		return fail(fmt.Errorf("resolve Workcell binary: %w", err))
	}
	if info, statErr := os.Stat(workcellExecutable); statErr != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return fail(fmt.Errorf("Workcell binary is not executable: %s", workcellExecutable))
	}
	outputRoot := *artifactDir
	if outputRoot == "" {
		outputRoot, err = os.MkdirTemp("", "workcell-agent-proof-*")
	} else {
		outputRoot, err = filepath.Abs(outputRoot)
		if err == nil {
			err = os.MkdirAll(outputRoot, 0o700)
		}
	}
	if err != nil {
		return fail(fmt.Errorf("create artifact directory: %w", err))
	}

	self, err := os.Executable()
	if err != nil {
		return fail(fmt.Errorf("resolve proof harness: %w", err))
	}
	runModes := []string{*mode}
	if *mode == "compare" {
		runModes = []string{"without", "workcell"}
	}
	result := proofResult{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC(),
		Repository:    repository,
		Commit:        strings.TrimSpace(commit),
		Model:         *model,
		Reasoning:     *reasoning,
		ArtifactDir:   outputRoot,
	}
	dashboard := newDashboard(ctx, cancel, outputRoot, !*plain, len(runModes)+1)
	dashboard.start()
	defer dashboard.close()
	for _, armMode := range runModes {
		arm, runErr := runArm(ctx, armMode, repository, self, codexExecutable, workcellExecutable, *model, *reasoning, outputRoot, *timeout, dashboard)
		if runErr != nil {
			if ctx.Err() != nil {
				return 130
			}
			dashboard.note("HARNESS ERROR: " + runErr.Error())
			fmt.Fprintf(os.Stderr, "%s arm: %v\n", armMode, runErr)
			_ = writeJSON(filepath.Join(outputRoot, "result.json"), result)
			return 1
		}
		result.Arms = append(result.Arms, arm)
		dashboard.waitForContinue(armTitle(armMode) + " COMPLETE")
		if ctx.Err() != nil {
			return 130
		}
	}
	result.Passed = proofPassed(result.Arms)
	if err := writeJSON(filepath.Join(outputRoot, "result.json"), result); err != nil {
		return fail(err)
	}
	dashboard.showFinal(result)
	dashboard.waitForContinue("FINAL SCORECARD")
	if ctx.Err() != nil {
		return 130
	}
	if !result.Passed {
		return 1
	}
	return 0
}

func runArm(parent context.Context, mode, repository, self, codexPath, workcellPath, model, reasoning, outputRoot string, timeout time.Duration, dashboard *dashboard) (armResult, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return armResult{}, err
	}
	started := time.Now()
	dashboard.startArm(mode, model, reasoning)
	armRoot := filepath.Join(outputRoot, mode)
	sharedDir := filepath.Join(armRoot, "shared")
	for _, directory := range []string{
		armRoot,
		sharedDir,
		filepath.Join(sharedDir, "ready"),
		filepath.Join(sharedDir, "gates"),
		filepath.Join(sharedDir, "deployed"),
		filepath.Join(sharedDir, "events"),
		filepath.Join(sharedDir, "results"),
		filepath.Join(sharedDir, "workcell-results"),
		filepath.Join(sharedDir, "slot"),
		filepath.Join(sharedDir, "cache"),
		filepath.Join(armRoot, "agents"),
		filepath.Join(armRoot, "oracle"),
		filepath.Join(armRoot, "diffs"),
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return armResult{}, err
		}
	}

	dashboard.stage("Creating three isolated candidate worktrees")
	seed := filepath.Join(armRoot, "seed")
	if output, err := runCommand("", "git", "clone", "--quiet", "--no-local", repository, seed); err != nil {
		return armResult{}, fmt.Errorf("clone proof repository: %w\n%s", err, output)
	}
	workspaces, err := createCandidateWorkspaces(mode, seed, sharedDir, self, workcellPath)
	if err != nil {
		return armResult{}, err
	}

	dashboard.stage("Starting three real Luna agents")
	running := make([]runningAgent, 0, len(workspaces))
	for _, workspace := range workspaces {
		agent, err := startAgent(ctx, workspace, sharedDir, codexPath, model, reasoning, armRoot)
		if err != nil {
			return armResult{}, err
		}
		running = append(running, agent)
		dashboard.agentStarting(workspace.ID)
	}
	watchCtx, cancelWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		watchProofEvents(watchCtx, sharedDir, dashboard)
		close(watchDone)
	}()
	if err := waitForCandidatesLive(ctx, sharedDir, "ready", candidateIDs, 2*time.Minute, dashboard.agentReady); err != nil {
		cancelWatch()
		<-watchDone
		return armResult{}, err
	}
	dashboard.stage("All candidates built. Releasing the shared deploy-and-test race")
	dashboard.agentReleased(regressionCandidateID, false)
	if err := touch(filepath.Join(sharedDir, "gates", regressionCandidateID)); err != nil {
		cancelWatch()
		<-watchDone
		return armResult{}, err
	}
	if err := waitForCandidates(ctx, sharedDir, "deployed", []string{regressionCandidateID}, 30*time.Second); err != nil {
		cancelWatch()
		<-watchDone
		return armResult{}, err
	}
	for _, id := range []string{baselineCandidateID, replicaCandidateID} {
		dashboard.agentReleased(id, mode == "workcell")
		if err := touch(filepath.Join(sharedDir, "gates", id)); err != nil {
			cancelWatch()
			<-watchDone
			return armResult{}, err
		}
	}
	if mode == "workcell" {
		if owner, queued, statusErr := waitForWorkcellQueue(ctx, workcellPath, filepath.Join(sharedDir, "workcell-state"), proofResource, 2, 3*time.Second); statusErr == nil {
			dashboard.workcellQueue(owner, queued)
		}
	}

	outcomes := make([]agentOutcome, 0, len(running))
	for _, agent := range running {
		select {
		case outcome := <-agent.Done:
			outcomes = append(outcomes, outcome)
			dashboard.agentFinished(outcome.CandidateID, outcome.ExitCode)
		case <-ctx.Done():
			cancelWatch()
			<-watchDone
			return armResult{}, ctx.Err()
		}
	}
	cancelWatch()
	<-watchDone
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].CandidateID < outcomes[j].CandidateID })

	attempts, err := readAttemptResults(filepath.Join(sharedDir, "results"))
	if err != nil {
		return armResult{}, err
	}
	dashboard.stage("Checking each final source tree")
	oracles, err := runOracles(ctx, workspaces, seed, armRoot)
	if err != nil {
		return armResult{}, err
	}
	workcellRuns, err := readWorkcellResults(filepath.Join(sharedDir, "workcell-results"))
	if err != nil {
		return armResult{}, err
	}
	arm := summarizeArm(mode, outcomes, attempts, oracles, workcellRuns, time.Since(started))
	if err := writeJSON(filepath.Join(armRoot, "summary.json"), arm); err != nil {
		return armResult{}, err
	}
	dashboard.completeArm(arm)
	return arm, nil
}

func createCandidateWorkspaces(mode, seed, sharedDir, self, workcellPath string) ([]candidateWorkspace, error) {
	worktreeRoot := filepath.Join(filepath.Dir(seed), "worktrees")
	if err := os.MkdirAll(worktreeRoot, 0o700); err != nil {
		return nil, err
	}
	workspaces := make([]candidateWorkspace, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		worktree := filepath.Join(worktreeRoot, id)
		branch := "agent-proof-" + id
		if output, err := runCommand(seed, "git", "worktree", "add", "--quiet", "-b", branch, worktree, "HEAD"); err != nil {
			return nil, fmt.Errorf("create %s worktree: %w\n%s", id, err, output)
		}
		proofDir := filepath.Join(worktree, ".proof")
		if err := os.MkdirAll(filepath.Join(proofDir, "tmp"), 0o700); err != nil {
			return nil, err
		}
		harnessPath := filepath.Join(proofDir, "agent-proof")
		if err := copyExecutable(self, harnessPath); err != nil {
			return nil, err
		}
		configPath := filepath.Join(proofDir, "config.json")
		config := proofConfig{
			Mode:         mode,
			CandidateID:  id,
			Worktree:     worktree,
			SharedDir:    sharedDir,
			OracleRepo:   seed,
			HarnessPath:  harnessPath,
			WorkcellPath: workcellPath,
			Resource:     proofResource,
		}
		if err := writeJSON(configPath, config); err != nil {
			return nil, err
		}
		verifier := "#!/bin/sh\nset -eu\nexec \"$PWD/.proof/agent-proof\" verify --config \"$PWD/.proof/config.json\"\n"
		verifierPath := filepath.Join(worktree, "verify-candidate")
		if err := os.WriteFile(verifierPath, []byte(verifier), 0o700); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, candidateWorkspace{ID: id, Worktree: worktree})
	}
	badWorktree := filepath.Join(worktreeRoot, regressionCandidateID)
	regressionPath := filepath.Join("workcell", "internal", "workcell", "cli.go")
	if err := injectWaitRegression(filepath.Join(badWorktree, regressionPath)); err != nil {
		return nil, err
	}
	for _, pair := range [][]string{{"user.name", "Workcell Agent Proof"}, {"user.email", "agent-proof@workcell.invalid"}} {
		if output, err := runCommand(badWorktree, "git", "config", pair[0], pair[1]); err != nil {
			return nil, fmt.Errorf("configure candidate commit: %w\n%s", err, output)
		}
	}
	if output, err := runCommand(badWorktree, "git", "add", regressionPath); err != nil {
		return nil, fmt.Errorf("stage candidate regression: %w\n%s", err, output)
	}
	if output, err := runCommand(badWorktree, "git", "commit", "--quiet", "-m", "candidate: refactor wait option handling"); err != nil {
		return nil, fmt.Errorf("commit candidate regression: %w\n%s", err, output)
	}
	return workspaces, nil
}

func injectWaitRegression(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	old := "\t\tcase arg == \"--wait\":\n\t\t\topts.Wait = true"
	newValue := "\t\tcase arg == \"--wait\":\n\t\t\topts.Wait = false"
	if strings.Count(string(data), old) != 1 {
		return errors.New("cannot inject wait regression: expected source pattern not found exactly once")
	}
	return os.WriteFile(filePath, []byte(strings.Replace(string(data), old, newValue, 1)), 0o644)
}

func startAgent(ctx context.Context, workspace candidateWorkspace, sharedDir, codexPath, model, reasoning, armRoot string) (runningAgent, error) {
	agentDir := filepath.Join(armRoot, "agents")
	eventsPath := filepath.Join(agentDir, workspace.ID+".jsonl")
	stderrPath := filepath.Join(agentDir, workspace.ID+".stderr.log")
	finalPath := filepath.Join(agentDir, workspace.ID+".final.txt")
	eventsFile, err := os.Create(eventsPath)
	if err != nil {
		return runningAgent{}, err
	}
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		eventsFile.Close()
		return runningAgent{}, err
	}
	prompt := "Run ./verify-candidate. If it passes, report candidate validated and stop. If it fails, fix the source without changing tests, verify-candidate, or .proof, then rerun until it passes."
	command := exec.CommandContext(ctx, codexPath,
		"-a", "never",
		"exec",
		"--ephemeral",
		"--json",
		"--ignore-user-config",
		"-m", model,
		"-c", fmt.Sprintf("model_reasoning_effort=%q", reasoning),
		"-s", "workspace-write",
		"-C", workspace.Worktree,
		"--add-dir", sharedDir,
		"-o", finalPath,
		prompt,
	)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 2 * time.Second
	command.Env = proofEnvironment(os.Environ(), workspace.Worktree, sharedDir, workspace.ID)
	command.Stdout = eventsFile
	command.Stderr = stderrFile
	if err := command.Start(); err != nil {
		eventsFile.Close()
		stderrFile.Close()
		return runningAgent{}, fmt.Errorf("start %s agent: %w", workspace.ID, err)
	}
	done := make(chan agentOutcome, 1)
	go func() {
		err := command.Wait()
		_ = eventsFile.Close()
		_ = stderrFile.Close()
		done <- agentOutcome{
			CandidateID: workspace.ID,
			ExitCode:    exitCode(err),
			EventsPath:  eventsPath,
			StderrPath:  stderrPath,
			FinalPath:   finalPath,
		}
	}()
	return runningAgent{ID: workspace.ID, EventsPath: eventsPath, StderrPath: stderrPath, FinalPath: finalPath, Done: done}, nil
}

func verifyMain(args []string) int {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	configPath := flags.String("config", "", "proof config")
	if err := flags.Parse(args); err != nil || *configPath == "" {
		return 2
	}
	config, err := readConfig(*configPath)
	if err != nil {
		return fail(err)
	}
	candidateBinary := filepath.Join(config.Worktree, ".proof", "candidate-workcell")
	build := exec.Command("go", "build", "-buildvcs=false", "-ldflags", "-buildid="+config.CandidateID, "-o", candidateBinary, "./cmd/workcell")
	build.Dir = filepath.Join(config.Worktree, "workcell")
	build.Env = proofEnvironment(os.Environ(), config.Worktree, config.SharedDir, config.CandidateID)
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return exitCode(err)
	}
	expectedSHA, err := fileSHA256(candidateBinary)
	if err != nil {
		return fail(err)
	}
	if err := touch(filepath.Join(config.SharedDir, "ready", config.CandidateID)); err != nil {
		return fail(err)
	}
	if err := waitForCandidates(context.Background(), config.SharedDir, "gates", []string{config.CandidateID}, 5*time.Minute); err != nil {
		return fail(err)
	}
	criticalArgv := []string{config.HarnessPath, "critical", "--config", *configPath, "--expected-sha", expectedSHA}
	if config.Mode == "without" {
		command := exec.Command(criticalArgv[0], criticalArgv[1:]...)
		command.Dir = config.Worktree
		command.Env = proofEnvironment(os.Environ(), config.Worktree, config.SharedDir, config.CandidateID)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		return exitCode(command.Run())
	}
	resultPath := filepath.Join(config.SharedDir, "workcell-results", fmt.Sprintf("%s-%d.json", config.CandidateID, time.Now().UnixNano()))
	workcellArgv := []string{
		"run", config.Resource,
		"--wait",
		"--session", config.CandidateID,
		"--json",
		"--",
	}
	workcellArgv = append(workcellArgv, criticalArgv...)
	command := exec.Command(config.WorkcellPath, workcellArgv...)
	command.Dir = config.Worktree
	command.Env = environmentSet(proofEnvironment(os.Environ(), config.Worktree, config.SharedDir, config.CandidateID), "WORKCELL_STATE_DIR", filepath.Join(config.SharedDir, "workcell-state"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}
	if writeErr := os.WriteFile(resultPath, stdout.Bytes(), 0o600); writeErr != nil {
		return fail(writeErr)
	}
	var completion workcellCompletion
	if err := json.Unmarshal(stdout.Bytes(), &completion); err != nil {
		fmt.Fprintf(os.Stderr, "invalid Workcell result: %v\n%s", err, stdout.String())
		return exitCode(runErr)
	}
	if completion.LogPath != "" {
		if logData, err := os.ReadFile(completion.LogPath); err == nil {
			_, _ = os.Stdout.Write(logData)
		}
	}
	return exitCode(runErr)
}

func criticalMain(args []string) int {
	flags := flag.NewFlagSet("critical", flag.ContinueOnError)
	configPath := flags.String("config", "", "proof config")
	expectedSHA := flags.String("expected-sha", "", "candidate binary SHA-256")
	if err := flags.Parse(args); err != nil || *configPath == "" || *expectedSHA == "" {
		return 2
	}
	config, err := readConfig(*configPath)
	if err != nil {
		return fail(err)
	}
	started := time.Now().UTC()
	candidateBinary := filepath.Join(config.Worktree, ".proof", "candidate-workcell")
	slotPath := filepath.Join(config.SharedDir, "slot", "workcell")
	if err := atomicDeploy(candidateBinary, slotPath); err != nil {
		return fail(err)
	}
	deployed := time.Now().UTC()
	deployEventPath := filepath.Join(config.SharedDir, "events", fmt.Sprintf("deploy-%d-%s.json", time.Now().UnixNano(), config.CandidateID))
	if err := writeJSON(deployEventPath, deployEvent{CandidateID: config.CandidateID, SHA: *expectedSHA, DeployedAt: deployed}); err != nil {
		return fail(err)
	}
	if err := touch(filepath.Join(config.SharedDir, "deployed", config.CandidateID)); err != nil {
		return fail(err)
	}
	for range 25 {
		health := exec.Command(slotPath, "help")
		health.Stdout = io.Discard
		health.Stderr = io.Discard
		if err := health.Run(); err != nil {
			return fail(fmt.Errorf("staging health probe: %w", err))
		}
		time.Sleep(40 * time.Millisecond)
	}
	observedSHA, err := fileSHA256(slotPath)
	if err != nil {
		return fail(err)
	}
	testStarted := time.Now().UTC()
	test := exec.Command("go", "test", "-count=1", "-run", acceptanceTest, "./integration")
	test.Dir = filepath.Join(config.OracleRepo, "workcell")
	test.Env = environmentSet(proofEnvironment(os.Environ(), config.Worktree, config.SharedDir, config.CandidateID), "WORKCELL_INTEGRATION_BINARY", slotPath)
	test.Stdout = os.Stdout
	test.Stderr = os.Stderr
	testErr := test.Run()
	finished := time.Now().UTC()
	result := attemptResult{
		CandidateID:  config.CandidateID,
		Mode:         config.Mode,
		ExpectedSHA:  *expectedSHA,
		ObservedSHA:  observedSHA,
		StartedAt:    started,
		DeployedAt:   deployed,
		TestStarted:  testStarted,
		FinishedAt:   finished,
		TestExitCode: exitCode(testErr),
	}
	resultPath := filepath.Join(config.SharedDir, "results", fmt.Sprintf("%s-%d.json", config.CandidateID, time.Now().UnixNano()))
	if err := writeJSON(resultPath, result); err != nil {
		return fail(err)
	}
	return result.TestExitCode
}

func runOracles(ctx context.Context, workspaces []candidateWorkspace, oracleRepo, armRoot string) ([]oracleResult, error) {
	results := make([]oracleResult, 0, len(workspaces))
	for _, workspace := range workspaces {
		binaryPath := filepath.Join(armRoot, "oracle", workspace.ID+"-workcell")
		logPath := filepath.Join(armRoot, "oracle", workspace.ID+".log")
		diffPath := filepath.Join(armRoot, "diffs", workspace.ID+".patch")
		var log bytes.Buffer
		build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", binaryPath, "./cmd/workcell")
		build.Dir = filepath.Join(workspace.Worktree, "workcell")
		build.Env = proofEnvironment(os.Environ(), workspace.Worktree, filepath.Join(armRoot, "shared"), workspace.ID+"-oracle")
		build.Stdout = &log
		build.Stderr = &log
		buildErr := build.Run()
		testErr := buildErr
		if buildErr == nil {
			test := exec.CommandContext(ctx, "go", "test", "-count=1", "-run", acceptanceTest, "./integration")
			test.Dir = filepath.Join(oracleRepo, "workcell")
			test.Env = environmentSet(proofEnvironment(os.Environ(), workspace.Worktree, filepath.Join(armRoot, "shared"), workspace.ID+"-oracle"), "WORKCELL_INTEGRATION_BINARY", binaryPath)
			test.Stdout = &log
			test.Stderr = &log
			testErr = test.Run()
		}
		if err := os.WriteFile(logPath, log.Bytes(), 0o600); err != nil {
			return nil, err
		}
		diffOutput, _ := runCommand(workspace.Worktree, "git", "diff", "--no-ext-diff", "--", "workcell/internal/workcell")
		if err := os.WriteFile(diffPath, []byte(diffOutput), 0o600); err != nil {
			return nil, err
		}
		results = append(results, oracleResult{
			CandidateID: workspace.ID,
			Passed:      testErr == nil,
			ExitCode:    exitCode(testErr),
			LogPath:     logPath,
			DiffPath:    diffPath,
		})
	}
	return results, nil
}

func summarizeArm(mode string, outcomes []agentOutcome, attempts []attemptResult, oracles []oracleResult, workcellRuns []workcellCompletion, elapsed time.Duration) armResult {
	greenByCandidate := make(map[string]bool)
	greenAttempts := 0
	falseGreens := 0
	for _, attempt := range attempts {
		if attempt.TestExitCode == 0 {
			greenAttempts++
			greenByCandidate[attempt.CandidateID] = true
			if attempt.ExpectedSHA != attempt.ObservedSHA {
				falseGreens++
			}
		}
	}
	agentsCompleted := 0
	for _, outcome := range outcomes {
		if outcome.ExitCode == 0 && greenByCandidate[outcome.CandidateID] {
			agentsCompleted++
		}
	}
	correct := 0
	for _, oracle := range oracles {
		if oracle.Passed {
			correct++
		}
	}
	var timeToAllCorrect *float64
	if correct == len(candidateIDs) {
		seconds := elapsed.Seconds()
		timeToAllCorrect = &seconds
	}
	return armResult{
		Mode:                       mode,
		Agents:                     len(candidateIDs),
		AgentsCompleted:            agentsCompleted,
		Attempts:                   len(attempts),
		GreenAttempts:              greenAttempts,
		FalseGreenAttempts:         falseGreens,
		MaximumConcurrentCriticals: maximumConcurrent(attempts),
		FinalCandidatesCorrect:     correct,
		TimeToAllCorrectSeconds:    timeToAllCorrect,
		ElapsedSeconds:             elapsed.Seconds(),
		AgentOutcomes:              outcomes,
		AttemptResults:             attempts,
		OracleResults:              oracles,
		WorkcellRuns:               workcellRuns,
	}
}

func maximumConcurrent(attempts []attemptResult) int {
	type boundary struct {
		at    time.Time
		delta int
	}
	boundaries := make([]boundary, 0, len(attempts)*2)
	for _, attempt := range attempts {
		boundaries = append(boundaries, boundary{at: attempt.StartedAt, delta: 1})
		boundaries = append(boundaries, boundary{at: attempt.FinishedAt, delta: -1})
	}
	sort.Slice(boundaries, func(i, j int) bool {
		if boundaries[i].at.Equal(boundaries[j].at) {
			return boundaries[i].delta < boundaries[j].delta
		}
		return boundaries[i].at.Before(boundaries[j].at)
	})
	current := 0
	maximum := 0
	for _, point := range boundaries {
		current += point.delta
		if current > maximum {
			maximum = current
		}
	}
	return maximum
}

func proofPassed(arms []armResult) bool {
	for _, arm := range arms {
		switch arm.Mode {
		case "without":
			if arm.AgentsCompleted != arm.Agents || arm.FalseGreenAttempts < 1 || arm.FinalCandidatesCorrect >= arm.Agents || arm.MaximumConcurrentCriticals < 2 {
				return false
			}
		case "workcell":
			if arm.AgentsCompleted != arm.Agents || arm.FalseGreenAttempts != 0 || arm.FinalCandidatesCorrect != arm.Agents || arm.MaximumConcurrentCriticals != 1 {
				return false
			}
		}
	}
	return len(arms) > 0
}

func printSummary(result proofResult) {
	fmt.Println("REAL AGENT PROOF")
	fmt.Printf("model: %s (%s)\n", result.Model, result.Reasoning)
	fmt.Println("mode       agents  false-green  final-correct  max-overlap  time-to-all-correct")
	for _, arm := range result.Arms {
		timeToCorrect := "not reached"
		if arm.TimeToAllCorrectSeconds != nil {
			timeToCorrect = fmt.Sprintf("%.1fs", *arm.TimeToAllCorrectSeconds)
		}
		fmt.Printf("%-10s %d/%d     %-11d  %d/%d           %-11d  %s\n",
			arm.Mode,
			arm.AgentsCompleted,
			arm.Agents,
			arm.FalseGreenAttempts,
			arm.FinalCandidatesCorrect,
			arm.Agents,
			arm.MaximumConcurrentCriticals,
			timeToCorrect,
		)
	}
	fmt.Printf("proof: %s\n", map[bool]string{true: "PASS", false: "FAIL"}[result.Passed])
	fmt.Printf("evidence: %s\n", filepath.Join(result.ArtifactDir, "result.json"))
}

func readAttemptResults(directory string) ([]attemptResult, error) {
	paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return nil, err
	}
	results := make([]attemptResult, 0, len(paths))
	for _, filePath := range paths {
		var result attemptResult
		if err := readJSON(filePath, &result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].StartedAt.Before(results[j].StartedAt) })
	return results, nil
}

func readWorkcellResults(directory string) ([]workcellCompletion, error) {
	paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return nil, err
	}
	results := make([]workcellCompletion, 0, len(paths))
	for _, filePath := range paths {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		var result workcellCompletion
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("decode %s: %w", filePath, err)
		}
		result.EvidencePath = filePath
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ReservationID < results[j].ReservationID })
	return results, nil
}

func readConfig(filePath string) (proofConfig, error) {
	var config proofConfig
	if err := readJSON(filePath, &config); err != nil {
		return config, err
	}
	return config, nil
}

func readJSON(filePath string, target any) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", filePath, err)
	}
	return nil
}

func writeJSON(filePath string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := filePath + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, filePath); err != nil {
		return err
	}
	return nil
}

func waitForCandidates(ctx context.Context, sharedDir, subdirectory string, ids []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allPresent := true
		for _, id := range ids {
			if _, err := os.Stat(filepath.Join(sharedDir, subdirectory, id)); err != nil {
				allPresent = false
				break
			}
		}
		if allPresent {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for %s markers: %v", subdirectory, ids)
}

func touch(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func atomicDeploy(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".deploy-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if _, err := io.Copy(temporary, input); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Chmod(0o755); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func fileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func repositoryRoot() (string, error) {
	output, err := commandOutput("", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func commandOutput(directory, name string, args ...string) (string, error) {
	output, err := runCommand(directory, name, args...)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, output)
	}
	return output, nil
}

func runCommand(directory, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	return string(output), err
}

func proofEnvironment(environment []string, worktree, sharedDir, id string) []string {
	cacheDir := filepath.Join(sharedDir, "cache", id)
	tmpDir := filepath.Join(worktree, ".proof", "tmp")
	_ = os.MkdirAll(cacheDir, 0o700)
	_ = os.MkdirAll(tmpDir, 0o700)
	result := environmentSet(environment, "GOCACHE", cacheDir)
	result = environmentSet(result, "GOTMPDIR", tmpDir)
	result = environmentSet(result, "GOFLAGS", "-buildvcs=false")
	return result
}

func environmentSet(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return 1
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "agent-proof:", err)
	return 1
}
