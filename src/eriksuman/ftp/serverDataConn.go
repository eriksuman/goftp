package ftp

import (
	"fmt"
	"net"
)

// serverDataConn is an interface for writing to a data connection
type serverDataConn interface {
	write([]byte) error
}

// serverActiveDataConn is an active data connection which connects to the client.
type serverActiveDataConn struct {
	address string
}

// initActiveDataConn sets up an active connection
func (h *handler) initActiveDataConn(addr string) {
	h.logMessage(fmt.Sprintf("Active data connection ready for %s", addr))
	h.dataConn = &serverActiveDataConn{address: addr}
}

// write connects to the client and writes data, closing the connection when finished.
func (s *serverActiveDataConn) write(msg []byte) error {
	conn, err := net.DialTimeout("tcp", s.address, connTimeout)
	if err != nil {
		return err
	}

	_, err = conn.Write(msg)
	if err != nil {
		return err
	}

	return conn.Close()
}

// serverPassiveDataConn is a passive data connection which listens for connections
type serverPassiveDataConn struct {
	ln net.Listener
	localAddr string
}

// initPassiveDataConn sets up a passive data connection
func (h *handler) initPassiveDataConn() (string, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}

	h.logMessage(fmt.Sprintf("Passive data connection listening on %s", ln.Addr()))
	addr, _, err := net.SplitHostPort(h.conn.RemoteAddr().String())
	h.dataConn = &serverPassiveDataConn{
		ln: ln,
		localAddr: addr,
	}
	return ln.Addr().String(), nil
}

// write accepts a connection from a client and writes data over the connection
func (s *serverPassiveDataConn) write(msg []byte) error {
	conn, err := s.ln.Accept()
	if err != nil {
		return err
	}

	// logic for checking host 
	dip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return err
	}

	if dip != s.localAddr {
		return fmt.Errorf("Unexpeted data client: want %s got %s", s.localAddr, dip)
	}

	_, err = conn.Write(msg)
	if err != nil {
		return err
	}

	return conn.Close()
}
