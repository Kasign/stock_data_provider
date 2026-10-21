package cache

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type Redis struct {
	Addr, Username, Password string
	DB                       int
	Timeout                  time.Duration
}

func NewRedis(addr, username, password string, db int, timeout time.Duration) *Redis {
	return &Redis{Addr: addr, Username: username, Password: password, DB: db, Timeout: timeout}
}
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := r.command(ctx, []string{"GET", key})
	if err != nil {
		return nil, false, err
	}
	if v == nil {
		return nil, false, nil
	}
	b, ok := v.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("unexpected Redis GET reply %T", v)
	}
	return b, true, nil
}
func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl.Milliseconds() < 1 {
		return errors.New("Redis TTL must be at least 1ms")
	}
	_, err := r.command(ctx, []string{"SET", key, string(value), "PX", strconv.FormatInt(ttl.Milliseconds(), 10)})
	return err
}
func (r *Redis) Ping(ctx context.Context) error {
	v, err := r.command(ctx, []string{"PING"})
	if err != nil {
		return err
	}
	if v != "PONG" {
		return fmt.Errorf("unexpected Redis PING response %v", v)
	}
	return nil
}
func (r *Redis) command(ctx context.Context, args []string) (any, error) {
	if strings.TrimSpace(r.Addr) == "" {
		return nil, errors.New("Redis address is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", r.Addr)
	if err != nil {
		return nil, fmt.Errorf("connect Redis: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	reader := bufio.NewReader(conn)
	if r.Password != "" {
		auth := []string{"AUTH", r.Password}
		if r.Username != "" {
			auth = []string{"AUTH", r.Username, r.Password}
		}
		if _, err := redisCommand(conn, reader, auth); err != nil {
			return nil, fmt.Errorf("Redis AUTH: %w", err)
		}
	}
	if r.DB != 0 {
		if _, err := redisCommand(conn, reader, []string{"SELECT", strconv.Itoa(r.DB)}); err != nil {
			return nil, fmt.Errorf("Redis SELECT: %w", err)
		}
	}
	return redisCommand(conn, reader, args)
}
func redisCommand(conn net.Conn, reader *bufio.Reader, args []string) (any, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len([]byte(a)), a)
	}
	if err := writeAll(conn, []byte(b.String())); err != nil {
		return nil, err
	}
	return readRESP(reader)
}
func readRESP(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(line, "\r\n") {
		return nil, errors.New("malformed Redis RESP line terminator")
	}
	line = strings.TrimSuffix(line, "\r\n")
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return nil, errors.New(line)
	case ':':
		v, e := strconv.ParseInt(line, 10, 64)
		return v, e
	case '$':
		n, e := strconv.Atoi(line)
		if e != nil {
			return nil, e
		}
		if n < 0 {
			return nil, nil
		}
		if n > 32<<20 {
			return nil, errors.New("Redis bulk reply too large")
		}
		buf := make([]byte, n+2)
		if _, e = io.ReadFull(r, buf); e != nil {
			return nil, e
		}
		if string(buf[n:]) != "\r\n" {
			return nil, errors.New("malformed Redis RESP bulk terminator")
		}
		return buf[:n], nil
	default:
		return nil, fmt.Errorf("unsupported Redis reply prefix %q", prefix)
	}
}
func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
