package workcell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type runContext struct {
	cwd        string
	host       string
	repository string
	branch     string
	actor      string
}

func run(opts runOptions, stdout, stderr io.Writer) int {
	sessionID, err := resolveSession(opts.Session)
	if err != nil {
		return writeError(stderr, err)
	}
	context, err := resolveRunContext()
	if err != nil {
		return writeError(stderr, err)
	}
	paths, err := pathsForResource(opts.Resource)
	if err != nil {
		return writeError(stderr, err)
	}
	lock, err := openResourceLock(paths.lockPath)
	if err != nil {
		return writeError(stderr, err)
	}

	signals := subscribeSignals()
	defer stopSignals(signals)
	waited, interruptedBy, acquired, queueAhead, err := lock.acquireFair(paths, opts.Wait, signals)
	if err != nil {
		return writeError(stderr, errors.Join(err, lock.close()))
	}
	if interruptedBy != nil {
		if closeErr := lock.close(); closeErr != nil {
			fmt.Fprintf(stderr, "workcell: %v\n", closeErr)
		}
		return signalExitCode(interruptedBy)
	}
	if !acquired {
		if err := lock.close(); err != nil {
			return writeError(stderr, err)
		}
		owner, estimate, err := inspectUnavailableResource(paths, opts.Resource)
		if err != nil {
			return writeError(stderr, err)
		}
		result := BusyResult{
			SchemaVersion:    SchemaVersion,
			Resource:         opts.Resource,
			Decision:         "busy",
			Owner:            owner,
			QueueAhead:       queueAhead,
			DurationEstimate: estimate,
			NextAction: NextAction{
				Kind:     "wait_or_continue",
				WaitArgv: waitArgv(opts, sessionID),
			},
		}
		if opts.JSON {
			if err := writeJSON(stdout, result); err != nil {
				return writeError(stderr, fmt.Errorf("write JSON result: %w", err))
			}
		} else {
			writeHumanBusy(stdout, opts.Resource, owner, queueAhead, estimate)
		}
		return ExitBusy
	}

	return runAcquired(opts, sessionID, waited, context, paths, lock, signals, stdout, stderr)
}

func resolveRunContext() (runContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return runContext{}, fmt.Errorf("resolve working directory: %w", err)
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	repository, branch := discoverRepository(cwd)
	actor := os.Getenv("WORKCELL_ACTOR")
	if actor == "" {
		actor = "agent"
	}
	if err := validateField("actor", actor); err != nil {
		return runContext{}, fmt.Errorf("WORKCELL_ACTOR: %w", err)
	}
	return runContext{cwd: cwd, host: host, repository: repository, branch: branch, actor: actor}, nil
}

