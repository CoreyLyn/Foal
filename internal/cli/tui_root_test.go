package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
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
	// Titles only — no (command) slug — with description first letters column-aligned.
	for _, forbidden := range []string{"(clean)", "(uninstall)", "(analyze)", "(status)", "(history)", "(future)"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("main menu must not show command slug %q:\n%s", forbidden, content)
		}
	}
	descCols := make([]int, 0, len(mainMenuItems))
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "> ") && !strings.HasPrefix(line, "  ") {
			continue
		}
		// Skip banner/side text and hints that also start with two spaces.
		if !strings.Contains(line, "Clean") && !strings.Contains(line, "Uninstall") &&
			!strings.Contains(line, "Analyze") && !strings.Contains(line, "Status") &&
			!strings.Contains(line, "History") && !strings.Contains(line, "Extensions") {
			continue
		}
		// Description starts after fixed-width title padding.
		trimmedTitle := strings.TrimLeft(line, "> ")
		parts := strings.Fields(trimmedTitle)
		if len(parts) < 2 {
			continue
		}
		// First description word is after title; locate its column on the raw line.
		title := parts[0]
		titleIdx := strings.Index(line, title)
		if titleIdx < 0 {
			continue
		}
		rest := line[titleIdx+len(title):]
		// Skip spaces after title to find description start column.
		spaces := 0
		for spaces < len(rest) && rest[spaces] == ' ' {
			spaces++
		}
		if spaces == 0 {
			continue
		}
		descCols = append(descCols, titleIdx+len(title)+spaces)
	}
	if len(descCols) < 2 {
		t.Fatalf("expected multiple command rows with descriptions:\n%s", content)
	}
	for i := 1; i < len(descCols); i++ {
		if descCols[i] != descCols[0] {
			t.Fatalf("description first letters not column-aligned: cols=%v\n%s", descCols, content)
		}
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

func TestRootModelAnalyzeMenuOpensDriveEntry(t *testing.T) {
	original := listAnalyzeLocalVolumes
	listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
		return []analyze.LocalVolume{{
			Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true,
			Label: "System", FileSystem: "NTFS", HasCapacity: true, TotalBytes: 1000, FreeBytes: 400,
		}}
	}
	t.Cleanup(func() { listAnalyzeLocalVolumes = original })

	model := newRootModel()
	// Navigate to Analyze (position 2)
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)

	if model.screen != screenAnalyzeDrive {
		t.Fatal("expected screenAnalyzeDrive after entering Analyze")
	}
	if cmd == nil {
		t.Fatal("opening Analyze must load volumes")
	}
	loaded, ok := cmd().(analyzeVolumesLoadedMsg)
	if !ok {
		t.Fatalf("load cmd produced %T, want analyzeVolumesLoadedMsg", cmd())
	}
	next, _ = model.Update(loaded)
	model = next.(rootModel)

	content := model.content()
	for _, want := range []string{"Analyze TUI", "Local drive entry", "read-only", "C:"} {
		if !strings.Contains(content, want) {
			t.Fatalf("Analyze drive entry missing %q: %s", want, content)
		}
	}
}

func TestAnalyzeViewerRendersCoreHumanReportFields(t *testing.T) {
	// The viewer uses renderAnalyzeBody, so we can test that directly
	body := renderAnalyzeBody("")

	// Should show status, bytes, and top children
	for _, want := range []string{"Status:", "Totals:", "bytes", "Top children"} {
		if !strings.Contains(body, want) {
			t.Fatalf("Analyze body missing core report field %q:\n%s", want, body)
		}
	}
}

func TestViewerModelAnalyzeAcceptsVolumeRootAndRejectsUNC(t *testing.T) {
	// Explicit local volume roots are valid Analyze read roots (CLI/JSON/TUI share analyze.Run).
	body := renderAnalyzeBody(`C:\`)
	if strings.Contains(body, "Status: invalid root") {
		t.Fatalf("volume root should be accepted for Analyze read-only insight:\n%s", body)
	}
	if !strings.Contains(body, "read-only") {
		t.Fatalf("volume root body should remain read-only:\n%s", body)
	}
	if !strings.Contains(body, "Status:") {
		t.Fatalf("volume root body missing Status:\n%s", body)
	}

	// Unsupported roots still fail closed with read-only feedback.
	body = renderAnalyzeBody(`\\server\share\proj`)
	if !strings.Contains(body, "read-only") {
		t.Fatalf("UNC root should show read-only feedback:\n%s", body)
	}
	if !strings.Contains(body, "invalid root") && !strings.Contains(body, "Status: invalid") {
		t.Fatalf("UNC root should show invalid root status:\n%s", body)
	}

	forbiddenPhrases := []string{
		"delete this", "remove this", "clean up", "reclaim space",
		"execute cleanup", "confirm deletion", "select items", "move to trash",
		"permanent delete", "click to delete", "press to remove",
	}
	for _, phrase := range forbiddenPhrases {
		if strings.Contains(strings.ToLower(body), phrase) {
			t.Fatalf("body contains forbidden phrase %q:\n%s", phrase, body)
		}
	}
}

func TestRootModelAnalyzeShowsDriveEntryHints(t *testing.T) {
	original := listAnalyzeLocalVolumes
	listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
		return []analyze.LocalVolume{{
			Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true,
		}}
	}
	t.Cleanup(func() { listAnalyzeLocalVolumes = original })

	model := newRootModel()
	// Navigate to Analyze
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd != nil {
		if loaded, ok := cmd().(analyzeVolumesLoadedMsg); ok {
			next, _ = model.Update(loaded)
			model = next.(rootModel)
		}
	}

	content := model.content()
	if !strings.Contains(content, "r refresh") {
		t.Fatalf("Analyze drive entry missing refresh hint: %s", content)
	}
	if !strings.Contains(content, "no cleanup or deletion") {
		t.Fatalf("Analyze drive entry missing read-only reminder: %s", content)
	}
	if strings.Contains(content, "e edit path") {
		t.Fatalf("drive entry must not show path-edit hint: %s", content)
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
