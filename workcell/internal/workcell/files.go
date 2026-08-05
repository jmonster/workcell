package workcell

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func writeAtomicJSON(filePath, description string, value any) (resultErr error) {
	temporary, err := os.CreateTemp(filepath.Dir(filePath), ".workcell-*")
	if err != nil {
		return fmt.Errorf("create %s: %w", description, err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if err := temporary.Close(); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("close incomplete %s: %w", description, err))
			}
		}
		if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove incomplete %s: %w", description, err))
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure %s: %w", description, err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode %s: %w", description, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", description, err)
	}
	closeErr := temporary.Close()
	closed = true
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", description, closeErr)
	}
	if err := os.Rename(temporaryPath, filePath); err != nil {
		return fmt.Errorf("publish %s: %w", description, err)
	}
	return nil
}
