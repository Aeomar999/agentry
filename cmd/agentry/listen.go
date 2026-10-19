package main

import (
	"bufio"
	"errors"
	"net"
	"sync"
	"time"
)

// With -https, TLS and plain HTTP share the one port, so a link someone
// already has (http://box:7800) still reaches the server, and is sent on to
// https. Each connection is sniffed: a TLS connection starts with a
// handshake record, whose first byte is 0x16, and plain HTTP starts with a
// method's letters. The first byte is read without losing it, and the
// connection handed to one of two listeners.

// tlsHandshake is the first byte of a TLS handshake record.
const tlsHandshake = 0x16

// sniffTimeout is how long a new connection may take to send its first
// byte. Sniffing runs beside Accept, so a slow client holds up only itself.
const sniffTimeout = 5 * time.Second

// splitTLS accepts on l and hands each connection to tlsConns when it starts
// a TLS handshake, else to plainConns. Closing either closes l, and both.
func splitTLS(l net.Listener) (tlsConns, plainConns net.Listener) {
	closed := make(chan struct{})
	var once sync.Once
	stop := func() error {
		var err error
		once.Do(func() {
			close(closed)
			err = l.Close()
		})
		return err
	}
	tlsQueue := &connQueue{addr: l.Addr(), conns: make(chan net.Conn), closed: closed, stop: stop}
	plainQueue := &connQueue{addr: l.Addr(), conns: make(chan net.Conn), closed: closed, stop: stop}
	go func() {
		defer stop()
		for {
			conn, err := l.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err != nil { // out of file descriptors and the like: wait, as net/http does
				time.Sleep(50 * time.Millisecond)
				continue
			}
			go route(conn, tlsQueue, plainQueue)
		}
	}()
	return tlsQueue, plainQueue
}

// route reads conn's first byte and queues it where it belongs, closing a
// connection that sends nothing in time.
func route(conn net.Conn, tlsQueue, plainQueue *connQueue) {
	conn.SetReadDeadline(time.Now().Add(sniffTimeout))
	reader := bufio.NewReader(conn)
	first, err := reader.Peek(1)
	if err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{}) // the server sets its own from here
	queue := plainQueue
	if first[0] == tlsHandshake {
		queue = tlsQueue
	}
	queue.hand(&sniffedConn{Conn: conn, reader: reader})
}

// sniffedConn is a connection whose first bytes were read into reader.
type sniffedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *sniffedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }

// connQueue is a listener whose connections another listener accepted.
type connQueue struct {
	addr   net.Addr
	conns  chan net.Conn
	closed chan struct{}
	stop   func() error
}

func (q *connQueue) hand(conn net.Conn) {
	select {
	case q.conns <- conn:
	case <-q.closed:
		conn.Close()
	}
}

func (q *connQueue) Accept() (net.Conn, error) {
	select {
	case conn := <-q.conns:
		return conn, nil
	case <-q.closed:
		return nil, net.ErrClosed
	}
}

func (q *connQueue) Close() error   { return q.stop() }
func (q *connQueue) Addr() net.Addr { return q.addr }
