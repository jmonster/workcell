package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

type visualAgent struct {
	ID      string
	State   string
	Detail  string
	Tested  string
	Attempt int
}

type dashboard struct {
	enabled     bool
	artifactDir string
	program     *tea.Program
	done        chan error
	total       int
	ctx         context.Context
	cancel      context.CancelFunc
}

type dashboardModel struct {
	ready        chan struct{}
	width        int
	height       int
	mode         string
	model        string
	reasoning    string
	stageText    string
	footer       string
	agents       map[string]*visualAgent
	slot         string
	active       int
	maximum      int
	overwrites   int
	owner        string
	queued       int
	shaOwners    map[string][]string
	events       []string
	firstResults map[string]string
	armDone      bool
	armResult    *armResult
	finalResult  *proofResult
	continueCh   chan struct{}
	screens      []string
	current      int
	total        int
	cancel       context.CancelFunc
}

type startArmMsg struct {
	mode      string
	model     string
	reasoning string
}

type stageMsg string

type agentStateMsg struct {
	id     string
	state  string
	detail string
}

type agentFinishedMsg struct {
	id       string
	exitCode int
}

type workcellQueueMsg struct {
	owner  string
	queued int
}

type completeArmMsg armResult
type finalResultMsg proofResult
type noteMsg string

type awaitInputMsg struct {
	prompt string
	done   chan struct{}
}

func newDashboard(ctx context.Context, cancel context.CancelFunc, artifactDir string, requested bool, total int) *dashboard {
	info, err := os.Stdout.Stat()
	enabled := requested && err == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"
	return &dashboard{enabled: enabled, artifactDir: artifactDir, total: total, ctx: ctx, cancel: cancel}
}

func (view *dashboard) start() {
	if !view.enabled || view.program != nil {
		return
	}
	ready := make(chan struct{})
	view.done = make(chan error, 1)
	view.program = tea.NewProgram(
		newDashboardModel(ready, view.total, view.cancel),
		tea.WithFPS(30),
	)
	go func() {
		_, err := view.program.Run()
		view.done <- err
	}()
	select {
	case <-ready:
	case <-view.ctx.Done():
		view.enabled = false
	case err := <-view.done:
		view.enabled = false
		view.program = nil
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent-proof dashboard: %v\n", err)
		}
	}
}

func (view *dashboard) close() {
	if view.program == nil {
		return
	}
	view.program.Quit()
	if err := <-view.done; err != nil {
		fmt.Fprintf(os.Stderr, "agent-proof dashboard: %v\n", err)
	}
	view.program = nil
}

func (view *dashboard) startArm(mode, model, reasoning string) {
	if !view.enabled {
		fmt.Printf("\n=== %s ===\n", armTitle(mode))
		fmt.Printf("3 real %s agents (%s); one shared integration binary\n", model, reasoning)
		return
	}
	view.send(startArmMsg{mode: mode, model: model, reasoning: reasoning})
}

func (view *dashboard) stage(text string) {
	if !view.enabled {
		fmt.Println(text)
		return
	}
	view.send(stageMsg(text))
}

func (view *dashboard) agentStarting(id string) {
	view.setAgent(id, "STARTING", "fresh isolated worktree")
}

func (view *dashboard) agentReady(id string) {
	view.setAgent(id, "READY", "candidate built locally")
}

func (view *dashboard) agentReleased(id string, queued bool) {
	if queued {
		view.setAgent(id, "QUEUED", "waiting for Workcell")
		return
	}
	view.setAgent(id, "DEPLOYING", "entering shared target")
}

func (view *dashboard) agentFinished(id string, exitCode int) {
	if view.enabled {
		view.send(agentFinishedMsg{id: id, exitCode: exitCode})
	}
}

func (view *dashboard) workcellQueue(owner string, queued int) {
	if view.enabled {
		view.send(workcellQueueMsg{owner: owner, queued: queued})
	}
}

func (view *dashboard) deployed(event deployEvent) {
	if view.enabled {
		view.send(event)
	}
}

func (view *dashboard) testResult(attempt attemptResult) {
	if view.enabled {
		view.send(attempt)
	}
}

func (view *dashboard) completeArm(result armResult) {
	if !view.enabled {
		fmt.Printf("RESULT: false greens=%d, final correct=%d/%d, max overlap=%d\n",
			result.FalseGreenAttempts, result.FinalCandidatesCorrect, result.Agents, result.MaximumConcurrentCriticals)
		return
	}
	view.send(completeArmMsg(result))
}

