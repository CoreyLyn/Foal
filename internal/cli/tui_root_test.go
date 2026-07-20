package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func updateRootKeys(t *testing.T, model rootModel, msgs ...tea.KeyPressMsg) rootModel {
	t.Helper()
	for _, msg := range msgs {
		next, _ := model.Update(msg)
		root, ok := next.(rootModel)
		if !ok {
			t.Fatalf("Update returned %T, want rootModel", next)
		}
		model = root
	}
	return model
}

func TestRootModelInitialContentShowsMainMenu(t *testing.T) {
	content := newRootModel().content()

	for _, want := range []string{
		"https://github.com/CoreyLyn/Foal",
		"Safe, preview-first cleanup for Windows.",
		"Foal main menu",
		"> Clean",
		"  Uninstall",
		"  Analyze",
		"  Status",
		"  Extensions",
		"j/k or up/down: move",
		"enter: open",
		"q: quit",
		"read-only navigation shell",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("main menu content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "\x1b[") {
		t.Fatalf("menu content must stay plain text without escape sequences:\n%q", content)
	}
}

func TestRootModelNavigationMovesSelection(t *testing.T) {
	model := updateRootKeys(t, newRootModel(), tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !strings.Contains(model.content(), "> Uninstall") {
		t.Fatalf("j should move selection to Uninstall:\n%s", model.content())
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if !strings.Contains(model.content(), "> Clean") {
		t.Fatalf("k should return selection to Clean:\n%s", model.content())
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if !strings.Contains(model.content(), "> Uninstall") {
		t.Fatalf("down arrow should move selection to Uninstall:\n%s", model.content())
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	if !strings.Contains(model.content(), "> Clean") {
		t.Fatalf("up arrow should return selection to Clean:\n%s", model.content())
	}
}

func TestRootModelQuitAndInterruptKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.KeyPressMsg
		want tea.Msg
	}{
		{name: "q quits", msg: tea.KeyPressMsg{Code: 'q', Text: "q"}, want: tea.QuitMsg{}},
		{name: "esc quits", msg: tea.KeyPressMsg{Code: tea.KeyEscape}, want: tea.QuitMsg{}},
		{name: "ctrl+c interrupts", msg: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, want: tea.InterruptMsg{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cmd := newRootModel().Update(tc.msg)
			if cmd == nil {
				t.Fatalf("%s returned nil cmd", tc.msg.String())
			}
			got := cmd()
			if got != tc.want {
				t.Fatalf("cmd() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestRootModelViewUsesAltScreen(t *testing.T) {
	if view := newRootModel().View(); !view.AltScreen {
		t.Fatal("root model view must declare the alternate screen")
	}
}

func TestRootModelUnknownKeyShowsHint(t *testing.T) {
	model := updateRootKeys(t, newRootModel(), tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !strings.Contains(model.content(), "Unknown key. Use j/k, up/down, enter, or q.") {
		t.Fatalf("unknown key should show the menu hint:\n%s", model.content())
	}
}

func TestRootModelPlaceholderSelectionsAreNonDestructive(t *testing.T) {
	disableHistoryRecording(t)
	originalExecute := executeClean
	executeClean = func(ctx context.Context, opts clean.Options) clean.Result {
		t.Fatal("main menu selection must not execute cleanup")
		return clean.Result{}
	}
	t.Cleanup(func() { executeClean = originalExecute })

	for _, tc := range []struct {
		name      string
		downs     int
		wantTitle string
		wantBody  string
	}{
		{name: "extensions", downs: 5, wantTitle: "Extensions", wantBody: "Future read-only views are not built in this slice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := newRootModel()
			for i := 0; i < tc.downs; i++ {
				model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
			}
			model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

			content := model.content()
			for _, want := range []string{tc.wantTitle, tc.wantBody, "No files were changed.", "Foal main menu"} {
				if !strings.Contains(content, want) {
					t.Fatalf("content missing %q:\n%s", want, content)
				}
			}
			for _, forbidden := range []string{"Execution complete", "Deleted:", "Run uninstaller", "Stop process", "Delete leftover"} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("content contains destructive wording %q:\n%s", forbidden, content)
				}
			}
		})
	}
}

func TestRootModelAnalyzeOpensViewer(t *testing.T) {
	model := newRootModel()
	// Navigate to Analyze (position 2)
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	// Should be on viewer screen
	if model.screen != screenViewer {
		t.Fatal("expected screenViewer after entering Analyze")
	}

	content := model.content()
	for _, want := range []string{"Analyze TUI", "read-only"} {
		if !strings.Contains(content, want) {
			t.Fatalf("Analyze viewer missing %q:\n%s", want, content)
		}
	}
}

func TestRootModelAnalyzeShowsPathEditHint(t *testing.T) {
	model := newRootModel()
	// Navigate to Analyze
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	content := model.content()
	if !strings.Contains(content, "e edit path") {
		t.Fatalf("Analyze viewer missing edit path hint:\n%s", content)
	}
	if !strings.Contains(content, "no cleanup or deletion") {
		t.Fatalf("Analyze viewer missing read-only reminder:\n%s", content)
	}
}

func TestInteractiveShellPrintsClosingLineAfterProgramEnds(t *testing.T) {
	originalRun := runMenuProgram
	runMenuProgram = func(model tea.Model, input io.Reader, output io.Writer) (tea.Model, error) {
		return model, nil
	}
	t.Cleanup(func() { runMenuProgram = originalRun })

	var stdout, stderr bytes.Buffer
	code := RunInvocation(Invocation{
		ExecutableName:            "foal",
		InteractiveTerminal:       true,
		OutputInteractiveTerminal: true,
		Input:                     strings.NewReader(""),
	}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("RunInvocation returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Foal main menu closed.") {
		t.Fatalf("stdout missing closing line:\n%s", stdout.String())
	}
}

func TestInteractiveShellInterruptedReturnsInterruptExitCode(t *testing.T) {
	originalRun := runMenuProgram
	runMenuProgram = func(model tea.Model, input io.Reader, output io.Writer) (tea.Model, error) {
		return model, tea.ErrInterrupted
	}
	t.Cleanup(func() { runMenuProgram = originalRun })

	var stdout, stderr bytes.Buffer
	code := RunInvocation(Invocation{
		ExecutableName:            "foal",
		InteractiveTerminal:       true,
		OutputInteractiveTerminal: true,
		Input:                     strings.NewReader(""),
	}, &stdout, &stderr)

	if code != exitInterrupted {
		t.Fatalf("RunInvocation returned %d, want %d", code, exitInterrupted)
	}
	if strings.Contains(stdout.String(), "Foal main menu closed.") {
		t.Fatalf("interrupted shell must not print the closing line:\n%s", stdout.String())
	}
}

func TestStylizedFramePreservesPlainFragments(t *testing.T) {
	plain := newRootModel().content()
	styled := stylizeFrame(plain)

	if styled == plain {
		t.Fatal("stylized frame should decorate the menu")
	}
	for _, want := range []string{
		"> Clean",
		"Foal main menu",
		"https://github.com/CoreyLyn/Foal",
		"Hints: j/k or up/down: move | enter: open | q: quit",
	} {
		if !strings.Contains(styled, want) {
			t.Fatalf("stylized frame must keep plain fragment %q:\n%q", want, styled)
		}
	}
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("plain content must stay free of escape sequences:\n%q", plain)
	}
}

func TestViewerModelAnalyzePathEditing(t *testing.T) {
	vm := newViewerModel("analyze", 80, 24)
	if vm.command != "analyze" {
		t.Fatal("expected analyze command")
	}
	if vm.editingPath {
		t.Fatal("should not start in edit mode")
	}

	// Enter edit mode
	cmd := vm.handleKey("e")
	if cmd != nil {
		t.Fatal("unexpected cmd from e key")
	}
	if !vm.editingPath {
		t.Fatal("should be in edit mode after e")
	}

	// Type some characters
	vm.handleKey("c")
	vm.handleKey(":")
	vm.handleKey(`\`)
	vm.handleKey("t")
	vm.handleKey("e")
	vm.handleKey("s")
	vm.handleKey("t")

	if vm.analyzePath != `c:\test` {
		t.Fatalf("path edit failed, got %q", vm.analyzePath)
	}

	// Cancel edit
	cmd = vm.handleKey("esc")
	if cmd != nil {
		t.Fatal("unexpected cmd from esc")
	}
	if vm.editingPath {
		t.Fatal("should not be in edit mode after esc")
	}
}

func TestViewerModelAnalyzeNoCleanupAffordances(t *testing.T) {
	// Render analyze body and verify no cleanup actions are suggested
	body := renderAnalyzeBody("")

	// Forbidden actionable phrases (not just single words that might appear in safe contexts)
	forbiddenPhrases := []string{
		"delete this", "remove this", "clean up", "reclaim space",
		"execute cleanup", "confirm deletion", "select items", "move to trash",
		"permanent delete", "click to delete", "press to remove",
	}
	for _, phrase := range forbiddenPhrases {
		if strings.Contains(strings.ToLower(body), phrase) {
			t.Fatalf("Analyze body contains forbidden phrase %q:\n%s", phrase, body)
		}
	}

	// Should contain the read-only disclaimer
	if !strings.Contains(body, "read-only") {
		t.Fatalf("Analyze body missing read-only disclaimer:\n%s", body)
	}

	// Should NOT contain any UI language suggesting the user can perform cleanup
	if strings.Contains(body, "[") && strings.Contains(body, "]") && !strings.Contains(body, "+--") {
		// Check for checkbox-like affordances, but ignore ASCII art frames
		t.Fatalf("Analyze body may contain checkbox affordances:\n%s", body)
	}
}

