//go:build linux

package shell

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// groupAlive reports whether any RUNNING (non-zombie) process still exists
// in pid's process group. A plain kill(-pgid, 0) probe would count zombies:
// orphaned grandchildren reparent to the test process's pid 1, which never
// reaps them. So scan /proc and check pgrp + state instead.
func groupAlive(pgid int) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // process gone between readdir and read
		}
		// stat: pid (comm) state ppid pgrp ... and comm may contain spaces,
		// so split after the LAST ')'
		i := strings.LastIndexByte(string(stat), ')')
		fields := strings.Fields(string(stat[i+1:]))
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "Z" {
			continue
		}
		if fields[2] == strconv.Itoa(pgid) {
			return true
		}
	}
	return false
}

// TestCloseEscalatesToProcessGroupKill: a child that traps the configured
// killsignal (SIGTERM) and has a grandchild must still be fully gone within
// killGrace plus some slack after Close().
func TestCloseEscalatesToProcessGroupKill(t *testing.T) {
	// trap makes sh ignore SIGTERM; the foreground sleep is a grandchild a
	// single-pid kill would never reach
	cmd := NewCommand(`sh -c "trap : TERM; while :; do sleep 60; done"`)
	// mimic exec's killsignal=15 override, the signal the child ignores
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}

	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid

	// let sh spawn its sleep grandchild
	time.Sleep(300 * time.Millisecond)
	require.True(t, groupAlive(pid))

	ts := time.Now()
	require.NoError(t, cmd.Close())

	// SIGTERM is trapped, so only the killGrace escalation can reap the
	// group (sh AND the sleep grandchild)
	require.Eventually(t, func() bool {
		return !groupAlive(pid)
	}, killGrace+5*time.Second, 100*time.Millisecond)

	elapsed := time.Since(ts)
	require.GreaterOrEqual(t, elapsed, killGrace,
		"group died before escalation: test setup no longer exercises it")
	t.Logf("process group reaped %s after Close()", elapsed)
}

// TestCloseKillsPromptlyByDefault: without a killsignal override the default
// Cancel (SIGKILL) reaps a plain child well before the escalation grace.
func TestCloseKillsPromptlyByDefault(t *testing.T) {
	cmd := NewCommand("sleep 60")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid

	require.NoError(t, cmd.Close())

	require.Eventually(t, func() bool {
		return !groupAlive(pid)
	}, 3*time.Second, 50*time.Millisecond)
}
