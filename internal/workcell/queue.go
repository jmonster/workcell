package workcell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// A blocking flock has no portable cancellation path. The head polls quickly
	// so signals remain deterministic; followers back off according to position.
	headQueuePollInterval = 20 * time.Millisecond
	maxQueuePollInterval  = 250 * time.Millisecond
	queueSequenceWidth    = 20
	queueTicketSuffix     = ".ticket"
)

type waitTicket struct {
	file *os.File
	path string
}

func (lock *resourceLock) acquireFair(paths statePaths, wait bool, signals <-chan os.Signal) (time.Duration, os.Signal, bool, int, error) {
	started := time.Now()
	if !wait {
		acquired, queueAhead, err := lock.tryAcquireWithoutBarging(paths)
		return time.Since(started), nil, acquired, queueAhead, err
	}

	ticket, err := enqueueWaitTicket(paths)
	if err != nil {
		return time.Since(started), nil, false, 0, err
	}
	for {
		head, queueAhead, err := ticket.position(paths)
		if err != nil {
			cleanupErr := ticket.remove(paths)
			return time.Since(started), nil, false, queueAhead, errors.Join(err, cleanupErr)
		}
		if head {
			acquired, err := lock.tryAcquire()
			if err != nil {
				cleanupErr := ticket.remove(paths)
				return time.Since(started), nil, false, queueAhead, errors.Join(err, cleanupErr)
			}
			if acquired {
				if err := ticket.remove(paths); err != nil {
					releaseErr := lock.release()
					return time.Since(started), nil, false, queueAhead, errors.Join(err, releaseErr)
				}
				return time.Since(started), nil, true, queueAhead, nil
			}
		}

		timer := time.NewTimer(queuePollDelay(queueAhead))
		select {
		case signalValue := <-signals:
			if !timer.Stop() {
				<-timer.C
			}
			cleanupErr := ticket.remove(paths)
			return time.Since(started), signalValue, false, queueAhead, cleanupErr
		case <-timer.C:
		}
	}
}

func (lock *resourceLock) tryAcquireWithoutBarging(paths statePaths) (bool, int, error) {
	var acquired bool
	var queueAhead int
	err := withQueueMutex(paths.queueLockPath, func() error {
		sequences, err := liveQueueSequences(paths.queueDir)
		if err != nil {
			return err
		}
		queueAhead = len(sequences)
		if queueAhead > 0 {
			return nil
		}
		acquired, err = lock.tryAcquire()
		return err
	})
	return acquired, queueAhead, err
}

func enqueueWaitTicket(paths statePaths) (*waitTicket, error) {
	var ticket *waitTicket
	err := withQueueMutex(paths.queueLockPath, func() error {
		sequences, err := liveQueueSequences(paths.queueDir)
		if err != nil {
			return err
		}
		sequence := uint64(1)
		if len(sequences) > 0 {
			sequence = sequences[len(sequences)-1] + 1
			if sequence == 0 {
				return errors.New("queue sequence exhausted")
			}
		}
		id, err := newReservationID(time.Now().UTC())
		if err != nil {
			return err
		}
		ticketPath := filepath.Join(paths.queueDir, fmt.Sprintf("%020d-%s%s", sequence, id, queueTicketSuffix))
		file, err := os.OpenFile(ticketPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			return fmt.Errorf("create queue ticket: %w", err)
		}
		closeWithRemoval := func(operationErr error) error {
			removeErr := os.Remove(ticketPath)
			if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				removeErr = fmt.Errorf("remove incomplete queue ticket: %w", removeErr)
			} else {
				removeErr = nil
			}
			closeErr := file.Close()
			if closeErr != nil {
				closeErr = fmt.Errorf("close incomplete queue ticket: %w", closeErr)
			}
			return errors.Join(operationErr, removeErr, closeErr)
		}
		if err := file.Chmod(0o600); err != nil {
			return closeWithRemoval(fmt.Errorf("secure queue ticket: %w", err))
		}
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			return closeWithRemoval(fmt.Errorf("lock queue ticket: %w", err))
		}
		ticket = &waitTicket{file: file, path: ticketPath}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ticket, nil
}

