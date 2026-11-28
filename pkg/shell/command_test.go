//go:build linux

package shell

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Start a shell leader and a descendant, and wait for an explicit readiness
// handshake instead of assuming the signal handler is installed after a sleep.
func startGroup(t *testing.T, script string, sig os.Signal, delay time.Duration) (*Command, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sh")
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := NewCommand("sh " + path)
	cmd.WaitDelay = delay
	if sig != nil {
		cmd.SetKillSignal(sig)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Close()
		// The direct child may still be running after a failed assertion.
		_ = cmd.Process.Kill()
	})

	// The child announces readiness after installing its signal handler.
	result := make(chan int, 1)
	go func() {
		var pid int
		var ready bool
		s := bufio.NewScanner(out)
		for s.Scan() {
			if s.Text() == "ready" {
				ready = true
			} else {
				pid, _ = strconv.Atoi(s.Text())
			}
			if pid > 0 && ready {
				result <- pid
				return
			}
		}
		result <- 0
	}()
	select {
	case pid := <-result:
		if pid == 0 {
			t.Fatal("child did not report readiness")
		}
		t.Cleanup(func() {
			if processRunning(pid) {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
			}
		})
		return cmd, pid
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for child")
		return nil, 0
	}
}

// The nested shell ignores TERM before exec replaces it with sleep.
// The ignored signal disposition survives exec; the parent shell prints
// the descendant PID and waits for it.
const ignoreTerm = `sh -c 'trap "" TERM; echo ready; exec sleep 60' &
echo $!
wait
`

// A killed orphan may remain as a zombie until PID 1 reaps it.
// Check the state in /proc rather than treating every existing PID as alive.
func processRunning(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	i := strings.LastIndexByte(string(b), ')')
	fields := strings.Fields(string(b[i+1:]))
	return len(fields) == 0 || fields[0] != "Z"
}

// Bound Wait in tests so a regression cannot hang the entire test suite.
func waitCommand(t *testing.T, cmd *Command) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return")
		return nil
	}
}

func waitProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for processRunning(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still running", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Default cancellation must kill the group immediately and remain safe
// when Close is called more than once.
func TestCloseKillsProcessGroup(t *testing.T) {
	cmd, child := startGroup(t, ignoreTerm, nil, 5*time.Second)
	start := time.Now()
	_ = cmd.Close()
	_ = cmd.Close()
	if err := waitCommand(t, cmd); err == nil {
		t.Fatal("expected killed command to return an error")
	}
	waitProcessExit(t, child)
	if elapsed := time.Since(start); elapsed >= 4*time.Second {
		t.Fatalf("default SIGKILL unnecessarily waited: %v", elapsed)
	}
}

// Cover both a stubborn leader and a leader that exits on TERM while its
// descendant survives. The latter exercises delayed reaping of the leader.
func TestCloseEscalatesProcessGroup(t *testing.T) {
	for _, tc := range []struct {
		name, script string
	}{
		{"leader ignores TERM", "trap '' TERM\n" + ignoreTerm},
		{"leader exits first", ignoreTerm},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const delay = 400 * time.Millisecond
			cmd, child := startGroup(t, tc.script, syscall.SIGTERM, delay)
			start := time.Now()
			_ = cmd.Close()
			_ = cmd.Close()
			if !processRunning(child) {
				t.Fatal("child died before the configured grace period")
			}
			if err := waitCommand(t, cmd); err == nil {
				t.Fatal("expected killed command to return an error")
			}
			if elapsed := time.Since(start); elapsed < delay {
				t.Fatalf("Wait returned before group escalation: %v", elapsed)
			}
			waitProcessExit(t, child)
		})
	}
}

func TestCloseSignalsChildren(t *testing.T) {
	// The child uses its default TERM disposition and must exit before escalation.
	cmd, child := startGroup(t, `sh -c 'echo ready; exec sleep 60' &
echo $!
wait
`, syscall.SIGTERM, 3*time.Second)
	start := time.Now()
	_ = cmd.Close()
	waitProcessExit(t, child)
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("child did not receive the initial SIGTERM: %v", elapsed)
	}
	if err := waitCommand(t, cmd); err == nil {
		t.Fatal("expected cancellation error")
	}
}

// An explicit zero disables forced escalation; clean up with SIGKILL
// after verifying the descendant survives the initial signal.
func TestCloseZeroKillTimeout(t *testing.T) {
	cmd, child := startGroup(t, "trap '' TERM\n"+ignoreTerm, syscall.SIGTERM, 0)
	_ = cmd.Close()
	time.Sleep(100 * time.Millisecond)
	if !processRunning(child) {
		t.Fatal("killtimeout=0 unexpectedly escalated")
	}
	if err := signalProcess(cmd.Process, os.Kill, 0); err != nil {
		t.Fatal(err)
	}
	if err := waitCommand(t, cmd); err == nil {
		t.Fatal("expected cancellation error")
	}
	waitProcessExit(t, child)
}

// Natural exit must retain the real exit status and return the same
// result to multiple Wait callers, even without a Close request.
func TestWaitNaturalExit(t *testing.T) {
	cmd := NewCommand("sh -c 'exit 7'")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	err := waitCommand(t, cmd)
	if err == nil || cmd.Wait() != err || cmd.ProcessState.ExitCode() != 7 {
		t.Fatalf("unexpected Wait result: %v", err)
	}
	_ = cmd.Close()
}

// A shell can exit without waiting for a background child. Clean up the
// group before reaping the shell, even if Close has not been called yet.
func TestWaitKillsRemainingGroup(t *testing.T) {
	cmd, child := startGroup(t, `sh -c 'trap "" TERM; echo ready; exec sleep 60' &
echo $!
# Give the test time to read the child PID before the shell exits.
sleep 1
`, nil, time.Second)
	if err := waitCommand(t, cmd); err != nil {
		t.Fatal(err)
	}
	waitProcessExit(t, child)
	_ = cmd.Close()
}
