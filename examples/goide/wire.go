package goide

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const maxProtocolMessage = 16 << 20

// LSP and DAP both frame JSON with a Content-Length header.
func readFrame(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if len(line) > 8192 {
			return nil, errors.New("protocol header is too long")
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, err
			}
		}
	}
	if length < 0 || length > maxProtocolMessage {
		return nil, fmt.Errorf("invalid protocol message length %d", length)
	}
	data := make([]byte, length)
	_, err := io.ReadFull(r, data)
	return data, err
}

func writeFrame(w io.Writer, body []byte) error {
	if len(body) > maxProtocolMessage {
		return errors.New("protocol message is too large")
	}
	packet := bytes.NewBuffer(make([]byte, 0, len(body)+64))
	fmt.Fprintf(packet, "Content-Length: %d\r\n\r\n", len(body))
	packet.Write(body)
	for packet.Len() > 0 {
		n, err := w.Write(packet.Bytes())
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		packet.Next(n)
	}
	return nil
}

// protocolConn owns the two worker goroutines. Message callbacks run on the
// UI owner through post; callers never touch widgets from the read goroutine.
type protocolConn struct {
	in io.ReadCloser
	out io.WriteCloser
	post func(func())
	onMessage func(json.RawMessage)
	onError func(error)
	queue chan []byte
	done chan struct{}
	once sync.Once
}

func newProtocolConn(in io.ReadCloser, out io.WriteCloser, post func(func()), onMessage func(json.RawMessage), onError func(error)) *protocolConn {
	c := &protocolConn{in: in, out: out, post: post, onMessage: onMessage,
		onError: onError, queue: make(chan []byte, 128), done: make(chan struct{})}
	go c.readLoop()
	go c.writeLoop()
	return c
}

func (c *protocolConn) send(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	select {
	case <-c.done:
		return io.ErrClosedPipe
	case c.queue <- data:
		return nil
	default:
		return errors.New("protocol output queue is full")
	}
}

func (c *protocolConn) fail(err error) {
	select {
	case <-c.done:
		return
	default:
		c.post(func() { c.onError(err) })
		c.Close()
	}
}

func (c *protocolConn) readLoop() {
	r := bufio.NewReader(c.in)
	for {
		data, err := readFrame(r)
		if err != nil {
			c.fail(err)
			return
		}
		c.post(func() { c.onMessage(json.RawMessage(data)) })
	}
}

func (c *protocolConn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case data := <-c.queue:
			if err := writeFrame(c.out, data); err != nil {
				c.fail(err)
				return
			}
		}
	}
}

func (c *protocolConn) Close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.in.Close()
		_ = c.out.Close()
	})
}
