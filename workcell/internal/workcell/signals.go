package workcell

import (
	"os"
	"os/signal"
	"syscall"
)

func subscribeSignals() chan os.Signal {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return signals
}

func stopSignals(signals chan os.Signal) {
	signal.Stop(signals)
}

func forwardSignals(signals <-chan os.Signal, processGroupID int, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		default:
		}
		select {
		case signalValue := <-signals:
			select {
			case <-done:
				return
			default:
			}
			unixSignal, ok := signalValue.(syscall.Signal)
			if !ok {
				continue
			}
			_ = syscall.Kill(-processGroupID, unixSignal)
		case <-done:
			return
		}
	}
}

func signalExitCode(signalValue os.Signal) int {
	unixSignal, ok := signalValue.(syscall.Signal)
	if !ok {
		return 1
	}
	return 128 + int(unixSignal)
}
