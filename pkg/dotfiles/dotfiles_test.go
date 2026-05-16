package dotfiles

import (
	"context"
	"strings"
	"testing"

	"github.com/garaemon/devgo/pkg/config"
)

func TestResolve_Disabled(t *testing.T) {
	got := Resolve(&config.DotfilesConfig{Repository: "https://example.com/x"}, Override{}, true)
	if got != nil {
		t.Errorf("expected nil when disabled, got %+v", got)
	}
}

func TestResolve_NoRepositoryAnywhere(t *testing.T) {
	got := Resolve(nil, Override{}, false)
	if got != nil {
		t.Errorf("expected nil when no repository configured, got %+v", got)
	}
}

func TestResolve_FileOnly_DefaultsTargetPath(t *testing.T) {
	got := Resolve(&config.DotfilesConfig{Repository: "https://example.com/x"}, Override{}, false)
	if got == nil {
		t.Fatalf("expected resolved config, got nil")
	}
	if got.Repository != "https://example.com/x" {
		t.Errorf("Repository = %q, want %q", got.Repository, "https://example.com/x")
	}
	if got.TargetPath != DefaultTargetPath {
		t.Errorf("TargetPath = %q, want %q", got.TargetPath, DefaultTargetPath)
	}
	if got.InstallCommand != "" {
		t.Errorf("InstallCommand = %q, want empty (auto-detect)", got.InstallCommand)
	}
}

func TestResolve_OverridesWinOverFile(t *testing.T) {
	file := &config.DotfilesConfig{
		Repository:     "https://example.com/file",
		TargetPath:     "/file/path",
		InstallCommand: "file.sh",
	}
	override := Override{
		Repository:     "https://example.com/cli",
		TargetPath:     "/cli/path",
		InstallCommand: "cli.sh",
	}
	got := Resolve(file, override, false)
	if got.Repository != "https://example.com/cli" {
		t.Errorf("Repository = %q, want CLI override", got.Repository)
	}
	if got.TargetPath != "/cli/path" {
		t.Errorf("TargetPath = %q, want CLI override", got.TargetPath)
	}
	if got.InstallCommand != "cli.sh" {
		t.Errorf("InstallCommand = %q, want CLI override", got.InstallCommand)
	}
}

func TestResolve_PartialOverridePreservesFile(t *testing.T) {
	file := &config.DotfilesConfig{
		Repository:     "https://example.com/file",
		TargetPath:     "/file/path",
		InstallCommand: "file.sh",
	}
	override := Override{Repository: "https://example.com/cli"}
	got := Resolve(file, override, false)
	if got.Repository != "https://example.com/cli" {
		t.Errorf("Repository = %q, want CLI override", got.Repository)
	}
	if got.TargetPath != "/file/path" {
		t.Errorf("TargetPath = %q, want file value preserved", got.TargetPath)
	}
	if got.InstallCommand != "file.sh" {
		t.Errorf("InstallCommand = %q, want file value preserved", got.InstallCommand)
	}
}

// fakeExec records every command and replies based on programmable rules.
type fakeExec struct {
	calls []fakeCall
	// rules: substring of the joined command -> response
	rules []fakeRule
}

type fakeCall struct {
	user string
	cmd  []string
}

type fakeRule struct {
	contains string
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (f *fakeExec) Exec(_ context.Context, user string, cmd []string) (string, string, int, error) {
	f.calls = append(f.calls, fakeCall{user: user, cmd: append([]string{}, cmd...)})
	joined := strings.Join(cmd, " ")
	for _, r := range f.rules {
		if strings.Contains(joined, r.contains) {
			return r.stdout, r.stderr, r.exitCode, r.err
		}
	}
	return "", "", 0, nil
}

func (f *fakeExec) commandsContaining(substr string) int {
	count := 0
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c.cmd, " "), substr) {
			count++
		}
	}
	return count
}

func TestApply_NilConfig_NoOp(t *testing.T) {
	exec := &fakeExec{}
	if err := Apply(context.Background(), exec, "user", nil, false, nil); err != nil {
		t.Errorf("Apply(nil) error = %v, want nil", err)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected no exec calls, got %d", len(exec.calls))
	}
}

