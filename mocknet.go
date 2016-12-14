package mocknet

import (
	"errors"
	"net"
	"strconv"
	"time"
)

// mocknet provides for mock objects for testing purposes. There is a mock
// net.Listener and a mock net.Conn.

// Functions that are intended to be used by the tester begin with Mock.
// For example, if you want the program being tested to read a new message,
// the tester would call MockWrite, and then the program would call Read.

// Listener implements the Listener interface and is used to mock
// a listener for testing purposes. It
type Listener struct {
	// A channel that the tester can use to send new connections to the listener.
	incoming chan net.Conn
	// A channel that is used to disconnect the listener.
	disconnect   chan struct{}
	localAddr    net.Addr
	disconnected bool
}

// Accept waits for and returns the next connection to the listener.
func (l *Listener) Accept() (net.Conn, error) {
	if l.disconnected {
		return nil, errors.New("Listner disconnected.")
	}
	select {
	case <-l.disconnect:
		l.disconnected = true
		return nil, errors.New("Listener disconnected.")
	case m := <-l.incoming:
		return m, nil
	}
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *Listener) Close() error {
	close(l.disconnect)
	return nil
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	return l.localAddr
}

// Send a new connection to the mock listener to be accepted by the test program.
func (l *Listener) MockOpenConnection(conn net.Conn) {
	l.incoming <- conn
}

// NewListener returns a new listener.
func NewListener(localAddr net.Addr, disconnected bool) *Listener {
	return &Listener{
		incoming:     make(chan net.Conn),
		disconnect:   make(chan struct{}),
		localAddr:    localAddr,
		disconnected: disconnected,
	}
}

type ListenerBuilder struct {
	localAddr net.Addr
	listeners []*Listener
}

func (lb *ListenerBuilder) Listen(service, addr string) (net.Listener, error) {
	if len(lb.listeners) == cap(lb.listeners) {
		return nil, errors.New("mock listener builder has reached its capacity.")
	}

	listener := NewListener(lb.localAddr, false)

	lb.listeners = append(lb.listeners, listener)
	return listener, nil
}

func (lb *ListenerBuilder) GetListener(n int) *Listener {
	if n < 0 || n >= len(lb.listeners) {
		return nil
	}

	return lb.listeners[n]
}

func NewListenerBuilder(localAddr net.Addr, capacity int) *ListenerBuilder {
	return &ListenerBuilder{
		localAddr: localAddr,
		listeners: make([]*Listener, 0, capacity),
	}
}

// Conn implements the net.Conn interface and is used to test the
// connection object without connecting to the real internet.
type Conn struct {
	sendChan    chan []byte
	receiveChan chan []byte
	done        chan struct{} // A channel to close the connection.
	outMessage  []byte        // A message ready to be sent back to the real peer.
	outPlace    int           //How much of the outMessage that has been sent.
	localAddr   net.Addr
	remoteAddr  net.Addr
	closed      bool
}

func (c *Conn) Close() error {
	c.closed = true
	close(c.done)
	return nil
}

// LocalAddr returns the localAddr field of the fake connection and satisfies
// the net.Conn interface.
func (c *Conn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr returns the remoteAddr field of the mock connection.
func (c *Conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Read allows the real peer to read message from the mock connection.
func (c *Conn) Read(b []byte) (int, error) {
	if c.closed {
		return 0, errors.New("Connection closed.")
	}

	i := 0
	for i < len(b) {
		if c.outMessage == nil {
			select {
			case <-c.done:
				return 0, errors.New("Connection closed.")
			case c.outMessage = <-c.receiveChan:
			}
			c.outPlace = 0
		}

		for c.outPlace < len(c.outMessage) && i < len(b) {
			b[i] = c.outMessage[c.outPlace]
			c.outPlace++
			i++
		}

		if c.outPlace == len(c.outMessage) {
			c.outMessage = nil
		}
	}

	return i, nil
}

// Write allows the peer to write to the mock connection.
func (c *Conn) Write(b []byte) (n int, err error) {
	if c.closed {
		return 0, errors.New("Connection closed.")
	}

	data := make([]byte, len(b))
	copy(data, b)
	c.sendChan <- data
	return len(b), nil
}

// MockRead is for the mock peer to read a message that has previously
// been written with Write.
func (c *Conn) MockRead() []byte {
	if c.closed {
		return nil
	}

	var b []byte
	select {
	case <-c.done:
		return nil
	case b = <-c.sendChan:
	}

	return b
}

// MockWrite is for the mock peer to write a message that will be read by
// the real peer.
func (c *Conn) MockWrite(b []byte) {
	c.receiveChan <- b
}

// NewConn creates a new mockConn
func NewConn(localAddr, remoteAddr net.Addr, closed bool) *Conn {
	return &Conn{
		localAddr:   localAddr,
		remoteAddr:  remoteAddr,
		sendChan:    make(chan []byte),
		receiveChan: make(chan []byte),
		done:        make(chan struct{}),
		closed:      closed,
	}
}

// Dialer is a function that returns a function with the same type as
// net.Dial but which returns a mock connection.
func Dialer(localAddr net.Addr, fail bool, closed bool) func(service, addr string) (net.Conn, error) {
	return func(service, addr string) (net.Conn, error) {
		if fail {
			return nil, errors.New("Connection failed.")
		}
		host, portstr, _ := net.SplitHostPort(addr)
		port, _ := strconv.ParseInt(portstr, 10, 0)
		return NewConn(localAddr, &net.TCPAddr{IP: net.ParseIP(host), Port: int(port)}, closed), nil
	}
}
