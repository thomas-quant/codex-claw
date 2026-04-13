package codexruntime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// NewStdIOClient creates a Client backed by a long-lived codex app-server
// subprocess speaking newline-delimited JSON-RPC over stdio.
func NewStdIOClient(command string, args []string, workdir string, requestTimeout time.Duration) *Client {
	return NewClient(
		NewStdIOTransport(command, args, workdir),
		ClientOptions{RequestTimeout: requestTimeout},
	)
}

type StdIOTransport struct {
	command string
	args    []string
	workdir string
}

func NewStdIOTransport(command string, args []string, workdir string) *StdIOTransport {
	return &StdIOTransport{
		command: command,
		args:    append([]string(nil), args...),
		workdir: workdir,
	}
}

func (t *StdIOTransport) Start(context.Context) (Conn, error) {
	cmd := exec.Command(t.command, t.args...)
	if t.workdir != "" {
		cmd.Dir = t.workdir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codexruntime: open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("codexruntime: open stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("codexruntime: open stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("codexruntime: start stdio transport: %w", err)
	}

	conn := &stdioConn{
		cmd:    cmd,
		stdin:  stdin,
		readCh: make(chan []byte, 64),
		done:   make(chan struct{}),
	}
	go conn.readLoop(stdout)
	go conn.captureStderr(stderr)
	go conn.waitLoop()

	return conn, nil
}

type stdioConn struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	readCh chan []byte
	done   chan struct{}

	closeOnce sync.Once

	stateMu sync.Mutex
	readErr error
	stderr  bytes.Buffer
}

func (c *stdioConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case data, ok := <-c.readCh:
		if !ok {
			return nil, c.currentErr()
		}
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *stdioConn) Write(ctx context.Context, payload []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if _, err := c.stdin.Write(payload); err != nil {
		return c.wrapErr(fmt.Errorf("codexruntime: write stdio payload: %w", err))
	}
	return nil
}

func (c *stdioConn) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-c.done
		closeErr = c.currentErr()
		if closeErr == io.EOF {
			closeErr = nil
		}
	})
	return closeErr
}

func (c *stdioConn) readLoop(stdout io.ReadCloser) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(line) > 0 {
				c.readCh <- append([]byte(nil), line...)
			}
		}

		if err != nil {
			if err == io.EOF {
				c.setErr(io.EOF)
			} else {
				c.setErr(c.wrapErr(fmt.Errorf("codexruntime: read stdio payload: %w", err)))
			}
			close(c.readCh)
			return
		}
	}
}

func (c *stdioConn) captureStderr(stderr io.ReadCloser) {
	defer stderr.Close()

	_, _ = io.Copy(&c.stderr, stderr)
}

func (c *stdioConn) waitLoop() {
	defer close(c.done)

	if err := c.cmd.Wait(); err != nil {
		c.setErr(c.wrapErr(fmt.Errorf("codexruntime: app-server exited: %w", err)))
	}
}

func (c *stdioConn) setErr(err error) {
	if err == nil {
		return
	}

	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.readErr == nil {
		c.readErr = err
	}
}

func (c *stdioConn) currentErr() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.readErr == nil {
		return io.EOF
	}
	return c.readErr
}

func (c *stdioConn) wrapErr(err error) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	stderr := strings.TrimSpace(c.stderr.String())
	if stderr == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, stderr)
}
