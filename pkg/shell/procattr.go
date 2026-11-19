//go:build !linux

package shell

import "syscall"

var procAttr *syscall.SysProcAttr

// killProcessGroup: no Setpgid outside linux. ensureKill's Process.Kill
// already did everything we can do portably.
func killProcessGroup(pid int) {}
