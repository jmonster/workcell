package workcell

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type resourceLock struct {
	file *os.File
	path string
}

func openResourceLock(filePath string) (*resourceLock, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("close lock file after permission failure: %w", closeErr)
		}
		return nil, errors.Join(fmt.Errorf("secure lock file: %w", err), closeErr)
	}
	return &resourceLock{file: file, path: filePath}, nil
}

func (lock *resourceLock) tryAcquire() (bool, error) {
	err := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, fmt.Errorf("lock %s: %w", lock.path, err)
}

func (lock *resourceLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	file := lock.file
	lock.file = nil
	var unlockErr error
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		unlockErr = fmt.Errorf("unlock %s: %w", lock.path, err)
	}
	var closeErr error
	if err := file.Close(); err != nil {
		closeErr = fmt.Errorf("close lock %s: %w", lock.path, err)
	}
	return errors.Join(unlockErr, closeErr)
}

func (lock *resourceLock) close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	file := lock.file
	lock.file = nil
	if err := file.Close(); err != nil {
		return fmt.Errorf("close lock %s: %w", lock.path, err)
	}
	return nil
}

func withExclusiveLock(filePath, description string, operation func() error) error {
	lock, err := openResourceLock(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", description, err)
	}
	if err := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_EX); err != nil {
		return errors.Join(fmt.Errorf("lock %s: %w", description, err), lock.close())
	}
	return errors.Join(operation(), lock.release())
}
