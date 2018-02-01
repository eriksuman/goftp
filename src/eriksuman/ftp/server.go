package ftp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

// common errors
var errTimeout = errors.New("timeout reached, connection closed")
var errDataConnNotSetUp = errors.New("data connection not set up")

// StartServer starts up the server listening on port
func StartServer(port string) error {
	config, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	l, err := newRolledLogger(config.logDir, config.nLogFiles)
	if err != nil {
		return err
	}
	defer l.close()

	if !config.pasv && !config.port {
		err := errors.New("ftpserver: port_mode and pasv_mode cannot both be NO")
		l.logError(err)
		return err
	}

	// populate users
	u, err := ioutil.ReadFile(config.usersFile)
	if err != nil {
		l.logError(err)
		return err
	}

	lines := strings.Split(string(u), "\n")
	users := make(map[string]string)
	for _, l := range lines {
		user := strings.Split(l, " ")
		if len(user) != 2 {
			continue
		}
		users[user[0]] = user[1]
	}

	// create listener
	ln, err := net.Listen("tcp", net.JoinHostPort("", port))
	if err != nil {
		l.logError(err)
		return err
	}

	//listen loop
	for {
		conn, err := ln.Accept()
		if err != nil {
			l.logError(err)
			return err
		}

		handler, err := newHandler(conn, l, config, users)
		if err != nil {
			l.logError(err)
			conn.Close()
			continue
		}

		go handler.handle()
	}
}

// hanldeFunc is a function pointer which handles a specific command
type handleFunc func(string)

type handler struct {
	config *config
	// control connection
	conn net.Conn
	// log file
	logger logger
	// username of currently loged in user, current directory
	username, dir string
	// data connection
	dataConn serverDataConn
	// map of available users
	users map[string]string
	// logged in flag
	isLoggedIn bool
	// map of command codes to handleFunc functions
	commands map[CommandCode]handleFunc
}

// newHandler creates a new handler for a client
func newHandler(conn net.Conn, l logger, c *config, users map[string]string) (*handler, error) {
	// get current directory
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// create a new handler object
	h := &handler{
		config:     c,
		conn:       conn,
		logger:     l,
		dir:        dir,
		users:      users,
		isLoggedIn: false,
		commands:   make(map[CommandCode]handleFunc),
	}

	h.logMessage(fmt.Sprintf("Accepted connection from %v", h.conn.RemoteAddr()))

	// initialize commands for not logged in state
	h.initCommandTable()

	//initialize default data connection
	if h.config.pasv {
		h.initPassiveDataConn()
	} else {
		// calculate default data port
		host, port, err := net.SplitHostPort(conn.RemoteAddr().String())
		if err != nil {
			return nil, err
		}

		var portNum int
		_, err = fmt.Sscanf(port, "%d", &portNum)
		if err != nil {
			return nil, err
		}

		portNum++
		port = fmt.Sprintf("%d", portNum)
		h.initActiveDataConn(net.JoinHostPort(host, port))
	}

	return h, nil
}

// logMessage appends a timestamp and logs msg
func (h *handler) logMessage(msg string) {
	h.logger.logMessage(msg)
}

// logSend appends a timestamp and logs a sent message
func (h *handler) logSend(msg string) {
	h.logger.logSend(msg)
}

// logReceive appends a timestamp and logs a received message
func (h *handler) logReceive(msg string) {
	h.logger.logReceive(msg)
}

// logError appends a timestamp and logs an error
func (h *handler) logError(err error) {
	h.logger.logError(err)
}

// readCommand reads from the control connection and translates into a Command. If no commands are
// received in 2 minutes, the connection times out.
func (h *handler) readCommand() (*Command, error) {
	// spin off goroutine for listener on connection
	msgChan := make(chan string)
	errChan := make(chan error)
	go func() {
		reader := bufio.NewReader(h.conn)
		msg, err := reader.ReadString('\n')
		if err != nil {
			errChan <- err
			return
		}

		msgChan <- msg
	}()

	// wait for command or timeout
	timer := time.After(2 * time.Minute)
	var msg string
	select {
	case msg = <-msgChan:
		//continue
	case err := <-errChan:
		return nil, err
	case <-timer:
		return nil, errTimeout
	}

	h.logReceive(msg)

	// make sure command syntax is valid
	commandRegex, err := regexp.Compile("^[a-zA-Z]{3,4} *.*")
	if err != nil {
		return nil, err
	}

	if !commandRegex.MatchString(msg) {
		return nil, fmt.Errorf("Unrecognized command: %s", strings.Trim(msg, "\r\n"))
	}

	// parse command
	ind := strings.IndexByte(msg, ' ')
	var code, arg string
	if ind <= 0 {
		code = strings.Trim(msg, "\r\n")
		arg = ""
	} else {
		code = msg[:ind]
		arg = strings.Trim(msg[ind+1:], "\r\n")
	}

	return &Command{
		Code:     CommandCode(code),
		Arugment: arg,
	}, nil
}