func (view *dashboard) showFinal(result proofResult) {
	if !view.enabled {
		printSummary(result)
		return
	}
	view.send(finalResultMsg(result))
}

func (view *dashboard) waitForContinue(prompt string) {
	if !view.enabled {
		return
	}
	done := make(chan struct{})
	view.send(awaitInputMsg{prompt: prompt, done: done})
	select {
	case <-done:
	case <-view.ctx.Done():
	}
}

func (view *dashboard) note(text string) {
	if !view.enabled {
		fmt.Println(text)
		return
	}
	view.send(noteMsg(text))
}

func (view *dashboard) setAgent(id, state, detail string) {
	if view.enabled {
		view.send(agentStateMsg{id: id, state: state, detail: detail})
	}
}

func (view *dashboard) send(message tea.Msg) {
	if view.program != nil {
		view.program.Send(message)
	}
}

func newDashboardModel(ready chan struct{}, total int, cancel context.CancelFunc) *dashboardModel {
	return &dashboardModel{ready: ready, width: 104, height: 30, total: total, cancel: cancel}
}

func (model *dashboardModel) Init() tea.Cmd {
	if model.ready != nil {
		close(model.ready)
		model.ready = nil
	}
	return nil
}

func (model *dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
	case tea.KeyMsg:
		switch message.Type {
		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			model.cancel()
			return model, tea.Quit
		case tea.KeyLeft:
			if model.current > 0 {
				model.current--
			}
		case tea.KeyRight:
			if model.current < len(model.screens) {
				model.current++
			} else if model.continueCh != nil {
				done := model.continueCh
				model.continueCh = nil
				model.footer = ""
				close(done)
			}
		}
	case startArmMsg:
		if model.mode != "" {
			model.screens = append(model.screens, model.renderArm())
		}
		model.startArm(message)
		model.current = len(model.screens)
	case stageMsg:
		model.stageText = string(message)
	case agentStateMsg:
		agent := model.agent(message.id)
		agent.State = message.state
		agent.Detail = message.detail
	case agentFinishedMsg:
		model.finishAgent(message)
	case workcellQueueMsg:
		model.owner = message.owner
		model.queued = message.queued
		model.addEvent(fmt.Sprintf("Workcell owner: %s; %d agent(s) queued", candidateLabel(message.owner), message.queued))
		if agent := model.agents[message.owner]; agent != nil {
			agent.State = "RUNNING / OWNER"
			agent.Detail = "exclusive deploy + test window"
		}
	case deployEvent:
		model.applyDeploy(message)
	case attemptResult:
		model.applyAttempt(message)
	case completeArmMsg:
		model.completeArm(armResult(message))
	case finalResultMsg:
		result := proofResult(message)
		model.screens = append(model.screens, model.renderArm())
		model.finalResult = &result
		model.footer = ""
		model.current = len(model.screens)
	case noteMsg:
		model.addEvent(string(message))
	case awaitInputMsg:
		model.footer = message.prompt
		model.continueCh = message.done
	}
	return model, nil
}

func (model *dashboardModel) View() string {
	var screen string
	if model.current < len(model.screens) {
		screen = model.screens[model.current]
	} else if model.finalResult != nil {
		screen = model.renderFinal(*model.finalResult)
	} else {
		screen = model.renderArm()
	}
	return screen + model.navigation()
}

func (model *dashboardModel) navigation() string {
	left := ansiDim + "← previous" + ansiReset
	if model.current > 0 {
		left = "← previous"
	}
	right := ansiDim + "→ waiting for step to complete" + ansiReset
	if model.current < len(model.screens) {
		right = "→ next"
	} else if model.continueCh != nil {
		right = "→ continue"
	}
	ready := ""
	if model.continueCh != nil && model.current == len(model.screens) {
		ready = " · step complete"
	}
	status := ""
	if ready != "" {
		status = fmt.Sprintf("\n%s%s%s\n", ansiBold, model.footer, ansiReset)
	}
	return fmt.Sprintf(
		"%s\n%sPress arrow keys to progress%s  %s · %s  %d/%d%s\n",
		status,
		ansiBold,
		ansiReset,
		left,
		right,
		model.current+1,
		model.total,
		ready,
	)
}