func runAcquired(opts runOptions, sessionID string, waited time.Duration, context runContext, paths statePaths, lock *resourceLock, signals chan os.Signal, stdout, stderr io.Writer) (exitCode int) {
	defer func() {
		if err := lock.release(); err != nil {
			fmt.Fprintf(stderr, "workcell: %v\n", err)
			if exitCode == 0 {
				exitCode = ExitInternal
			}
		}
	}()

	if err := removeMetadata(paths.metadataPath); err != nil {
		return writeError(stderr, err)
	}
	now := time.Now().UTC()
	reservationID, err := newReservationID(now)
	if err != nil {
		return writeError(stderr, err)
	}
	logPath := filepath.Join(paths.logsDir, reservationID+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return writeError(stderr, fmt.Errorf("create command log: %w", err))
	}
	logClosed := false
	closeLog := func() error {
		if logClosed {
			return nil
		}
		logClosed = true
		if err := logFile.Close(); err != nil {
			return fmt.Errorf("close command log: %w", err)
		}
		return nil
	}
	removeLog := func() error {
		err := os.Remove(logPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove command log: %w", err)
		}
		return nil
	}
	owner := Owner{
		ReservationID: reservationID,
		Session:       sessionID,
		Actor:         context.actor,
		Host:          context.host,
		PID:           os.Getpid(),
		Repository:    context.repository,
		Branch:        context.branch,
		CWD:           context.cwd,
		Command:       displayCommand(opts.Command),
		StartedAt:     now.Format(time.RFC3339Nano),
		Elapsed:       0,
		LogPath:       logPath,
	}
	if err := writeMetadata(paths.metadataPath, opts.Resource, owner); err != nil {
		cleanupErr := errors.Join(closeLog(), removeLog(), lock.release())
		return writeError(stderr, errors.Join(err, cleanupErr))
	}

	if !opts.JSON {
		writeHumanAcquired(stderr, opts.Resource, owner)
	}
	command := exec.Command(opts.Command[0], opts.Command[1:]...)
	command.Dir = context.cwd
	command.Env = os.Environ()
	command.Stdin = os.Stdin
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.ExtraFiles = []*os.File{lock.file}
	if opts.JSON {
		command.Stdout = logFile
		command.Stderr = logFile
	} else {
		command.Stdout = io.MultiWriter(stdout, logFile)
		command.Stderr = io.MultiWriter(stderr, logFile)
	}

	started := time.Now()
	if err := command.Start(); err != nil {
		cleanupErr := errors.Join(
			removeMetadataForReservation(paths.metadataPath, opts.Resource, reservationID),
			closeLog(),
			removeLog(),
			lock.release(),
		)
		if cleanupErr != nil {
			fmt.Fprintf(stderr, "workcell: cleanup after command start failure: %v\n", cleanupErr)
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "workcell: command not found: %s\n", opts.Command[0])
			return ExitNoCommand
		}
		fmt.Fprintf(stderr, "workcell: start command: %v\n", err)
		return ExitCannotRun
	}
	stopForwarding := make(chan struct{})
	forwardingDone := make(chan struct{})
	go func() {
		forwardSignals(signals, command.Process.Pid, stopForwarding)
		close(forwardingDone)
	}()
	waitErr := command.Wait()
	close(stopForwarding)
	<-forwardingDone
	ran := time.Since(started)
	exitCode = commandExitCode(waitErr)

	metadataErr := removeMetadataForReservation(paths.metadataPath, opts.Resource, reservationID)
	releaseErr := lock.release()
	logErr := closeLog()
	estimate, historyErr := recordDuration(paths.historyPath, paths.historyLockPath, opts.Resource, ran)
	for _, completionErr := range []error{metadataErr, releaseErr, logErr} {
		if completionErr == nil {
			continue
		}
		fmt.Fprintf(stderr, "workcell: %v\n", completionErr)
		if exitCode == 0 {
			exitCode = ExitInternal
		}
	}
	if historyErr != nil {
		fmt.Fprintf(stderr, "workcell: duration estimate unavailable: %v\n", historyErr)
	}

	result := CompletedResult{
		SchemaVersion:    SchemaVersion,
		Resource:         opts.Resource,
		Decision:         "completed",
		ReservationID:    reservationID,
		Session:          sessionID,
		ExitCode:         exitCode,
		Waited:           seconds(waited),
		Ran:              seconds(ran),
		LogPath:          logPath,
		DurationEstimate: estimate,
	}
	if opts.JSON {
		if err := writeJSON(stdout, result); err != nil {
			return writeError(stderr, fmt.Errorf("write JSON result: %w", err))
		}
	} else {
		writeHumanReleased(stderr, opts.Resource, exitCode, ran.Seconds(), estimate)
	}
	return exitCode
}

func waitArgv(opts runOptions, sessionID string) []string {
	argv := []string{opts.Executable, "run", opts.Resource, "--wait", "--session", sessionID}
	if opts.JSON {
		argv = append(argv, "--json")
	}
	argv = append(argv, "--")
	argv = append(argv, opts.Command...)
	return argv
}

func commandExitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) {
		return ExitInternal
	}
	if waitStatus, ok := exitError.Sys().(syscall.WaitStatus); ok {
		if waitStatus.Signaled() {
			return 128 + int(waitStatus.Signal())
		}
		return waitStatus.ExitStatus()
	}
	if code := exitError.ExitCode(); code >= 0 {
		return code
	}
	return ExitInternal
}

func seconds(duration time.Duration) float64 {
	return float64(duration.Round(time.Millisecond)) / float64(time.Second)
}
