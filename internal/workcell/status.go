package workcell

import (
	"errors"
	"fmt"
	"io"
)

func status(opts statusOptions, stdout, stderr io.Writer) int {
	paths, err := pathsForResource(opts.Resource)
	if err != nil {
		return writeError(stderr, err)
	}
	lock, err := openResourceLock(paths.lockPath)
	if err != nil {
		return writeError(stderr, err)
	}
	acquired, queueAhead, err := lock.tryAcquireWithoutBarging(paths)
	if err != nil {
		return writeError(stderr, errors.Join(err, lock.close()))
	}
	if acquired {
		if err := errors.Join(removeMetadata(paths.metadataPath), lock.release()); err != nil {
			return writeError(stderr, err)
		}
		estimate, _ := readDurationEstimate(paths.historyPath, opts.Resource, nil)
		result := StatusResult{SchemaVersion: SchemaVersion, Resource: opts.Resource, Decision: "free", DurationEstimate: estimate}
		if opts.JSON {
			if err := writeJSON(stdout, result); err != nil {
				return writeError(stderr, fmt.Errorf("write JSON result: %w", err))
			}
		} else {
			writeHumanFreeStatus(stdout, opts.Resource, estimate)
		}
		return 0
	}
	if err := lock.close(); err != nil {
		return writeError(stderr, err)
	}
	owner, estimate, err := inspectUnavailableResource(paths, opts.Resource)
	if err != nil {
		return writeError(stderr, err)
	}
	result := StatusResult{
		SchemaVersion:    SchemaVersion,
		Resource:         opts.Resource,
		Decision:         "owned",
		Owner:            owner,
		QueueAhead:       queueAhead,
		DurationEstimate: estimate,
	}
	if opts.JSON {
		if err := writeJSON(stdout, result); err != nil {
			return writeError(stderr, fmt.Errorf("write JSON result: %w", err))
		}
	} else {
		writeHumanOwnedStatus(stdout, opts.Resource, owner, queueAhead, estimate)
	}
	return 0
}
