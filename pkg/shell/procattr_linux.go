package shell

import "syscall"

//   - Pdeathsig will stop child if parent died (even with SIGKILL)
//   - Setpgid puts the child in its own process group so Command.ensureKill
//     can SIGKILL the whole tree, grandchildren (e.g. ffmpeg spawned via a
//     shell wrapper) included, which a plain Process.Kill never reaches
var procAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM, Setpgid: true}

// killProcessGroup sends SIGKILL to pid's entire process group.
func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
