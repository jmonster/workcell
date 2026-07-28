package integration_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

const busyExit = 75

var (
	repositoryRoot string
	workcellBinary string
)

type owner struct {
	Session string `json:"session"`
}

type result struct {
	Decision      string  `json:"decision"`
	ReservationID string  `json:"reservation_id"`
	Session       string  `json:"session"`
	ExitCode      int     `json:"exit_code"`
	WaitedSeconds float64 `json:"waited_seconds"`
	LogPath       string  `json:"log_path"`
	Owner         *owner  `json:"owner"`
	QueueAhead    int     `json:"queue_ahead"`
	NextAction    struct {
		Kind     string   `json:"kind"`
		WaitArgv []string `json:"wait_argv"`
	} `json:"next_action"`
}

func TestFIFO(t *testing.T) {
	state := t.TempDir()
	files := t.TempDir()
	ready := filepath.Join(files, "ready")
	gate := filepath.Join(files, "gate")
	order := filepath.Join(files, "order")

	ownerCommand := command(state,
		"run", "fifo", "--session", "owner", "--json", "--",
		"/bin/sh", "-c", `touch "$1"; while [ ! -f "$2" ]; do sleep 0.02; done`, "sh", ready, gate,
	)
	ownerCommand.Stdout, ownerCommand.Stderr = io.Discard, io.Discard
	start(t, ownerCommand)
	waitForFile(t, ready)

	waiterB, outputB := fifoWaiter(state, "B", order)
	start(t, waiterB)
	waitForQueueDepth(t, state, "fifo", 1)
	waiterC, outputC := fifoWaiter(state, "C", order)
	start(t, waiterC)
	waitForQueueDepth(t, state, "fifo", 2)

	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ownerCommand.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := waiterB.Wait(); err != nil {
		t.Fatalf("waiter B failed: %v; %s", err, outputB.String())
	}
	if err := waiterC.Wait(); err != nil {
		t.Fatalf("waiter C failed: %v; %s", err, outputC.String())
	}
	orderData, err := os.ReadFile(order)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(orderData)); !slices.Equal(got, []string{"B", "C"}) {
		t.Fatalf("acquisition order = %v", got)
	}
}

func TestWaitAcquiresAfterOwnerExits(t *testing.T) {
	state := t.TempDir()
	files := t.TempDir()
	ready := filepath.Join(files, "ready")
	gate := filepath.Join(files, "gate")

	ownerCommand := command(state,
		"run", "acceptance-wait", "--session", "owner", "--json", "--",
		"/bin/sh", "-c", `touch "$1"; while [ ! -f "$2" ]; do sleep 0.02; done`, "sh", ready, gate,
	)
	ownerCommand.Stdout, ownerCommand.Stderr = io.Discard, io.Discard
	start(t, ownerCommand)
	waitForFile(t, ready)

	waiterCommand := command(state,
		"run", "acceptance-wait", "--wait", "--session", "waiter", "--json", "--",
		"/usr/bin/true",
	)
	var waiterOutput bytes.Buffer
	waiterCommand.Stdout, waiterCommand.Stderr = &waiterOutput, &waiterOutput
	start(t, waiterCommand)
	waiterDone := make(chan error, 1)
	go func() { waiterDone <- waiterCommand.Wait() }()
	select {
	case err := <-waiterDone:
		t.Fatalf("--wait returned before the owner exited: exit=%d output=%s", exitCode(err), waiterOutput.String())
	case <-time.After(100 * time.Millisecond):
	}

	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ownerCommand.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := <-waiterDone; err != nil {
		t.Fatalf("waiter failed after owner exit: %v; %s", err, waiterOutput.String())
	}
	var completed result
	decodeOne(t, waiterOutput.String(), &completed)
	if completed.Decision != "completed" || completed.Session != "waiter" || completed.ExitCode != 0 || completed.WaitedSeconds <= 0 {
		t.Fatalf("unexpected waiter result: %+v", completed)
	}
}

func TestMain(tests *testing.M) {
	if injectedBinary := os.Getenv("WORKCELL_INTEGRATION_BINARY"); injectedBinary != "" {
		absolute, err := filepath.Abs(injectedBinary)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		info, err := os.Stat(absolute)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			fmt.Fprintf(os.Stderr, "WORKCELL_INTEGRATION_BINARY is not executable: %s\n", absolute)
			os.Exit(1)
		}
		workcellBinary = absolute
		os.Exit(tests.Run())
	}

	root, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	repositoryRoot = root
	buildDirectory, err := os.MkdirTemp("", "workcell-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	workcellBinary = filepath.Join(buildDirectory, "workcell")
	build := exec.Command("go", "build", "-o", workcellBinary, "./cmd/workcell")
	build.Dir = repositoryRoot
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		_ = os.RemoveAll(buildDirectory)
		os.Exit(1)
	}
	exitCode := tests.Run()
	_ = os.RemoveAll(buildDirectory)
	os.Exit(exitCode)
}