func (ticket *waitTicket) position(paths statePaths) (bool, int, error) {
	var queueAhead int
	found := false
	err := withQueueMutex(paths.queueLockPath, func() error {
		names, err := queueTicketNames(paths.queueDir)
		if err != nil {
			return err
		}
		ticketName := filepath.Base(ticket.path)
		for _, name := range names {
			if name == ticketName {
				found = true
				break
			}
			// Later tickets cannot affect this ticket's position.
			if name > ticketName {
				break
			}
			entryPath := filepath.Join(paths.queueDir, name)
			live, err := queueTicketIsLive(entryPath)
			if err != nil {
				return err
			}
			if !live {
				continue
			}
			if _, err := parseQueueSequence(name); err != nil {
				return err
			}
			queueAhead++
		}
		return nil
	})
	if err != nil {
		return false, queueAhead, err
	}
	if !found {
		return false, queueAhead, errors.New("queue ticket disappeared before acquisition")
	}
	return queueAhead == 0, queueAhead, nil
}

func (ticket *waitTicket) remove(paths statePaths) error {
	if ticket == nil || ticket.file == nil {
		return nil
	}
	removeErr := withQueueMutex(paths.queueLockPath, func() error {
		err := os.Remove(ticket.path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove queue ticket: %w", err)
		}
		return nil
	})
	unlockErr := syscall.Flock(int(ticket.file.Fd()), syscall.LOCK_UN)
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlock queue ticket: %w", unlockErr)
	}
	closeErr := ticket.file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close queue ticket: %w", closeErr)
	}
	ticket.file = nil
	return errors.Join(removeErr, unlockErr, closeErr)
}

func liveQueueSequences(queueDir string) ([]uint64, error) {
	names, err := queueTicketNames(queueDir)
	if err != nil {
		return nil, err
	}
	sequences := make([]uint64, 0, len(names))
	for _, name := range names {
		entryPath := filepath.Join(queueDir, name)
		live, err := queueTicketIsLive(entryPath)
		if err != nil {
			return nil, err
		}
		if !live {
			continue
		}
		sequence, err := parseQueueSequence(name)
		if err != nil {
			return nil, err
		}
		sequences = append(sequences, sequence)
	}
	return sequences, nil
}

func queueTicketNames(queueDir string) ([]string, error) {
	directoryEntries, err := os.ReadDir(queueDir)
	if err != nil {
		return nil, fmt.Errorf("read queue directory: %w", err)
	}
	names := make([]string, 0, len(directoryEntries))
	for _, directoryEntry := range directoryEntries {
		if !directoryEntry.IsDir() && strings.HasSuffix(directoryEntry.Name(), queueTicketSuffix) {
			names = append(names, directoryEntry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func queueTicketIsLive(filePath string) (bool, error) {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open queue ticket: %w", err)
	}
	lockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if lockErr == nil {
		removeErr := os.Remove(filePath)
		closeErr := file.Close()
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return false, errors.Join(fmt.Errorf("remove stale queue ticket: %w", removeErr), closeErr)
		}
		if closeErr != nil {
			return false, fmt.Errorf("close stale queue ticket: %w", closeErr)
		}
		return false, nil
	}
	closeErr := file.Close()
	if !errors.Is(lockErr, syscall.EWOULDBLOCK) && !errors.Is(lockErr, syscall.EAGAIN) {
		return false, errors.Join(fmt.Errorf("inspect queue ticket: %w", lockErr), closeErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close queue ticket: %w", closeErr)
	}
	return true, nil
}

func parseQueueSequence(name string) (uint64, error) {
	stem := strings.TrimSuffix(name, queueTicketSuffix)
	if stem == name || len(stem) <= queueSequenceWidth+1 || stem[queueSequenceWidth] != '-' {
		return 0, fmt.Errorf("invalid queue ticket name %q", name)
	}
	sequence, err := strconv.ParseUint(stem[:queueSequenceWidth], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decode queue ticket sequence %q: %w", name, err)
	}
	return sequence, nil
}

func queuePollDelay(queueAhead int) time.Duration {
	if queueAhead <= 0 {
		return headQueuePollInterval
	}
	delay := headQueuePollInterval * time.Duration(queueAhead+1)
	if delay > maxQueuePollInterval {
		return maxQueuePollInterval
	}
	return delay
}

func withQueueMutex(lockPath string, operation func() error) error {
	return withExclusiveLock(lockPath, "queue mutex", operation)
}
