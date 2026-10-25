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

func openConfirm(t *testing.T, m *chatModel) chan confirmDecision {
	t.Helper()
	reply := make(chan confirmDecision, 1)
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
	if got := <-reply; got != confirmOnce {
		t.Errorf("decision = %v, want confirmOnce", got)
	}
}

func TestConfirmDeny(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	updated, _ := m.Update(keyRune("n"))
	m = updated.(*chatModel)
	if got := <-reply; got != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got)
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
	if got := <-reply; got != confirmAlways {
		t.Errorf("decision = %v, want confirmAlways", got)
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
	if got := <-reply; got != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got)
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
	reply := make(chan confirmDecision, 1)

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
	if got := <-reply; got != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got)
	}
}

func TestCancelTurnDeniesPendingConfirmation(t *testing.T) {
	m := newConfirmTestModel(t)
	reply := openConfirm(t, m)

	m.cancelTurn()
	if m.pending != nil {
		t.Error("cancelTurn did not clear the pending prompt")
	}
	if got := <-reply; got != confirmDeny {
		t.Errorf("decision = %v, want confirmDeny", got)
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
	for _, want := range []string{"Confirm bash", "$ rm -rf /tmp/x", "# second line", "[y] allow once", "[a] allow for this session", "[n] deny"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirmation box missing %q:\n%s", want, view)
		}
	}
}
