package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

// newConfirmTestModel returns a model with the default confirmation policy.
func newConfirmTestModel(t *testing.T) *chatModel {
	t.Helper()
	p, err := newConfirmPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(*chatModel)
	m.confirm = p
	m.turnID = 7
	return m
}

func openConfirm(t *testing.T, m *chatModel) chan confirmReply {
	t.Helper()
	reply := make(chan confirmReply, 1)
	updated, _ := m.Update(confirmRequest{
		turn:    7,
		name:    "bash",
		summary: "$ rm -rf /tmp/x",
		detail:  "rm -rf /tmp/x\n# second line",
		reply:   reply,
	})
	got := updated.(*chatModel)
	if got.pending == nil {
		t.Fatal("confirmRequest did not create a pending prompt")
	}
	if got.state != tuiConfirming {
		t.Errorf("state = %v, want tuiConfirming", got.state)
	}
	return reply
}

func keyRune(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestConfirmOnce(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("y"))
	m = updated.(*chatModel)
	if m.pending != nil {
		t.Error("pending prompt was not cleared")
	}
	if m.state != tuiToolCall {
		t.Errorf("state = %v, want tuiToolCall", m.state)
	}
	if got := <-reply; got.decision != confirmOnce {
		t.Errorf("decision = %v, want confirmOnce", got.decision)
	}
}

func TestConfirmDeny(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("n"))
	m = updated.(*chatModel)
	if got := <-reply; got.decision != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got.decision)
	}
	if m.state != tuiProcessing {
		t.Errorf("state = %v, want tuiProcessing (model may continue)", m.state)
	}
}

func TestConfirmAlwaysPersistsForSession(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("A")) // uppercase must also work
	m = updated.(*chatModel)
	if got := <-reply; got.decision != confirmAlways {
		t.Errorf("decision = %v, want confirmAlways", got.decision)
	}
	if m.confirm.needs("bash", `{"command":"rm -rf /"}`) {
		t.Error("bash should be always-allowed after choosing a")
	}
	if !m.confirm.needs("write", `{"filepath":"a.go","content":"x"}`) {
		t.Error("always-allow must not leak to other tools")
	}
}

func TestConfirmEscapeDeniesAndCancels(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(*chatModel)
	if got := <-reply; got.decision != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got.decision)
	}
	if m.pending != nil {
		t.Error("pending prompt was not cleared")
	}
	if m.state != tuiIdle {
		t.Errorf("state = %v, want idle after Esc", m.state)
	}
}

func TestConfirmStaleTurnIsRejected(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := make(chan confirmReply, 1)

	updated, _ := m.Update(confirmRequest{
		turn:    6, // previous turn
		name:    "bash",
		summary: "$ ls",
		reply:   reply,
	})
	m = updated.(*chatModel)
	if m.pending != nil {
		t.Error("stale confirmRequest must not open a prompt")
	}
	if got := <-reply; got.decision != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got.decision)
	}
}

func TestCancelTurnDeniesPendingConfirmation(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	m.cancelTurn()
	if m.pending != nil {
		t.Error("cancelTurn did not clear the pending prompt")
	}
	if got := <-reply; got.decision != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got.decision)
	}
}

func TestConfirmIgnoresOtherKeys(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("hello"))
	m = updated.(*chatModel)
	if m.pending == nil {
		t.Fatal("unrelated input dismissed the prompt")
	}
	select {
	case got := <-reply:
		t.Fatalf("unrelated input produced decision %v", got)
	default:
	}
}

func TestConfirmBoxRenders(t *testing.T) {
	m := newConfirmTestModel(t)
	openConfirm(t, m)

	view := m.View()
	for _, want := range []string{"Confirm bash", "$ rm -rf /tmp/x", "# second line", "[y] allow once", "[a] always", "[n] deny", "[s] suggest", "[Esc] stop"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirmation box missing %q:\n%s", want, view)
		}
	}
}

func TestConfirmClickChoices(t *testing.T) {
	tests := []struct {
		label string
		want  confirmDecision
	}{
		{"[y] allow once", confirmOnce},
		{"[a] always", confirmAlways},
		{"[n] deny", confirmDeny},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			m := newConfirmTestModel(t)
			reply := openConfirm(t, m)

			row, ok := m.confirmOptionRow()
			if !ok {
				t.Fatal("option row is not clickable with the test width")
			}
			x := 4 + strings.Index(confirmPromptHint(false), tc.label)
			updated, _ := m.Update(tea.MouseMsg{
				X: x, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
			})
			m = updated.(*chatModel)
			if m.pending != nil {
				t.Error("clicking the option did not resolve the prompt")
			}
			if got := <-reply; got.decision != tc.want {
				t.Errorf("decision = %v, want %v", got.decision, tc.want)
			}
		})
	}
}