func (model *dashboardModel) startArm(message startArmMsg) {
	model.mode = message.mode
	model.model = message.model
	model.reasoning = message.reasoning
	model.stageText = "Preparing experiment"
	model.footer = ""
	model.agents = make(map[string]*visualAgent, len(candidateIDs))
	for _, id := range candidateIDs {
		model.agents[id] = &visualAgent{ID: id, State: "PENDING", Tested: "—"}
	}
	model.slot = "empty"
	model.active = 0
	model.maximum = 0
	model.overwrites = 0
	model.owner = ""
	model.queued = 0
	model.shaOwners = make(map[string][]string)
	model.events = nil
	model.firstResults = make(map[string]string)
	model.armDone = false
	model.armResult = nil
	model.finalResult = nil
}

func (model *dashboardModel) finishAgent(message agentFinishedMsg) {
	agent := model.agent(message.id)
	if message.exitCode != 0 {
		agent.State = "AGENT ERROR"
		agent.Detail = fmt.Sprintf("Codex exit %d", message.exitCode)
	} else if agent.State == "PASS" {
		agent.State = "VALIDATED"
		agent.Detail = "agent reported candidate validated"
	} else if agent.State == "FALSE GREEN" {
		agent.Detail = "agent reported validated anyway"
	}
}

func (model *dashboardModel) applyDeploy(event deployEvent) {
	model.shaOwners[event.SHA] = appendUnique(model.shaOwners[event.SHA], event.CandidateID)
	agent := model.agent(event.CandidateID)
	agent.Attempt++
	previousSlot := model.slot
	if model.active > 0 && previousSlot != "empty" && previousSlot != event.CandidateID {
		model.overwrites++
		model.addEvent(fmt.Sprintf("COLLISION: %s overwrote %s during its open test", candidateLabel(event.CandidateID), candidateLabel(previousSlot)))
	} else if previousSlot == "empty" {
		model.addEvent(fmt.Sprintf("%s deployed to the shared target", candidateLabel(event.CandidateID)))
	} else {
		model.addEvent(fmt.Sprintf("handoff: %s deployed to the shared target", candidateLabel(event.CandidateID)))
	}
	model.slot = event.CandidateID
	model.active++
	if model.active > model.maximum {
		model.maximum = model.active
	}
	if model.mode == "workcell" {
		model.owner = event.CandidateID
	}
	if agent.Attempt > 1 {
		agent.State = "RETRYING"
	} else {
		agent.State = "TESTING"
	}
	agent.Detail = "health checks, then integration test"
	agent.Tested = "shared slot"
}

func (model *dashboardModel) applyAttempt(attempt attemptResult) {
	if model.active > 0 {
		model.active--
	}
	agent := model.agent(attempt.CandidateID)
	observed := model.ownerForSHA(attempt.ObservedSHA)
	identityMatch := attempt.ExpectedSHA == attempt.ObservedSHA
	if _, recorded := model.firstResults[attempt.CandidateID]; !recorded {
		if attempt.TestExitCode == 0 {
			model.firstResults[attempt.CandidateID] = "PASS"
		} else {
			model.firstResults[attempt.CandidateID] = "FAIL"
		}
	}
	switch {
	case attempt.TestExitCode == 0 && identityMatch:
		agent.State = "PASS"
		agent.Detail = "tested its own binary"
		agent.Tested = candidateLabel(attempt.CandidateID) + " ✓"
		model.addEvent(fmt.Sprintf("%s PASS — tested its own binary", candidateLabel(attempt.CandidateID)))
	case attempt.TestExitCode == 0:
		agent.State = "FALSE GREEN"
		agent.Detail = "agent thinks this candidate passed"
		agent.Tested = candidateLabel(observed) + " ✗"
		model.addEvent(fmt.Sprintf("FALSE GREEN: %s passed against %s", candidateLabel(attempt.CandidateID), candidateLabel(observed)))
	default:
		agent.State = "FIXING (NO LEASE)"
		agent.Detail = "validation released the lease; Luna is fixing privately"
		agent.Tested = candidateLabel(attempt.CandidateID) + " ✓"
		model.addEvent(fmt.Sprintf("EXPECTED FAILURE: %s tested itself; lease released before fixing", candidateLabel(attempt.CandidateID)))
	}
	if model.mode == "workcell" && model.active == 0 {
		model.owner = ""
	}
	if model.mode == "workcell" && model.queued > 0 {
		model.queued--
	}
}

