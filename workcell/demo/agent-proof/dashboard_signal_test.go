package main

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDashboardCtrlCCancelsProof(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := newDashboardModel(nil, 3, cancel)

	_, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if command == nil {
		t.Fatal("Ctrl-C did not quit the dashboard")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Ctrl-C did not cancel the proof")
	}
}

func TestDashboardWaitsForRightArrow(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	model := newDashboardModel(nil, 3, cancel)
	model.Update(startArmMsg{mode: "without"})
	done := make(chan struct{})
	model.Update(awaitInputMsg{prompt: "CONTROL COMPLETE", done: done})

	select {
	case <-done:
		t.Fatal("proof continued without user input")
	default:
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRight})
	select {
	case <-done:
	default:
		t.Fatal("Right did not continue a completed step")
	}
}