func TestConfirmClickSuggestStartsSuggestion(t *testing.T) {
	m := newConfirmTestModel(t)
	openConfirm(t, m)

	row, ok := m.confirmOptionRow()
	if !ok {
		t.Fatal("option row is not clickable with the test width")
	}
	x := 4 + strings.Index(confirmPromptHint(false), "[s] suggest")
	updated, _ := m.Update(tea.MouseMsg{X: x, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(*chatModel)

	if !m.suggesting {
		t.Error("clicking [s] did not open the suggestion box")
	}
	if m.pending == nil {
		t.Error("suggestion box must keep the confirmation pending")
	}
}

// TestConfirmSuggestTypesFeedback covers the "s" path: the user denies the call
// but hands the model an instruction, which must reach it as the tool result.
func TestConfirmSuggestTypesFeedback(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("s"))
	m = updated.(*chatModel)
	if !m.suggesting {
		t.Fatal("s did not open the suggestion box")
	}
	if m.pending == nil {
		t.Fatal("suggestion box must keep the confirmation pending")
	}

	for _, r := range "use read instead" {
		updated, _ = m.Update(keyRune(string(r)))
		m = updated.(*chatModel)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*chatModel)

	got := <-reply
	if got.decision != confirmSuggest {
		t.Errorf("decision = %v, want confirmSuggest", got.decision)
	}
	if got.feedback != "use read instead" {
		t.Errorf("feedback = %q, want %q", got.feedback, "use read instead")
	}
	if m.pending != nil || m.suggesting {
		t.Error("prompt state was not cleared")
	}
	if m.state != tuiProcessing {
		t.Errorf("state = %v, want tuiProcessing (model may retry)", m.state)
	}
	if m.confirm.needs("bash", `{"command":"rm -rf /"}`) == false {
		t.Error("a suggestion must not enable always-allow")
	}
}

func TestConfirmSuggestEmptyEnterDenies(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("s"))
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*chatModel)

	got := <-reply
	if got.decision != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny for an empty suggestion", got.decision)
	}
	if got.feedback != "" {
		t.Errorf("feedback = %q, want empty", got.feedback)
	}
	if m.state != tuiProcessing {
		t.Errorf("state = %v, want tuiProcessing", m.state)
	}
}

func TestConfirmSuggestEscapeReturnsToPrompt(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("s"))
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(*chatModel)

	if m.suggesting {
		t.Error("Esc did not leave the suggestion box")
	}
	if m.pending == nil {
		t.Fatal("Esc must return to the prompt, not answer it")
	}
	select {
	case got := <-reply:
		t.Fatalf("Esc produced decision %v", got)
	default:
	}

	// The prompt still answers normally afterwards.
	updated, _ = m.Update(keyRune("y"))
	m = updated.(*chatModel)
	if got := <-reply; got.decision != confirmOnce {
		t.Errorf("decision = %v, want confirmOnce", got.decision)
	}
	if m.pending != nil {
		t.Error("prompt was not cleared after answering")
	}
}

// TestConfirmSuggestRestoresDraft guards the user's half-written message: the
// suggestion box borrows the input box, and closing it must put the draft back.
func TestConfirmSuggestRestoresDraft(t *testing.T) {
	m := newConfirmTestModel(t)
	m.input.SetValue("a draft in progress")
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("s"))
	m = updated.(*chatModel)
	if got := m.input.Value(); got != "" {
		t.Errorf("input value = %q, want it cleared for the suggestion", got)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(*chatModel)
	if got := m.input.Value(); got != "a draft in progress" {
		t.Errorf("draft = %q, want it restored", got)
	}

	// Submitting a suggestion restores it as well.
	updated, _ = m.Update(keyRune("s"))
	m = updated.(*chatModel)
	updated, _ = m.Update(keyRune("x"))
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*chatModel)
	if got := <-reply; got.decision != confirmSuggest {
		t.Errorf("decision = %v, want confirmSuggest", got.decision)
	}
	if got := m.input.Value(); got != "a draft in progress" {
		t.Errorf("draft after submitting = %q, want it restored", got)
	}
}

func TestDeniedToolResult(t *testing.T) {
	tests := []struct {
		name     string
		reply    confirmReply
		want     string
		wantSumm string
	}{
		{
			name:     "plain denial",
			reply:    confirmReply{decision: confirmDeny},
			want:     "User denied this tool call.",
			wantSumm: deniedSummary,
		},
		{
			name:     "suggestion becomes guidance",
			reply:    confirmReply{decision: confirmSuggest, feedback: "  use read  "},
			want:     "The user rejected this tool call and asked for a different approach: use read",
			wantSumm: "denied by user · use read",
		},
		{
			name:     "blank suggestion degrades to a denial",
			reply:    confirmReply{decision: confirmSuggest, feedback: "   "},
			want:     "User denied this tool call.",
			wantSumm: deniedSummary,
		},
		{
			name:     "no confirmation ran",
			reply:    confirmReply{},
			want:     "User denied this tool call.",
			wantSumm: deniedSummary,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deniedToolResult(tc.reply); got != tc.want {
				t.Errorf("deniedToolResult() = %q, want %q", got, tc.want)
			}
			if got := deniedSummaryFor(tc.reply); got != tc.wantSumm {
				t.Errorf("deniedSummaryFor() = %q, want %q", got, tc.wantSumm)
			}
		})
	}
}

func TestConfirmSuggestBoxRenders(t *testing.T) {
	m := newConfirmTestModel(t)
	openConfirm(t, m)

	if view := m.View(); !strings.Contains(view, "[s] suggest") {
		t.Errorf("confirmation box missing the suggest key:\n%s", view)
	}

	updated, _ := m.Update(keyRune("s"))
	m = updated.(*chatModel)
	updated, _ = m.Update(keyRune("x"))
	m = updated.(*chatModel)
	view := m.View()
	for _, want := range []string{"tell the model what to do instead", "[Enter] send", "x"} {
		if !strings.Contains(view, want) {
			t.Errorf("suggestion box missing %q:\n%s", want, view)
		}
	}
}
