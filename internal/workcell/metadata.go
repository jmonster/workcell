package workcell

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"
)

type metadataRecord struct {
	SchemaVersion int    `json:"schema_version"`
	Resource      string `json:"resource"`
	Owner         Owner  `json:"owner"`
}

func writeMetadata(filePath, resource string, owner Owner) error {
	record := metadataRecord{SchemaVersion: SchemaVersion, Resource: resource, Owner: owner}
	return writeAtomicJSON(filePath, "metadata file", record)
}

func readMetadata(filePath, resource string) (Owner, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Owner{}, err
	}
	var record metadataRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return Owner{}, fmt.Errorf("decode owner metadata: %w", err)
	}
	if record.SchemaVersion != SchemaVersion {
		return Owner{}, fmt.Errorf("unsupported metadata schema %d", record.SchemaVersion)
	}
	if record.Resource != resource {
		return Owner{}, errors.New("metadata resource does not match lock resource")
	}
	startedAt, err := time.Parse(time.RFC3339Nano, record.Owner.StartedAt)
	if err != nil {
		return Owner{}, fmt.Errorf("decode owner start time: %w", err)
	}
	record.Owner.Elapsed = max(0, time.Since(startedAt).Seconds())
	return record.Owner, nil
}

func readMetadataWithRetry(filePath, resource string) (Owner, error) {
	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		owner, err := readMetadata(filePath, resource)
		if err == nil {
			return owner, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Owner{}, err
		}
		lastErr = err
		if attempt < 19 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return Owner{}, lastErr
}

func removeMetadata(filePath string) error {
	err := os.Remove(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove metadata: %w", err)
	}
	return nil
}

func removeMetadataForReservation(filePath, resource, reservationID string) error {
	owner, err := readMetadata(filePath, resource)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner.ReservationID != reservationID {
		return nil
	}
	return removeMetadata(filePath)
}

func newReservationID(now time.Time) (string, error) {
	var raw [16]byte
	milliseconds := uint64(now.UnixMilli())
	raw[0] = byte(milliseconds >> 40)
	raw[1] = byte(milliseconds >> 32)
	raw[2] = byte(milliseconds >> 24)
	raw[3] = byte(milliseconds >> 16)
	raw[4] = byte(milliseconds >> 8)
	raw[5] = byte(milliseconds)
	if _, err := rand.Read(raw[6:]); err != nil {
		return "", fmt.Errorf("generate reservation ID: %w", err)
	}

	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	value := new(big.Int).SetBytes(raw[:])
	base := big.NewInt(32)
	remainder := new(big.Int)
	encoded := make([]byte, 26)
	for i := len(encoded) - 1; i >= 0; i-- {
		value.QuoRem(value, base, remainder)
		encoded[i] = alphabet[remainder.Int64()]
	}
	return string(encoded), nil
}
