package workcell

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ExitUsage     = 2
	ExitBusy      = 75
	ExitInternal  = 70
	ExitCannotRun = 126
	ExitNoCommand = 127
)

const (
	runSyntax    = "workcell run <resource> [--wait] [--session <id>] [--json] -- <command...>"
	statusSyntax = "workcell status <resource> [--json]"
	runUsage     = "usage: " + runSyntax
	statusUsage  = "usage: " + statusSyntax
)

var errHelpRequested = errors.New("help requested")

type runOptions struct {
	Executable string
	Resource   string
	Wait       bool
	Session    string
	JSON       bool
	Command    []string
}

type statusOptions struct {
	Resource string
	JSON     bool
}

func Main(executable string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return ExitUsage
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "run":
		opts, err := parseRunArgs(args[1:])
		if errors.Is(err, errHelpRequested) {
			printRunUsage(stdout)
			return 0
		}
		if err != nil {
			fmt.Fprintf(stderr, "workcell run: %v\n", err)
			printRunUsage(stderr)
			return ExitUsage
		}
		opts.Executable = executable
		return run(opts, stdout, stderr)
	case "status":
		opts, err := parseStatusArgs(args[1:])
		if errors.Is(err, errHelpRequested) {
			printStatusUsage(stdout)
			return 0
		}
		if err != nil {
			fmt.Fprintf(stderr, "workcell status: %v\n", err)
			printStatusUsage(stderr)
			return ExitUsage
		}
		return status(opts, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "workcell: unknown command %q\n", args[0])
		printUsage(stderr)
		return ExitUsage
	}
}

func parseRunArgs(args []string) (runOptions, error) {
	var opts runOptions
	var sessionSet bool
	if len(args) == 0 {
		return opts, errors.New("resource is required")
	}
	if isHelp(args[0]) {
		return opts, errHelpRequested
	}

	opts.Resource = args[0]
	if err := validateField("resource", opts.Resource); err != nil {
		return opts, err
	}

	separator := -1
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			separator = i
			break
		}
		switch {
		case arg == "--wait":
			opts.Wait = true
		case arg == "--json":
			opts.JSON = true
		case arg == "--session":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s requires a value", arg)
			}
			i++
			opts.Session = args[i]
			sessionSet = true
		case strings.HasPrefix(arg, "--session="):
			opts.Session = strings.TrimPrefix(arg, "--session=")
			sessionSet = true
		case isHelp(arg):
			return opts, errHelpRequested
		default:
			return opts, fmt.Errorf("unknown option %q", arg)
		}
	}

	if separator == -1 {
		return opts, errors.New("missing -- command separator")
	}
	if separator == len(args)-1 {
		return opts, errors.New("wrapped command is required")
	}
	opts.Command = append([]string(nil), args[separator+1:]...)

	if sessionSet {
		if err := validateField("session", opts.Session); err != nil {
			return opts, err
		}
	}
	return opts, nil
}

func parseStatusArgs(args []string) (statusOptions, error) {
	var opts statusOptions
	if len(args) == 0 {
		return opts, errors.New("resource is required")
	}
	if isHelp(args[0]) {
		return opts, errHelpRequested
	}
	opts.Resource = args[0]
	if err := validateField("resource", opts.Resource); err != nil {
		return opts, err
	}
	for _, arg := range args[1:] {
		switch {
		case arg == "--json":
			opts.JSON = true
		case isHelp(arg):
			return opts, errHelpRequested
		default:
			return opts, fmt.Errorf("unknown option %q", arg)
		}
	}
	return opts, nil
}

func validateField(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	if len(value) > 512 {
		return fmt.Errorf("%s exceeds 512 bytes", name)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	if hasNonPrintingCharacter(value) {
		return fmt.Errorf("%s contains a non-printing character", name)
	}
	return nil
}

func hasNonPrintingCharacter(value string) bool {
	for _, character := range value {
		if !unicode.IsPrint(character) {
			return true
		}
	}
	return false
}

func isHelp(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, runUsage)
	fmt.Fprintln(w, "       "+statusSyntax)
}

func printRunUsage(w io.Writer) {
	fmt.Fprintln(w, runUsage)
	fmt.Fprintln(w, "  --session <id> identifies the agent task in owner and status results; it does not affect queue order")
}

func printStatusUsage(w io.Writer) {
	fmt.Fprintln(w, statusUsage)
}