func TestResourceCoordination(t *testing.T) {
	state := t.TempDir()
	files := t.TempDir()
	ready := filepath.Join(files, "ready")
	gate := filepath.Join(files, "gate")
	contenderRan := filepath.Join(files, "contender-ran")

	ownerCommand := command(state,
		"run", "shared-gpu", "--session", "owner", "--json", "--",
		"/bin/sh", "-c", `touch "$1"; while [ ! -f "$2" ]; do sleep 0.02; done`, "sh", ready, gate,
	)
	var ownerOutput bytes.Buffer
	ownerCommand.Stdout, ownerCommand.Stderr = &ownerOutput, &ownerOutput
	start(t, ownerCommand)
	waitForFile(t, ready)

	statusOutput, statusError, err := run(state, "status", "shared-gpu", "--json")
	if err != nil || statusError != "" {
		t.Fatalf("status failed: %v; %s", err, statusError)
	}
	var owned result
	decodeOne(t, statusOutput, &owned)
	if owned.Decision != "owned" || owned.Owner == nil || owned.Owner.Session != "owner" {
		t.Fatalf("unexpected owned status: %+v", owned)
	}

	busyOutput, _, busyError := run(state,
		"run", "shared-gpu", "--session", "contender", "--json", "--",
		"/bin/sh", "-c", `touch "$1"`, "sh", contenderRan,
	)
	if exitCode(busyError) != busyExit {
		t.Fatalf("contender exit = %d, want %d; %s", exitCode(busyError), busyExit, busyOutput)
	}
	var busy result
	decodeOne(t, busyOutput, &busy)
	if busy.Decision != "busy" || busy.Owner == nil || busy.Owner.Session != "owner" || busy.NextAction.Kind != "wait_or_continue" || len(busy.NextAction.WaitArgv) == 0 || busy.NextAction.WaitArgv[0] != workcellBinary || !slices.Contains(busy.NextAction.WaitArgv, "--wait") {
		t.Fatalf("unexpected busy result: %+v", busy)
	}
	if _, err := os.Stat(contenderRan); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("contender entered the critical section")
	}

	if output, stderr, err := run(state, "run", "other-resource", "--json", "--", "/usr/bin/true"); err != nil {
		t.Fatalf("different resource was blocked: %v; %s%s", err, stderr, output)
	}

	waiterCommand := exec.Command(busy.NextAction.WaitArgv[0], busy.NextAction.WaitArgv[1:]...)
	waiterCommand.Dir = repositoryRoot
	waiterCommand.Env = environmentWithState(state)
	var waiterOutput bytes.Buffer
	waiterCommand.Stdout, waiterCommand.Stderr = &waiterOutput, &waiterOutput
	start(t, waiterCommand)
	waiterDone := make(chan error, 1)
	go func() { waiterDone <- waiterCommand.Wait() }()
	select {
	case err := <-waiterDone:
		t.Fatalf("waiter completed while owner was active: %v; %s", err, waiterOutput.String())
	case <-time.After(100 * time.Millisecond):
	}

	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ownerCommand.Wait(); err != nil {
		t.Fatalf("owner failed: %v; %s", err, ownerOutput.String())
	}
	if err := <-waiterDone; err != nil {
		t.Fatalf("waiter failed: %v; %s", err, waiterOutput.String())
	}
	var completed result
	decodeOne(t, waiterOutput.String(), &completed)
	if completed.Decision != "completed" || completed.Session != "contender" || completed.WaitedSeconds <= 0 {
		t.Fatalf("unexpected waiter completion: %+v", completed)
	}
	if _, err := os.Stat(contenderRan); err != nil {
		t.Fatal("returned wait_argv did not run the contender command")
	}

	freeOutput, _, err := run(state, "status", "shared-gpu", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var free result
	decodeOne(t, freeOutput, &free)
	if free.Decision != "free" {
		t.Fatalf("unexpected free status: %+v", free)
	}
}

func TestOutputAndExitContract(t *testing.T) {
	stdout, stderr, err := run(t.TempDir(),
		"run", "output", "--session", "agent", "--json", "--",
		"/bin/sh", "-c", `printf 'out\n'; printf 'err\n' >&2; exit 42`,
	)
	if exitCode(err) != 42 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", exitCode(err), stderr)
	}
	var completed result
	decodeOne(t, stdout, &completed)
	if completed.Decision != "completed" || completed.ReservationID == "" || completed.Session != "agent" || completed.ExitCode != 42 || completed.LogPath == "" {
		t.Fatalf("unexpected completion: %+v", completed)
	}
	logData, err := os.ReadFile(completed.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(logData, []byte("out")) || !bytes.Contains(logData, []byte("err")) {
		t.Fatalf("log did not capture command output: %s", logData)
	}

	humanOut, humanErr, err := run(t.TempDir(),
		"run", "human", "--session", "agent", "--",
		"/bin/sh", "-c", `printf 'command-output\n'`,
	)
	if err != nil || humanOut != "command-output\n" || humanErr == "" {
		t.Fatalf("unexpected human output: stdout=%q stderr=%q error=%v", humanOut, humanErr, err)
	}
}