func TestApply_SkipsWhenTargetExistsAndNotForce(t *testing.T) {
	exec := &fakeExec{
		rules: []fakeRule{
			{contains: "[ -e", exitCode: 0}, // target exists
		},
	}
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "/home/u/df"}
	if err := Apply(context.Background(), exec, "u", cfg, false, nil); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if exec.commandsContaining("git clone") != 0 {
		t.Errorf("did not expect git clone when target exists and not force")
	}
}

func TestApply_ForceRemovesAndReclones(t *testing.T) {
	exec := &fakeExec{
		rules: []fakeRule{
			// Probing the install scripts after clone returns "not found".
			// First check (the targetPath itself) returns "exists".
			{contains: "[ -e '/home/u/df'", exitCode: 0},
			{contains: "[ -e", exitCode: 1}, // install script probes -> not found
		},
	}
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "/home/u/df"}
	if err := Apply(context.Background(), exec, "u", cfg, true, nil); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if exec.commandsContaining("rm -rf") != 1 {
		t.Errorf("expected exactly 1 rm -rf call, got %d", exec.commandsContaining("rm -rf"))
	}
	if exec.commandsContaining("git clone") != 1 {
		t.Errorf("expected exactly 1 git clone call, got %d", exec.commandsContaining("git clone"))
	}
}

func TestApply_RunsExplicitInstallCommand(t *testing.T) {
	exec := &fakeExec{
		rules: []fakeRule{
			{contains: `printf %s "$HOME"`, stdout: "/home/u"},
			{contains: "[ -e '/home/u/df/bootstrap.sh'", exitCode: 0}, // explicit script probe says exists
			{contains: "[ -e", exitCode: 1},                           // target dir probe says missing
		},
	}
	cfg := &Config{
		Repository:     "https://example.com/x",
		TargetPath:     "~/df",
		InstallCommand: "bootstrap.sh",
	}
	if err := Apply(context.Background(), exec, "u", cfg, false, nil); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if exec.commandsContaining("./bootstrap.sh") != 1 {
		t.Errorf("expected install command ./bootstrap.sh to run, got calls=%v", exec.calls)
	}
	if exec.commandsContaining("/home/u/df") == 0 {
		t.Errorf("expected resolved path /home/u/df to appear in commands, got calls=%v", exec.calls)
	}
}

func TestApply_FailsWhenExplicitInstallCommandMissing(t *testing.T) {
	exec := &fakeExec{
		rules: []fakeRule{
			{contains: `printf %s "$HOME"`, stdout: "/home/u"},
			{contains: "[ -e", exitCode: 1}, // both target and script probe miss
		},
	}
	cfg := &Config{
		Repository:     "https://example.com/x",
		TargetPath:     "~/df",
		InstallCommand: "missing.sh",
	}
	err := Apply(context.Background(), exec, "u", cfg, false, nil)
	if err == nil {
		t.Fatalf("expected Apply to error when explicit install command is missing")
	}
	if !strings.Contains(err.Error(), "missing.sh") {
		t.Errorf("error %q should mention the missing script name", err.Error())
	}
}

func TestApply_DetectsDefaultInstallScript(t *testing.T) {
	// Simulate that target doesn't exist initially, then clone happens, then
	// install.sh is missing but bootstrap.sh exists.
	exec := &fakeExec{}
	exec.rules = []fakeRule{
		{contains: `printf %s "$HOME"`, stdout: "/home/u"},
		{contains: "[ -e '/home/u/df/install.sh'", exitCode: 1},
		{contains: "[ -e '/home/u/df/install'", exitCode: 1},
		{contains: "[ -e '/home/u/df/bootstrap.sh'", exitCode: 0},
		{contains: "[ -e", exitCode: 1}, // target path probe and any others
	}
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "~/df"}
	if err := Apply(context.Background(), exec, "u", cfg, false, nil); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if exec.commandsContaining("./bootstrap.sh") != 1 {
		t.Errorf("expected bootstrap.sh to run, got calls=%v", exec.calls)
	}
	if exec.commandsContaining("./install.sh") != 0 {
		t.Errorf("did not expect install.sh to run when missing")
	}
}