// need to handle multi line replies?
func (h *handler) writeReply(r *Reply) error {
	msg := r.String()
	h.logSend(msg)
	_, err := h.conn.Write([]byte(msg + "\r\n"))
	return err
}

// general use error replies

func (h *handler) writeError501Args() {
	h.writeReply(newReply("501", "Error in arguments."))
}

func (h *handler) writeError500Syntax(cmd string) {
	h.writeReply(newReply("500", fmt.Sprintf("%s: command not understood.", cmd)))
}

func (h *handler) writeError550FileAction() {
	h.writeReply(newReply("550", "File action failed."))
}

func (h *handler) writeError425DataConn() {
	h.writeReply(newReply("425", "Failed to open data connection."))
}

func (h *handler) writeError530NotLoggedIn(arg string) {
	h.writeReply(newReply("530", "Log in with USER and PASS first."))
}

func (h *handler) writeError421Server() {
	h.writeReply(newReply("421", "An internal error occurred."))
}

// handle handles a connection to a specific client. It interprets and executes commands in a loop
func (h *handler) handle() {
	// close connection on return
	defer h.Close()

	// send welcome message
	h.writeReply(newReply("220", "Welcome to Erik's FTP Server"))

	for {
		// get a command from client
		cmd, err := h.readCommand()
		if err != nil {
			// client terminated conn
			if err == io.EOF {
				h.logMessage(fmt.Sprintf("Connection to %s closed", h.conn.RemoteAddr()))
				return
			}

			// timeout occurred
			if err == errTimeout {
				h.writeReply(newReply("421", "Timeout."))
				return
			}

			h.logError(fmt.Errorf("reading command: %v", err))
			h.writeReply(newReply("500", "Unrecognized command."))
			continue
		}

		// check for quit command
		cmd.Code = CommandCode(strings.ToUpper(string(cmd.Code)))
		if cmd.Code == "QUIT" {
			h.HandleQUIT(cmd.Arugment)
			return
		}

		// see if command is in command table, execute it
		command, exists := h.commands[cmd.Code]
		if !exists {
			h.writeReply(newReply("500", fmt.Sprintf("%s: command not recognized.", cmd.Code)))
			continue
		}

		command(cmd.Arugment)
	}
}

// initCommandTable initializes the command table to the not logged in state allowing only login and
// help commands. All other commands result in an error reply
func (h *handler) initCommandTable() {
	h.commands[CommandUSER] = h.HandleUSER
	h.commands[CommandPASS] = h.HandlePASS
	h.commands[CommandHELP] = h.HandleHELP
	h.commands[CommandPWD] = h.writeError530NotLoggedIn
	h.commands[CommandCWD] = h.writeError530NotLoggedIn
	h.commands[CommandCDUP] = h.writeError530NotLoggedIn
	h.commands[CommandPORT] = h.writeError530NotLoggedIn
	h.commands[CommandEPRT] = h.writeError530NotLoggedIn
	h.commands[CommandPASV] = h.writeError530NotLoggedIn
	h.commands[CommandEPSV] = h.writeError530NotLoggedIn
	h.commands[CommandLIST] = h.writeError530NotLoggedIn
	h.commands[CommandRETR] = h.writeError530NotLoggedIn
	h.commands[CommandQUIT] = h.HandleQUIT
}

// initCommandTableLoggedIn initializes the command table to the logged in state giving the
// user full functionality.
func (h *handler) initCommandTableLoggedIn() {
	h.commands[CommandUSER] = h.HandleUSER
	h.commands[CommandPASS] = h.HandlePASS
	h.commands[CommandHELP] = h.HandleHELP
	h.commands[CommandPWD] = h.HandlePWD
	h.commands[CommandCWD] = h.HandleCWD
	h.commands[CommandCDUP] = h.HandleCDUP
	h.commands[CommandPORT] = h.HandlePORT
	h.commands[CommandEPRT] = h.HandleEPRT
	h.commands[CommandPASV] = h.HandlePASV
	h.commands[CommandEPSV] = h.HandleEPSV
	h.commands[CommandLIST] = h.HandleLIST
	h.commands[CommandRETR] = h.HandleRETR
	h.commands[CommandQUIT] = h.HandleQUIT
}

// Close closes the logfile and connection.
func (h *handler) Close() error {
	h.logMessage(fmt.Sprintf("Closing connection to %v", h.conn.RemoteAddr()))
	return h.conn.Close()
}
