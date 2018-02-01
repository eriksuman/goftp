package ftp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

// timeout period for establishing a connection
const connTimeout = 5 * time.Second

// controlConn is the connection over which FTP commands are sent and replies
// are received
type controlConn struct {
	conn   io.ReadWriteCloser
	logger io.WriteCloser
}

// newControlConn opens a TCP connection to the given host and port, opens the log file,
// and reads the status of the response
func newControlConn(host, port, logFile string) (*controlConn, *Reply, string, string, error) {
	pc := new(controlConn)
	// all messges that pass through the control connection are logged
	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, nil, "", "", err
	}
	pc.logger = file

	pc.logMessage(fmt.Sprintf("Connecting to %s:%s", host, port))

	// connect to specified server with timeout
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), connTimeout)
	if err != nil {
		return nil, nil, "", "", err
	}
	pc.conn = conn

	// read the reply from the server, return it
	rply, err := pc.readReply()
	return pc, rply, conn.LocalAddr().String(), conn.RemoteAddr().String(), err
}

// Close closes the protocol connection and the log file
func (c *controlConn) Close() error {
	if err := c.conn.Close(); err != nil {
		return err
	}

	return c.logger.Close()
}

// getReplyForCommand issues cmd to the FTP server and waits for a reply. The
// reply is then parsed into a Reply type and returned
func (c *controlConn) getReplyForCommand(cmd *Command) (*Reply, error) {
	if err := c.writeCommand(cmd); err != nil {
		return nil, err
	}

	return c.readReply()
}

// logMessage appends a timestamp and logs msg
func (c *controlConn) logMessage(msg string) {
	fmt.Fprintf(c.logger, "%s: %s\n", time.Now().Format(time.StampMicro), msg)
}

// logSend appends a timestamp and logs a sent message
func (c *controlConn) logSend(msg string) {
	fmt.Fprintf(c.logger, "%s: Sent %s\n", time.Now().Format(time.StampMicro), msg)
}

// logReceive appends a timestamp and logs a received message
func (c *controlConn) logReceive(msg string) {
	fmt.Fprintf(c.logger, "%s: Received %s\n", time.Now().Format(time.StampMicro), msg[:len(msg)-2])
}

// readReply waits for, reads, and parses a message from the ftp server.
// The message is then placed into a Reply type
func (c *controlConn) readReply() (*Reply, error) {
	// regular expression to match the first line in a multiple line response
	multiLineRegex, err := regexp.Compile("^\\d{3}-.*")
	if err != nil {
		return nil, err
	}

	// regular expression to match a single line response
	singleLineRegex, err := regexp.Compile("^\\d{3} .*")
	if err != nil {
		return nil, err
	}

	// read from connection
	reader := bufio.NewReader(c.conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	c.logReceive(line)

	// if single line message, parse and return single line
	if singleLineRegex.MatchString(line) {
		ind := strings.IndexByte(line, ' ')
		rply := &Reply{
			StatusCode: StatusCode(line[:ind]),
			Message:    line[ind+1 : len(line)-1],
		}
		return rply, nil
	// if multi-line message, continue reading until a single line string
	// is matched indicating the end of the message
	} else if multiLineRegex.MatchString(line) {
		ind := strings.IndexByte(line, '-')
		status := line[:ind]
		rply := &Reply{StatusCode: StatusCode(status)}
		for {
			nextLine, err := reader.ReadString('\n')
			if err != nil {
				return nil, err
			}
			c.logReceive(nextLine)

			line += nextLine
			if singleLineRegex.MatchString(nextLine) && nextLine[:3] == status {
				rply.Message = line[ind : len(line)-1]
				return rply, nil
			}
		}
	}

	return nil, fmt.Errorf("a malformed response was recieved from the server")
}

// writeCommand writes a Command type to the server
func (c *controlConn) writeCommand(cmd *Command) error {
	msg := cmd.String()
	c.logSend(msg)
	_, err := c.conn.Write([]byte(msg + "\r\n"))
	return err
}
