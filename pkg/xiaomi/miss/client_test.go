package miss

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoginDrainsCommands(t *testing.T) {
	conn := &commandConn{
		commands: make(chan commandResult, 10),
		readDone: make(chan struct{}),
	}
	defer close(conn.commands)
	conn.commands <- commandResult{data: []byte(`{"result":"success"}`)}
	require.NoError(t, login(conn, "public", "sign"))
	require.Equal(t, uint32(cmdAuthReq), conn.writtenCommand)

	// More notifications than the CS2 command queue can hold must be consumed
	// even when the caller only reads media after login.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for i := 0; i < 32; i++ {
		select {
		case conn.commands <- commandResult{data: []byte(`{"result":"ok"}`)}:
		case <-timer.C:
			t.Fatal("command queue was not drained after login")
		}
	}

	conn.commands <- commandResult{err: io.EOF}
	select {
	case <-conn.readDone:
	case <-timer.C:
		t.Fatal("command reader did not reach the connection error")
	}
}

func TestLoginErrors(t *testing.T) {
	readErr := errors.New("read failed")
	writeErr := errors.New("write failed")
	for _, test := range []struct {
		name     string
		response commandResult
		writeErr error
		wantErr  string
	}{
		{name: "write", writeErr: writeErr, wantErr: "write failed"},
		{name: "read", response: commandResult{err: readErr}, wantErr: "read failed"},
		{name: "authentication", response: commandResult{data: []byte(`{"result":"failure"}`)}, wantErr: "miss: auth:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn := &commandConn{
				commands: make(chan commandResult, 1),
				readDone: make(chan struct{}),
				writeErr: test.writeErr,
			}
			defer close(conn.commands)
			conn.commands <- test.response
			require.ErrorContains(t, login(conn, "public", "sign"), test.wantErr)
		})
	}
}

type commandResult struct {
	data []byte
	err  error
}

type commandConn struct {
	Conn
	commands       chan commandResult
	readDone       chan struct{}
	writtenCommand uint32
	writeErr       error
}

func (c *commandConn) WriteCommand(cmd uint32, _ []byte) error {
	c.writtenCommand = cmd
	return c.writeErr
}

func (c *commandConn) ReadCommand() (uint32, []byte, error) {
	res, ok := <-c.commands
	if !ok {
		res.err = io.EOF
	}
	if res.err != nil {
		close(c.readDone)
	}
	return cmdAuthRes, res.data, res.err
}