func TestSignalForwarding(t *testing.T) {
	files := t.TempDir()
	ready := filepath.Join(files, "ready")
	forwarded := filepath.Join(files, "forwarded")
	wrapped := command(t.TempDir(),
		"run", "signal", "--session", "agent", "--json", "--",
		"/bin/sh", "-c", `trap 'touch "$1"; exit 23' TERM; touch "$2"; while :; do sleep 1; done`, "sh", forwarded, ready,
	)
	var output bytes.Buffer
	wrapped.Stdout, wrapped.Stderr = &output, &output
	start(t, wrapped)
	waitForFile(t, ready)
	if err := wrapped.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := exitCode(wrapped.Wait()); code != 23 {
		t.Fatalf("signal exit = %d, want 23; %s", code, output.String())
	}
	waitForFile(t, forwarded)
}

func TestChildRetainsLockIfWrapperDies(t *testing.T) {
	state := t.TempDir()
	files := t.TempDir()
	ready := filepath.Join(files, "ready")
	gate := filepath.Join(files, "gate")
	done := filepath.Join(files, "done")
	ownerCommand := command(state,
		"run", "shared", "--session", "owner", "--json", "--",
		"/bin/sh", "-c", `touch "$1"; while [ ! -f "$2" ]; do sleep 0.02; done; touch "$3"`, "sh", ready, gate, done,
	)
	ownerCommand.Stdout, ownerCommand.Stderr = io.Discard, io.Discard
	start(t, ownerCommand)
	waitForFile(t, ready)
	if err := ownerCommand.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = ownerCommand.Wait()

	output, _, err := run(state, "run", "shared", "--json", "--", "/usr/bin/true")
	if exitCode(err) != busyExit {
		t.Fatalf("child lost the lock with its wrapper: exit=%d output=%s", exitCode(err), output)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, done)
	if output, stderr, err := run(state, "run", "shared", "--wait", "--json", "--", "/usr/bin/true"); err != nil {
		t.Fatalf("resource did not release after child exit: %v; %s%s", err, stderr, output)
	}
}

func TestUsageErrors(t *testing.T) {
	tests := [][]string{nil, []string{"run", "gpu"}, []string{"unknown"}}
	for _, arguments := range tests {
		stdout, stderr, err := run(t.TempDir(), arguments...)
		if exitCode(err) != 2 || stdout != "" || stderr == "" {
			t.Fatalf("args=%v exit=%d stdout=%q stderr=%q", arguments, exitCode(err), stdout, stderr)
		}
	}
}

func command(state string, arguments ...string) *exec.Cmd {
	command := exec.Command(workcellBinary, arguments...)
	command.Dir = repositoryRoot
	command.Env = environmentWithState(state)
	return command
}

func run(state string, arguments ...string) (string, string, error) {
	command := command(state, arguments...)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func start(t *testing.T, command *exec.Cmd) {
	t.Helper()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Signal(syscall.SIGTERM)
		}
	})
}

func waitForFile(t *testing.T, filePath string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filePath); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filePath)
}

func fifoWaiter(state, name, order string) (*exec.Cmd, *bytes.Buffer) {
	command := command(state,
		"run", "fifo", "--wait", "--session", name, "--json", "--",
		"/bin/sh", "-c", `printf '%s\n' "$1" >> "$2"`, "sh", name, order,
	)
	output := &bytes.Buffer{}
	command.Stdout, command.Stderr = output, output
	return command, output
}

func waitForQueueDepth(t *testing.T, state, resource string, wanted int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stdout, _, err := run(state, "run", resource, "--session", "probe", "--json", "--", "/usr/bin/true")
		if exitCode(err) == busyExit {
			var busy result
			decodeOne(t, stdout, &busy)
			if busy.QueueAhead == wanted {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for queue depth %d", wanted)
}

func decodeOne(t *testing.T, raw string, target any) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode JSON: %v; %s", err, raw)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contained multiple JSON values: %s", raw)
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

func environmentWithState(state string) []string {
	result := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "WORKCELL_STATE_DIR=") {
			result = append(result, entry)
		}
	}
	return append(result, "WORKCELL_STATE_DIR="+state)
}
