package workcell

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"
)

const durationHistoryLimit = 32

type durationHistoryRecord struct {
	SchemaVersion int       `json:"schema_version"`
	Resource      string    `json:"resource"`
	Samples       []float64 `json:"samples_seconds"`
}

func recordDuration(filePath, lockPath, resource string, duration time.Duration) (*DurationEstimate, error) {
	var estimate *DurationEstimate
	err := withExclusiveLock(lockPath, "duration history mutex", func() error {
		record, err := readDurationHistory(filePath, resource)
		if err != nil {
			return err
		}
		record.Samples = append(record.Samples, seconds(duration))
		if len(record.Samples) > durationHistoryLimit {
			record.Samples = append([]float64(nil), record.Samples[len(record.Samples)-durationHistoryLimit:]...)
		}
		if err := writeDurationHistory(filePath, record); err != nil {
			return err
		}
		estimate = durationEstimate(record.Samples, nil)
		return nil
	})
	return estimate, err
}

func readDurationEstimate(filePath, resource string, elapsedSeconds *float64) (*DurationEstimate, error) {
	record, err := readDurationHistory(filePath, resource)
	if err != nil {
		return nil, err
	}
	return durationEstimate(record.Samples, elapsedSeconds), nil
}

func readDurationHistory(filePath, resource string) (durationHistoryRecord, error) {
	record := durationHistoryRecord{SchemaVersion: SchemaVersion, Resource: resource}
	data, err := os.ReadFile(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return durationHistoryRecord{}, fmt.Errorf("read duration history: %w", err)
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return durationHistoryRecord{}, fmt.Errorf("decode duration history: %w", err)
	}
	if record.SchemaVersion != SchemaVersion {
		return durationHistoryRecord{}, fmt.Errorf("unsupported duration history schema %d", record.SchemaVersion)
	}
	if record.Resource != resource {
		return durationHistoryRecord{}, errors.New("duration history resource does not match lock resource")
	}
	if len(record.Samples) > durationHistoryLimit {
		return durationHistoryRecord{}, fmt.Errorf("duration history contains %d samples; maximum is %d", len(record.Samples), durationHistoryLimit)
	}
	for _, sample := range record.Samples {
		if sample < 0 || math.IsNaN(sample) || math.IsInf(sample, 0) {
			return durationHistoryRecord{}, errors.New("duration history contains an invalid sample")
		}
	}
	return record, nil
}

func writeDurationHistory(filePath string, record durationHistoryRecord) error {
	return writeAtomicJSON(filePath, "duration history", record)
}

func durationEstimate(samples []float64, elapsedSeconds *float64) *DurationEstimate {
	if len(samples) == 0 {
		return nil
	}
	var total float64
	for _, sample := range samples {
		total += sample
	}
	average := total / float64(len(samples))
	estimate := &DurationEstimate{
		SampleCount:    len(samples),
		AverageSeconds: roundSeconds(average),
	}
	if elapsedSeconds != nil {
		remaining := max(0, average-*elapsedSeconds)
		remaining = roundSeconds(remaining)
		estimate.EstimatedRemainingSeconds = &remaining
	}
	return estimate
}

func roundSeconds(value float64) float64 {
	return math.Round(value*1000) / 1000
}
