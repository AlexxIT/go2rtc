package shell

import "syscall"

// will stop child if parent died (even with SIGKILL)
var procAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
