package exec

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
)

func parseParams(s string) (*Params, error) {
	args := &Params{
		KillSignal:  syscall.SIGKILL,
		KillTimeout: 5 * time.Second,
		Command:     s,
	}

	var query url.Values
	if i := strings.IndexByte(s, '#'); i > 0 {
		query = streams.ParseQuery(s[i+1:])
		args.Command = s[:i]
	}

	if val, ok := query["killsignal"]; ok {
		if sig, err := parseSignal(val[0]); err == nil {
			args.KillSignal = sig
		} else {
			return nil, fmt.Errorf("could not parse killsignal param (%s)", val[0])
		}
	}

	if val, ok := query["killtimeout"]; ok {
		if i, err := strconv.Atoi(val[0]); err == nil {
			args.KillTimeout = time.Duration(i) * time.Second
		} else {
			return nil, fmt.Errorf("could not convert killtimeout param (%s) to int", val[0])
		}
	}

	return args, nil
}

func parseSignal(signalString string) (os.Signal, error) {
	signalMap := map[string]os.Signal{
		"sighup":    syscall.SIGHUP,
		"sigint":    syscall.SIGINT,
		"sigquit":   syscall.SIGQUIT,
		"sigill":    syscall.SIGILL,
		"sigtrap":   syscall.SIGTRAP,
		"sigabrt":   syscall.SIGABRT,
		"sigbus":    syscall.SIGBUS,
		"sigfpe":    syscall.SIGFPE,
		"sigkill":   syscall.SIGKILL,
		"sigusr1":   syscall.SIGUSR1,
		"sigsegv":   syscall.SIGSEGV,
		"sigusr2":   syscall.SIGUSR2,
		"sigpipe":   syscall.SIGPIPE,
		"sigalrm":   syscall.SIGALRM,
		"sigterm":   syscall.SIGTERM,
		"sigchld":   syscall.SIGCHLD,
		"sigcont":   syscall.SIGCONT,
		"sigstop":   syscall.SIGSTOP,
		"sigtstp":   syscall.SIGTSTP,
		"sigttin":   syscall.SIGTTIN,
		"sigttou":   syscall.SIGTTOU,
		"sigurg":    syscall.SIGURG,
		"sigxcpu":   syscall.SIGXCPU,
		"sigxfsz":   syscall.SIGXFSZ,
		"sigvtalrm": syscall.SIGVTALRM,
		"sigprof":   syscall.SIGPROF,
		"sigwinch":  syscall.SIGWINCH,
		"sigio":     syscall.SIGIO,
		"sigpoll":   syscall.SIGPOLL,
		"sigpwr":    syscall.SIGPWR,
		"sigsys":    syscall.SIGSYS,
	}

	signalValue, ok := signalMap[strings.ToLower(signalString)]
	if !ok {
		return nil, fmt.Errorf("invalid signal: %s", signalString)
	}

	return signalValue, nil
}