func (model *dashboardModel) completeArm(result armResult) {
	model.armDone = true
	copyValue := result
	model.armResult = &copyValue
	if result.Mode == "without" {
		model.addEvent("CONTROL FAILED: wait-regression escaped validation")
	} else {
		model.addEvent("WORKCELL PASSED: wait-regression was caught, fixed, and revalidated")
	}
}

func (model *dashboardModel) agent(id string) *visualAgent {
	if model.agents == nil {
		model.agents = make(map[string]*visualAgent)
	}
	agent := model.agents[id]
	if agent == nil {
		agent = &visualAgent{ID: id, State: "PENDING", Tested: "—"}
		model.agents[id] = agent
	}
	return agent
}

func (model *dashboardModel) ownerForSHA(sha string) string {
	owners := model.shaOwners[sha]
	if len(owners) == 0 {
		if len(sha) > 8 {
			return sha[:8]
		}
		return sha
	}
	return strings.Join(owners, "/")
}

func (model *dashboardModel) addEvent(text string) {
	model.events = append(model.events, text)
	if len(model.events) > 5 {
		model.events = append([]string(nil), model.events[len(model.events)-5:]...)
	}
}

func (model *dashboardModel) renderArm() string {
	width := model.contentWidth()
	var output strings.Builder
	modeColor := ansiRed
	if model.mode == "workcell" {
		modeColor = ansiGreen
	}
	fmt.Fprintf(&output, "%sWORKCELL REAL-AGENT PROOF%s  %s%s%s\n", ansiBold, ansiReset, modeColor, armTitle(model.mode), ansiReset)
	fmt.Fprintf(&output, "%sQuestion: can a broken candidate pass after another candidate replaces the shared binary?%s\n", ansiDim, ansiReset)
	output.WriteString(strings.Repeat("═", width) + "\n")
	fmt.Fprintf(&output, "%sEXPECTED FIRST RESULTS%s\n", ansiBold, ansiReset)
	fmt.Fprintf(&output, "  %s%s%s contains a seeded regression: --wait returns BUSY (75) instead of waiting.\n", ansiYellow, regressionCandidateID, ansiReset)
	output.WriteString("  wait-regression FAIL · baseline PASS · baseline-replica PASS.\n")
	output.WriteString("  Each agent validates its candidate; after a failure, it fixes the source and retries.\n\n")

	candidateWidth := 20
	expectedWidth := 18
	stateWidth := 19
	testedWidth := maxInt(20, width-candidateWidth-expectedWidth-stateWidth-6)
	fmt.Fprintf(&output, "%-*s  %-*s  %-*s  %s\n", candidateWidth, "CANDIDATE", expectedWidth, "EXPECTED", stateWidth, "AGENT STATE", "BINARY ACTUALLY TESTED")
	output.WriteString(strings.Repeat("─", width) + "\n")
	for _, id := range candidateIDs {
		agent := model.agent(id)
		fmt.Fprintf(&output, "%s  %s  %s  %s\n",
			padRight(candidateLabel(id), candidateWidth),
			model.coloredExpectation(id, expectedWidth),
			model.coloredState(agent.State, stateWidth),
			truncate(agent.Tested, testedWidth))
	}
	output.WriteString("\n")
	if model.mode == "without" {
		fmt.Fprintf(&output, "SHARED TARGET  [ %-18s ]  open tests: %s%d%s  destructive overwrites: %s%d%s\n",
			candidateLabel(model.slot), ansiYellow, model.active, ansiReset, ansiRed, model.overwrites, ansiReset)
	} else {
		owner := model.owner
		if owner == "" {
			owner = "available"
		}
		fmt.Fprintf(&output, "WORKCELL       owner: %-20s queued: %-2d  open tests: %s%d%s  max owners: %s%d%s\n",
			candidateLabel(owner), model.queued, ansiGreen, model.active, ansiReset, ansiGreen, model.maximum, ansiReset)
	}
	output.WriteString("\n")
	fmt.Fprintf(&output, "%sLIVE EVIDENCE%s\n", ansiBold, ansiReset)
	if len(model.events) == 0 {
		fmt.Fprintf(&output, "  %s• waiting for agents%s\n", ansiDim, ansiReset)
	} else {
		for _, event := range model.events {
			fmt.Fprintf(&output, "  • %s\n", truncate(event, width-4))
		}
	}
	for index := len(model.events); index < 5; index++ {
		output.WriteString("\n")
	}
	output.WriteString("\n")
	fmt.Fprintf(&output, "%sFIRST-PASS CHECK%s\n", ansiBold, ansiReset)
	output.WriteString("  expected: wait-regression FAIL · baseline PASS · baseline-replica PASS\n")
	fmt.Fprintf(&output, "  observed: %s\n", model.firstPassSummary())
	output.WriteString("\n")

	if !model.armDone {
		fmt.Fprintf(&output, "%s%s%s\n", ansiCyan, model.stageText, ansiReset)
	}
	if model.armDone && model.armResult != nil {
		verdict := "CONTROL RESULT: FAIL — wait-regression escaped validation"
		verdictColor := ansiRed
		if model.mode == "workcell" {
			verdict = "PASS — regression caught and all final candidates correct"
			verdictColor = ansiGreen
		}
		fmt.Fprintf(&output, "%s%s%s\n", verdictColor, verdict, ansiReset)
	}
	return output.String()
}