func TestResolveHome(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "no tilde returns unchanged", path: "/abs/path", want: "/abs/path"},
		{name: "tilde alone resolves to HOME", path: "~", want: "/home/u"},
		{name: "tilde slash prefix resolves", path: "~/dotfiles", want: "/home/u/dotfiles"},
		{name: "tilde middle of path is left alone", path: "/etc/~user", want: "/etc/~user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fakeExec{rules: []fakeRule{{contains: "printf", stdout: "/home/u"}}}
			got, err := resolveHome(context.Background(), exec, "u", tt.path)
			if err != nil {
				t.Fatalf("resolveHome error = %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestSanitizeRepoURL(t *testing.T) {
	cases := map[string]string{
		"https://alice:secret@github.com/x/y": "https://***@github.com/x/y",
		"https://github.com/x/y":              "https://github.com/x/y",
		"git@github.com:x/y.git":              "git@github.com:x/y.git",
		"":                                    "",
	}
	for input, want := range cases {
		got := SanitizeRepoURL(input)
		if got != want {
			t.Errorf("SanitizeRepoURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCloneRepo_RedactsCredentialsInErrorMessage(t *testing.T) {
	repo := "https://alice:secret@github.com/x/y"
	exec := &fakeExec{rules: []fakeRule{
		{
			contains: "git clone",
			stderr:   "fatal: could not read from " + repo,
			exitCode: 128,
		},
	}}
	err := cloneRepo(context.Background(), exec, "u", repo, "/tmp/x")
	if err == nil {
		t.Fatalf("expected error from cloneRepo, got nil")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error message leaks credential: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("error message should contain redaction marker (***): %v", err)
	}
}

func TestResolveHome_RejectsOtherUserForm(t *testing.T) {
	exec := &fakeExec{}
	_, err := resolveHome(context.Background(), exec, "u", "~someone/dotfiles")
	if err == nil {
		t.Fatalf("expected error for ~someone/... path, got nil")
	}
	if !strings.Contains(err.Error(), "~user form") {
		t.Errorf("error %q does not mention unsupported ~user form", err.Error())
	}
}

func TestApply_RejectsOtherUserPath(t *testing.T) {
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "~someone/df"}
	err := Apply(context.Background(), &fakeExec{}, "u", cfg, false, nil)
	if err == nil {
		t.Fatalf("expected wrapped ~user error from Apply, got nil")
	}
	if !strings.Contains(err.Error(), "~user form") {
		t.Errorf("Apply error %q should propagate the ~user diagnostic", err.Error())
	}
}

func TestApply_PropagatesInstallScriptFailure(t *testing.T) {
	exec := &fakeExec{rules: []fakeRule{
		{contains: `printf %s "$HOME"`, stdout: "/home/u"},
		{contains: "[ -e '/home/u/df/install.sh'", exitCode: 0},
		{contains: "[ -e", exitCode: 1},
		{contains: "&& './install.sh'", stdout: "starting", stderr: "boom", exitCode: 2},
	}}
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "~/df"}
	err := Apply(context.Background(), exec, "u", cfg, false, nil)
	if err == nil {
		t.Fatalf("expected install script error, got nil")
	}
	if !strings.Contains(err.Error(), "exited with 2") {
		t.Errorf("error should include exit code, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should include stderr content, got %v", err)
	}
}

func TestApply_InstallScriptExit126_AddsExecutableHint(t *testing.T) {
	exec := &fakeExec{rules: []fakeRule{
		{contains: `printf %s "$HOME"`, stdout: "/home/u"},
		{contains: "[ -e '/home/u/df/install.sh'", exitCode: 0},
		{contains: "[ -e", exitCode: 1},
		{contains: "&& './install.sh'", stderr: "Permission denied", exitCode: 126},
	}}
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "~/df"}
	err := Apply(context.Background(), exec, "u", cfg, false, nil)
	if err == nil {
		t.Fatalf("expected error for exit 126, got nil")
	}
	if !strings.Contains(err.Error(), "executable") {
		t.Errorf("exit 126 should trigger executable-bit hint, got %v", err)
	}
}

func TestDefaultTargetPath_IsSubdirectoryOfHome(t *testing.T) {
	if DefaultTargetPath != "~/dotfiles" {
		t.Errorf("DefaultTargetPath = %q, want %q (clone into a subdir, not $HOME)", DefaultTargetPath, "~/dotfiles")
	}
}

// TestApply_NeverPassesUnresolvedTilde is a regression test for the bug
// where shell-quoted paths like `'~'` would not be expanded by the inner
// shell, causing devgo to create a literal "~" directory in the container.
// After Apply runs, no command (other than the HOME probe itself) may
// contain a single-quoted tilde.
func TestApply_NeverPassesUnresolvedTilde(t *testing.T) {
	exec := &fakeExec{
		rules: []fakeRule{
			{contains: `printf %s "$HOME"`, stdout: "/home/u"},
			{contains: "[ -e", exitCode: 1},
		},
	}
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "~/dotfiles"}
	if err := Apply(context.Background(), exec, "u", cfg, false, nil); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	for i, c := range exec.calls {
		joined := strings.Join(c.cmd, " ")
		if strings.Contains(joined, `printf %s "$HOME"`) {
			continue
		}
		if strings.Contains(joined, "'~") {
			t.Errorf("call %d still contains unresolved tilde-quoted path: %v", i, c.cmd)
		}
	}
}

// TestApply_DefaultConfig_DoesNotCloneIntoBareHome is a regression test for
// the bug where DefaultTargetPath was "~", which made git clone target
// $HOME directly. Cloning into a non-empty directory fails. The default
// must be a subdirectory.
func TestApply_DefaultConfig_DoesNotCloneIntoBareHome(t *testing.T) {
	exec := &fakeExec{
		rules: []fakeRule{
			{contains: `printf %s "$HOME"`, stdout: "/home/u"},
			{contains: "[ -e", exitCode: 1},
		},
	}
	resolved := Resolve(&config.DotfilesConfig{Repository: "https://example.com/x"}, Override{}, false)
	if resolved == nil {
		t.Fatalf("Resolve returned nil")
	}
	if err := Apply(context.Background(), exec, "u", resolved, false, nil); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if exec.commandsContaining("git clone") == 0 {
		t.Fatalf("expected git clone to run, got none")
	}
	for _, c := range exec.calls {
		joined := strings.Join(c.cmd, " ")
		if !strings.Contains(joined, "git clone") {
			continue
		}
		// The clone target must not be the bare HOME directory.
		if strings.Contains(joined, "'/home/u'") {
			t.Errorf("git clone targeted bare $HOME, must clone into a subdir: %v", c.cmd)
		}
		if !strings.Contains(joined, "'/home/u/dotfiles'") {
			t.Errorf("expected clone target /home/u/dotfiles, got: %v", c.cmd)
		}
	}
}

func TestApply_NoInstallScriptFound_LeavesCloneInPlace(t *testing.T) {
	exec := &fakeExec{
		rules: []fakeRule{
			{contains: "[ -e", exitCode: 1}, // every existence check fails
		},
	}
	cfg := &Config{Repository: "https://example.com/x", TargetPath: "/home/u/df"}
	if err := Apply(context.Background(), exec, "u", cfg, false, nil); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if exec.commandsContaining("git clone") != 1 {
		t.Errorf("expected clone to happen, got %d", exec.commandsContaining("git clone"))
	}
	for _, c := range exec.calls {
		joined := strings.Join(c.cmd, " ")
		if strings.Contains(joined, "cd ") && strings.Contains(joined, " && ./") {
			t.Errorf("did not expect any install command to run, got %v", c.cmd)
		}
	}
}

func TestShellQuote_HandlesEmbeddedQuote(t *testing.T) {
	got := shellQuote("a'b")
	want := `'a'\''b'`
	if got != want {
		t.Errorf("shellQuote(%q) = %q, want %q", "a'b", got, want)
	}
}

