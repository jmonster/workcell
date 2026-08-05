package workcell

import (
	"errors"
	"fmt"
	"os"
)

func inspectUnavailableResource(paths statePaths, resource string) (*Owner, *DurationEstimate, error) {
	ownerValue, metadataErr := readMetadataWithRetry(paths.metadataPath, resource)
	if metadataErr != nil && !errors.Is(metadataErr, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("resource is unavailable but owner metadata cannot be read: %w", metadataErr)
	}

	var owner *Owner
	var elapsedSeconds *float64
	if metadataErr == nil {
		owner = &ownerValue
		elapsedSeconds = &ownerValue.Elapsed
	}
	estimate, _ := readDurationEstimate(paths.historyPath, resource, elapsedSeconds)
	return owner, estimate, nil
}
