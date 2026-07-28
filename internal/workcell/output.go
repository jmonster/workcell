package workcell

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "workcell: %v\n", err)
	return ExitInternal
}

func writeHumanAcquired(w io.Writer, resource string, owner Owner) {
	fmt.Fprintf(w, "ACQUIRED  %s\n", resource)
	fmt.Fprintf(w, "session   %s\n", owner.Session)
	writeHumanRepository(w, owner)
	fmt.Fprintf(w, "log       %s\n\n", displayPath(owner.LogPath))
}

func writeHumanBusy(w io.Writer, resource string, owner *Owner, queueAhead int, estimate *DurationEstimate) {
	fmt.Fprintf(w, "BUSY  %s\n\n", resource)
	if owner != nil {
		writeHumanOwnerDetails(w, *owner, estimate)
	} else {
		fmt.Fprintln(w, "owner     unavailable")
		writeHumanEstimate(w, estimate)
	}
	writeHumanQueue(w, queueAhead)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "next:")
	fmt.Fprintln(w, "  rerun with --wait")
	fmt.Fprintln(w, "  continue unrelated work")
	fmt.Fprintln(w, "  inspect the owner log")
}

func writeHumanReleased(w io.Writer, resource string, exitCode int, ranSeconds float64, estimate *DurationEstimate) {
	fmt.Fprintf(w, "\nRELEASED  %s\n", resource)
	fmt.Fprintf(w, "exit      %d\n", exitCode)
	fmt.Fprintf(w, "duration  %s\n", formatDuration(ranSeconds))
	writeHumanEstimate(w, estimate)
}

func writeHumanFreeStatus(w io.Writer, resource string, estimate *DurationEstimate) {
	fmt.Fprintf(w, "FREE  %s\n", resource)
	writeHumanEstimate(w, estimate)
}

func writeHumanOwnedStatus(w io.Writer, resource string, owner *Owner, queueAhead int, estimate *DurationEstimate) {
	fmt.Fprintf(w, "OWNED  %s\n\n", resource)
	if owner == nil {
		fmt.Fprintln(w, "owner     unavailable")
		writeHumanEstimate(w, estimate)
		writeHumanQueue(w, queueAhead)
		return
	}
	writeHumanOwnerDetails(w, *owner, estimate)
	writeHumanQueue(w, queueAhead)
}

func writeHumanOwnerDetails(w io.Writer, owner Owner, estimate *DurationEstimate) {
	fmt.Fprintf(w, "owner     %s\n", owner.Session)
	writeHumanRepository(w, owner)
	fmt.Fprintf(w, "running   %s\n", formatDuration(owner.Elapsed))
	writeHumanEstimate(w, estimate)
	fmt.Fprintf(w, "command   %s\n", owner.Command)
	fmt.Fprintf(w, "log       %s\n", displayPath(owner.LogPath))
}

func writeHumanRepository(w io.Writer, owner Owner) {
	if owner.Repository != "" {
		fmt.Fprintf(w, "repo      %s\n", displayText(owner.Repository))
	}
	if owner.Branch != "" {
		fmt.Fprintf(w, "branch    %s\n", displayText(owner.Branch))
	}
}

func writeHumanEstimate(w io.Writer, estimate *DurationEstimate) {
	if estimate == nil {
		return
	}
	runLabel := "runs"
	if estimate.SampleCount == 1 {
		runLabel = "run"
	}
	fmt.Fprintf(w, "average   %s (%d %s)\n", formatDuration(estimate.AverageSeconds), estimate.SampleCount, runLabel)
	if estimate.EstimatedRemainingSeconds != nil {
		fmt.Fprintf(w, "remaining %s estimated\n", formatDuration(*estimate.EstimatedRemainingSeconds))
	}
}

func writeHumanQueue(w io.Writer, queueAhead int) {
	if queueAhead > 0 {
		fmt.Fprintf(w, "queued    %d waiter(s) ahead\n", queueAhead)
	}
}

func formatDuration(seconds float64) string {
	if seconds < 10 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	minutes := int(seconds) / 60
	remainder := int(seconds) % 60
	if minutes == 0 {
		return fmt.Sprintf("%ds", remainder)
	}
	return fmt.Sprintf("%dm %ds", minutes, remainder)
}

func displayCommand(argv []string) string {
	parts := make([]string, len(argv))
	for i, arg := range argv {
		if arg != "" && utf8.ValidString(arg) && !hasNonPrintingCharacter(arg) && !strings.ContainsAny(arg, " \t\r\n\"'\\$`;&|<>*?()[]{}!") {
			parts[i] = arg
			continue
		}
		parts[i] = strconv.Quote(arg)
	}
	return strings.Join(parts, " ")
}

func displayPath(filePath string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(filePath, home+string(filepath.Separator)) {
		filePath = "~" + strings.TrimPrefix(filePath, home)
	}
	return displayText(filePath)
}

func displayText(value string) string {
	if utf8.ValidString(value) && !hasNonPrintingCharacter(value) {
		return value
	}
	return strconv.Quote(value)
}