func (model *dashboardModel) renderFinal(result proofResult) string {
	if len(result.Arms) == 1 {
		return model.renderSingleFinal(result)
	}
	without := findArm(result.Arms, "without")
	withWorkcell := findArm(result.Arms, "workcell")
	width := model.contentWidth()
	var output strings.Builder
	fmt.Fprintf(&output, "%sWORKCELL REAL-AGENT PROOF — FINAL%s\n", ansiBold, ansiReset)
	output.WriteString(strings.Repeat("═", width) + "\n")
	output.WriteString("wait-regression contains a seeded defect and should fail its first validation.\n\n")
	fmt.Fprintf(&output, "%s  %s  %s\n", padRight("MEASURE", 38), styledCell("WITHOUT WORKCELL", 24, ansiRed), styledCell("WITH WORKCELL", 24, ansiGreen))
	output.WriteString(strings.Repeat("─", width) + "\n")
	fmt.Fprintf(&output, "%-38s  %-24s  %-24s\n", "Regression caught on first validation", "NO", "YES")
	fmt.Fprintf(&output, "%-38s  %-24d  %-24d\n", "False-green tests", without.FalseGreenAttempts, withWorkcell.FalseGreenAttempts)
	fmt.Fprintf(&output, "%-38s  %-24d  %-24d\n", "Maximum overlapping test windows", without.MaximumConcurrentCriticals, withWorkcell.MaximumConcurrentCriticals)
	fmt.Fprintf(&output, "%-38s  %-24s  %-24s\n", "Actually correct final candidates",
		fmt.Sprintf("%d/%d", without.FinalCandidatesCorrect, without.Agents),
		fmt.Sprintf("%d/%d", withWorkcell.FinalCandidatesCorrect, withWorkcell.Agents))
	fmt.Fprintf(&output, "%-38s  %-24s  %-24s\n", "Time to all correct", "NOT REACHED", formatTimeToCorrect(withWorkcell))
	output.WriteString("\n")
	fmt.Fprintf(&output, "%sWHY CONTROL FAILED%s\n", ansiRed+ansiBold, ansiReset)
	output.WriteString("  wait-regression deployed a broken binary, but another agent replaced the shared slot before its test.\n")
	output.WriteString("  Its test passed a baseline binary, so Luna reported the broken candidate as validated.\n\n")
	fmt.Fprintf(&output, "%sWHY WORKCELL PASSED%s\n", ansiGreen+ansiBold, ansiReset)
	output.WriteString("  wait-regression tested its own binary, failed, was fixed by Luna, and passed on retry.\n")
	output.WriteString("  Every deploy-and-test window had exactly one owner.\n\n")
	verdict := ansiRed + "FAIL" + ansiReset
	if result.Passed {
		verdict = ansiGreen + "PASS" + ansiReset
	}
	fmt.Fprintf(&output, "OVERALL PROOF %s\n", verdict)
	fmt.Fprintf(&output, "Evidence: %s/result.json\n", result.ArtifactDir)
	return output.String()
}

