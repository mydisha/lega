package clean

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
)

// fakeSudo returns a SudoCall running a shell script in place of sudo,
// with stderr only kept, so tests do not print to the terminal.
func fakeSudo(script string) *SudoCall {
	s := &SudoCall{Cmd: exec.Command("sh", "-c", script)}
	s.Cmd.Stderr = &s.stderr
	s.Cmd.Stdout = &s.stdout
	return s
}

func TestSudoCallErr(t *testing.T) {
	if (&SudoCall{}).Err(nil) != nil {
		t.Error("nil error should stay nil")
	}

	missing := &SudoCall{Cmd: exec.Command("lega-test-no-such-command")}
	if err := missing.Err(missing.Cmd.Run()); err != errNoSudo {
		t.Errorf("missing sudo: %v", err)
	}

	for _, c := range []struct{ script, want string }{
		{`echo "Sorry, try again." >&2; echo "sudo: 3 incorrect password attempts" >&2; exit 1`, "sudo: 3 incorrect password attempts"},
		{`echo "tester is not in the sudoers file." >&2; exit 1`, "tester is not in the sudoers file."},
		{`echo "sudo: ユーザー ✓ 拒否" >&2; exit 1`, "sudo: ユーザー ✓ 拒否"},
		{`exit 1`, errNoPassword.Error()}, // Ctrl+C at the prompt prints nothing
	} {
		s := fakeSudo(c.script)
		if err := s.Err(s.Cmd.Run()); err == nil || err.Error() != c.want {
			t.Errorf("%q: got %v, want %q", c.script, err, c.want)
		}
	}
}

func TestSudoReady(t *testing.T) {
	ctx := context.Background()
	type run struct {
		lookErr, probeErr error
		tty               bool
		askErr            error
	}
	for _, c := range []struct {
		name string
		r    run
		want error
		asks int
	}{
		{name: "sudo missing", r: run{lookErr: errors.New("nope")}, want: errNoSudo},
		{name: "no password needed", r: run{}, want: nil},
		{name: "no terminal", r: run{probeErr: errors.New("password required")}, want: errNoTerminal},
		{name: "asks", r: run{probeErr: errors.New("password required"), tty: true}, asks: 1},
		{name: "cancelled", r: run{probeErr: errors.New("password required"), tty: true, askErr: &exec.ExitError{}}, asks: 1, want: errNoPassword},
	} {
		t.Run(c.name, func(t *testing.T) {
			cx := testContext(t)
			cx.LookPath = func(string) (string, error) { return "/usr/bin/sudo", c.r.lookErr }
			var probed []string
			cx.Run = func(_ context.Context, name string, args ...string) ([]byte, error) {
				probed = append([]string{name}, args...)
				return nil, c.r.probeErr
			}
			prev := canPrompt
			canPrompt = func() bool { return c.r.tty }
			t.Cleanup(func() { canPrompt = prev })

			asks := 0
			err := SudoReady(ctx, cx, func(s *SudoCall) error {
				asks++
				if !slices.Equal(s.Cmd.Args[:2], []string{"sudo", "-v"}) {
					t.Errorf("ask got %v, want sudo -v", s.Cmd.Args)
				}
				return c.r.askErr
			})
			if err != c.want {
				t.Errorf("err = %v, want %v", err, c.want)
			}
			if asks != c.asks {
				t.Errorf("asked %d times, want %d", asks, c.asks)
			}
			if c.r.lookErr == nil && !slices.Equal(probed, []string{"sudo", "-n", "-v"}) {
				t.Errorf("probe ran %v", probed)
			}
		})
	}
}

func TestCleanAsRootWithoutCachedLogin(t *testing.T) {
	c := testContext(t)
	var calls int
	c.Run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls++
		return nil, errors.New("sudo: a password is required")
	}
	apt, _ := Lookup("apt-cache")
	reps, err := CleanAsRoot(context.Background(), c, []Finding{{Rule: apt}})
	if !errors.Is(err, ErrSudoNotCached) || reps != nil {
		t.Fatalf("got %v %v, want ErrSudoNotCached", reps, err)
	}
	if calls != 1 {
		t.Errorf("the helper must not run when sudo has no login: %d calls", calls)
	}
}

func TestRootHelper(t *testing.T) {
	apt, _ := Lookup("apt-cache")
	journal, _ := Lookup("journal")
	h, err := RootHelper([]Finding{{Rule: apt}, {Rule: journal}})
	if err != nil {
		t.Fatal(err)
	}
	args := h.Cmd.Args
	if args[0] != "sudo" || slices.Contains(args, "-n") || args[1] != "-p" {
		t.Errorf("helper must let sudo ask: %v", args)
	}
	if n := len(args); args[n-3] != RootCommand || args[n-2] != "apt-cache" || args[n-1] != "journal" {
		t.Errorf("args %v", args)
	}
	if h.Cmd.Stdout != &h.stdout || h.Cmd.Stdin != nil {
		t.Error("stdout must be collected and stdin left for the terminal")
	}

	c := testContext(t)
	ok := fakeSudo(`echo '[{"id":"apt-cache","removals":[{"path":"apt-get clean","bytes":7000}]}]'`)
	reps, err := ok.Reports(c, ok.Cmd.Run())
	if err != nil || len(reps) != 1 || reps[0].Freed != 7000 {
		t.Errorf("reports %+v, %v", reps, err)
	}

	bad := fakeSudo(`echo "sudo: 3 incorrect password attempts" >&2; exit 1`)
	if _, err := bad.Reports(c, bad.Cmd.Run()); err == nil || err.Error() != "sudo: 3 incorrect password attempts" {
		t.Errorf("failed helper: %v", err)
	}

	garbage := fakeSudo(`echo not json`)
	if _, err := garbage.Reports(c, garbage.Cmd.Run()); err == nil {
		t.Error("bad output should fail")
	}
}
