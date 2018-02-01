package ftp

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"
)

// timeout for data reads
const dataReadTimeout = 10 * time.Second

// clientDataConn is an interface for a data connection
type clientDataConn interface {
	read() ([]byte, error)
}

// dataConnType represents a data connection type (active or passive)
type dataConnType int

// enumeration for dataConnType
const (
	dataConnTypeActive dataConnType = iota
	dataConnTypePassive
	dataConnTypeInvalid
)

// activeDataConn listens on the specified port and waits for the FTP server to
// initiate a data connection
type activeDataConn struct {
	dataChan chan []byte
}

// newActiveDataConn initializes an active data connection by opening a listener on a
// random port and returning it and its address
func newActiveDataConn() (*activeDataConn, string, error) {
	dc := new(activeDataConn)
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, "", err
	}

	dataChan := make(chan []byte, 5)
	dc.dataChan = dataChan
	go dc.waitForConn(ln)
	return dc, ln.Addr().String(), nil
}

// read reads a raw message from the active data connection
func (d *activeDataConn) read() ([]byte, error) {
	t := time.After(dataReadTimeout)
	select {
	case msg := <-d.dataChan:
		return msg, nil
	case <-t:
		return nil, fmt.Errorf("Error: read timeout exceeeded")
	}
}

// waitForConn concurrently waits for the server to connect and send data.
// the data is then passed to read via d's data channel
func (d *activeDataConn) waitForConn(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		fmt.Printf("A fatal error has occurred: %s\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	msg, err := ioutil.ReadAll(conn)
	if err != nil {
		fmt.Printf("Failed to read from active data connection: %v", err)

	}

	d.dataChan <- msg
}

// passiveDataConn connects to the specified address and port on the FTP server
// and waits for a data transmission
type passiveDataConn struct {
	conn net.Conn
}

// newPassiveDataConn connects to addr and returns the connection
func newPassiveDataConn(addr string) (*passiveDataConn, error) {
	conn, err := net.DialTimeout("tcp", addr, connTimeout)
	if err != nil {
		return nil, err
	}

	return &passiveDataConn{conn: conn}, nil
}

// read reads raw data from the pasive data connection
func (d *passiveDataConn) read() ([]byte, error) {
	defer d.conn.Close()

	return ioutil.ReadAll(d.conn)
}