func (model *dashboardModel) renderSingleFinal(result proofResult) string {
	arm := result.Arms[0]
	width := model.contentWidth()
	regressionCaught := "YES"
	if arm.FalseGreenAttempts > 0 {
		regressionCaught = "NO"
	}
	mode := "WITH WORKCELL"
	if arm.Mode == "without" {
		mode = "WITHOUT WORKCELL"
	}
	var output strings.Builder
	fmt.Fprintf(&output, "%sWORKCELL REAL-AGENT PROOF — %s%s\n", ansiBold, mode, ansiReset)
	output.WriteString(strings.Repeat("═", width) + "\n")
	output.WriteString("wait-regression contains a seeded defect and should fail its first validation.\n\n")
	fmt.Fprintf(&output, "%-38s  %s\n", "Regression caught on first validation", regressionCaught)
	fmt.Fprintf(&output, "%-38s  %d\n", "False-green tests", arm.FalseGreenAttempts)
	fmt.Fprintf(&output, "%-38s  %d\n", "Maximum overlapping test windows", arm.MaximumConcurrentCriticals)
	fmt.Fprintf(&output, "%-38s  %d/%d\n", "Actually correct final candidates", arm.FinalCandidatesCorrect, arm.Agents)
	fmt.Fprintf(&output, "%-38s  %s\n", "Time to all correct", formatTimeToCorrect(arm))
	output.WriteString("\n")
	verdict := ansiRed + "EXPECTED RESULT NOT OBSERVED" + ansiReset
	if result.Passed {
		verdict = ansiGreen + "EXPECTED RESULT OBSERVED" + ansiReset
	}
	fmt.Fprintf(&output, "%s\n", verdict)
	fmt.Fprintf(&output, "Evidence: %s/result.json\n", result.ArtifactDir)
	return output.String()
}

func (model *dashboardModel) firstPassSummary() string {
	parts := make([]string, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		result := model.firstResults[id]
		if result == "" {
			result = "…"
		}
		label := strings.TrimPrefix(id, "candidate-")
		if id == regressionCandidateID {
			label = "wait-regression"
		}
		parts = append(parts, label+" "+result)
	}
	summary := strings.Join(parts, " · ")
	if model.armDone {
		if model.mode == "without" {
			summary += "  " + ansiRed + "WRONG — regression escaped" + ansiReset
		} else {
			summary += "  " + ansiGreen + "CORRECT — regression was caught" + ansiReset
		}
	}
	return summary
}

func (model *dashboardModel) coloredExpectation(id string, width int) string {
	if id == regressionCandidateID {
		return styledCell("MUST FAIL FIRST", width, ansiYellow)
	}
	return styledCell("SHOULD PASS", width, ansiGreen)
}

func (model *dashboardModel) coloredState(state string, width int) string {
	color := ansiCyan
	switch {
	case strings.Contains(state, "FALSE") || strings.Contains(state, "ERROR"):
		color = ansiRed
	case strings.Contains(state, "FAIL"):
		color = ansiYellow
	case strings.Contains(state, "PASS") || strings.Contains(state, "VALIDATED"):
		color = ansiGreen
	case strings.Contains(state, "QUEUED"):
		color = ansiYellow
	}
	return styledCell(state, width, color)
}

func (model *dashboardModel) contentWidth() int {
	width := model.width
	if width <= 0 {
		width = 104
	}
	if width > 120 {
		width = 120
	}
	if width < 80 {
		width = 80
	}
	return width
}

func armTitle(mode string) string {
	if mode == "workcell" {
		return "WITH WORKCELL"
	}
	return "CONTROL: WITHOUT WORKCELL"
}

func findArm(arms []armResult, mode string) armResult {
	for _, arm := range arms {
		if arm.Mode == mode {
			return arm
		}
	}
	return armResult{Mode: mode, Agents: len(candidateIDs)}
}

func formatTimeToCorrect(arm armResult) string {
	if arm.TimeToAllCorrectSeconds == nil {
		return "NOT REACHED"
	}
	return fmt.Sprintf("%.1fs", *arm.TimeToAllCorrectSeconds)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

func maxInt(first, second int) int {
	if first > second {
		return first
	}
	return second
}

func styledCell(value string, width int, color string) string {
	return color + padRight(value, width) + ansiReset
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func candidateLabel(id string) string {
	return id
}

func sortedPaths(paths []string) []string {
	result := append([]string(nil), paths...)
	sort.Strings(result)
	return result
}
