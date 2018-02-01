package ftp

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// Client is an FTP client
type Client struct {
	// local and remote ip addresses
	remoteAddr, localAddr string
	// control connection
	control *controlConn
	// data connection type (active/passive)
	dataConnType dataConnType
	// use extended or legacy pasv/port commands
	extended bool
}

// StartClient bootstraps the ftp client, opening the log file and attempting to connect to host:port.
// The return code from the server is verified and the user is then prompted to sign in and taken
// into the command loop.
func StartClient(host, port, log string) error {
	// open control connection
	cont, rply, localAddr, remoteAddr, err := newControlConn(host, port, log)
	if err != nil {
		return err
	}
	defer cont.Close()

	c := &Client{
		control:    cont,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
		extended:   false,
	}

	// check initial reply code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "220":
		//server ready
	case "120":
		//server not ready, wait for 220
		rply, err = cont.readReply()
		if err != nil {
			return err
		}

		if rply.StatusCode != "220" {
			fmt.Printf("Connection failed: %v\n", rply)
			return nil
		}
	case "421":
		// negative reply, abort
		return nil
	default:
		c.closeAndExit("Unrecognized reply, exiting")
	}

	// attempt to log in user
	if err := c.logIn(); err != nil {
		return err
	}

	// enter command loop
	c.commandLoop()

	return nil
}

// logIn displays the necessary prompts and issues the commands to sign a user in.
func (c *Client) logIn() error {
	// ask user for a username
	fmt.Print("Username: ")
	in := bufio.NewReader(os.Stdin)
	str, err := in.ReadString('\n')
	if err != nil {
		return err
	}

	// issue USER command to server
	rply, err := c.control.getReplyForCommand(newCommand(CommandUSER, str[:len(str)-1]))
	if err != nil {
		return err
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "230":
		// user already logged in
		return nil
	case "500", "501", "421":
		// an error has occurred, exit
		c.control.Close()
		os.Exit(1)
	case "331":
		// need password, continue
	case "332":
		// ACCT not supported, abort
		fmt.Println("Log in with accounts is not supported. Exiting.")
		os.Exit(1)
	default:
		c.closeAndExit("Unrecognized response, exiting")
	}

	// ask user for password
	fmt.Printf("Password: ")
	str, err = in.ReadString('\n')
	if err != nil {
		return err
	}

	// issue PASS command to server
	rply, err = c.control.getReplyForCommand(newCommand(CommandPASS, str[:len(str)-1]))
	if err != nil {
		return err
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "230", "202":
		// logged in, continue
	case "530":
		// incorrect username/password
		c.closeAndExit("Login failed. Exiting.")
	case "500", "503", "421", "332":
		// an error has occurred, exit
		c.closeAndExit("Exiting")
	case "501":
		// bad parameters
		c.closeAndExit("Error in parameters. Exiting.")
	default:
		c.closeAndExit("Unrecognized response, exiting")
	}

	return nil
}

// commandLoop displays a command prompt, reads, and executes commands from the user
func (c *Client) commandLoop() {
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("ftp> ")
		cmd, err := in.ReadString('\n')
		if err != nil {
			fmt.Printf("ftp: %s", err)
			os.Exit(1)
		}

		// remove newline, execute input command
		c.executeCommand(cmd[:len(cmd)-1])
	}
}

// executeCommand attempts to parse command and execute its corresponding method
func (c *Client) executeCommand(command string) {
	// split string, switch on first token
	cmd := strings.Split(strings.ToLower(command), " ")
	switch cmd[0] {
	// change directory
	case "cd":
		if len(cmd) != 2 {
			fmt.Println("Usage: cd <path>")
			return
		}
		c.CommandCD(cmd[1])
	// change directory up
	case "cdup":
		if len(cmd) != 1 {
			fmt.Println("Usage: cdup")
			return
		}
		c.CommandCDUP()
	// print working directory
	case "pwd":
		if len(cmd) != 1 {
			fmt.Println("Usage: pwd")
			return
		}
		c.CommandPWD()
	// current directory listing
	case "ls":
		if len(cmd) > 2 {
			fmt.Println("Usage: ls [path]")
			return
		}
		c.CommandLS("")
	// download a file from server
	case "get":
		if len(cmd) != 2 {
			fmt.Println("Usage: get <filename>")
			return
		}
		c.CommandGet(cmd[1])
	// use passive data connections
	case "pasv", "passive":
		if len(cmd) != 1 {
			fmt.Println("Usage: passive")
			return
		}
		fmt.Println("Switching to passive mode...")
		c.dataConnType = dataConnTypePassive
	// use active data connections
	case "active":
		if len(cmd) != 1 {
			fmt.Println("Usage: active")
			return
		}
		fmt.Println("Switching to active mode...")
		c.dataConnType = dataConnTypeActive
	// turn on and off extended pasv/port commands
	case "ext", "extended":
		if len(cmd) != 2 {
			fmt.Println("Usage: extended <on|off>")
			return
		}
		switch cmd[1] {
		case "on":
			fmt.Println("Extended configuration commands will be preferred.")
			c.extended = true
		case "off":
			fmt.Println("Legacy configuration commands will be preferred.")
			c.extended = false
		default:
			fmt.Println("Usage: extended <on|off>")
		}
	// display help message from server
	case "help":
		if len(cmd) != 1 {
			fmt.Println("Usage: help")
			return
		}
		c.CommandHELP()
	// exit client
	case "exit", "quit":
		if len(cmd) != 1 {
			fmt.Println("Usage: exit")
			return
		}
		c.CommandExit()
	default:
		fmt.Printf("Unrecognized command: %s\n", cmd[0])
	}
}

// openDataConn opens a data connection using the set connection type
// and returns a dataConn interface type
func (c *Client) openDataConn() (clientDataConn, error) {
	switch c.dataConnType {
	case dataConnTypeActive:
		return c.initActiveDataConn()
	case dataConnTypePassive:
		return c.initPassiveDataConn()
	default:
		return nil, fmt.Errorf("unknown dataConnType: %d", c.dataConnType)
	}
}

// initActiveDataConn opens an active data connection listener and issues
// the required port command
func (c *Client) initActiveDataConn() (*activeDataConn, error) {
	// open data connection
	conn, addr, err := newActiveDataConn()
	if err != nil {
		return nil, err
	}

	// get port number of listener
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// get local address of client
	host, _, err := net.SplitHostPort(c.localAddr)
	if err != nil {
		return nil, err
	}

	// issue port command
	if err := c.issuePortCommand(host, port); err != nil {
		return nil, err
	}

	return conn, nil
}

// issuePortCommand issues the proper port command based on the c's extended
// property. If the ip address is ipv5, EPRT is always used.
func (c *Client) issuePortCommand(host, port string) error {
	// get ip type
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("unable to parse IP address: %v", host)
	}

	// check v4/v6
	if ip.To4() != nil {
		if !c.extended {
			return c.CommandPORT(host, port)
		}
	}
	return c.CommandEPRT(host, port)
}

// initPassiveDataConn opens a new passive data connection to the server by
// issuing the proper pasv command and connecting to the port specified by the server
func (c *Client) initPassiveDataConn() (*passiveDataConn, error) {
	var addr string

	if c.extended {
		msg, err := c.CommandEPSV()
		if err != nil {
			return nil, err
		}

		// parse response from server
		port, err := parseEPSVString(msg)
		if err != nil {
			return nil, err
		}

		// get server's remote address
		host, _, err := net.SplitHostPort(c.remoteAddr)
		if err != nil {
			return nil, err
		}

		// build host:port address
		addr = net.JoinHostPort(host, port)
	} else {
		msg, err := c.CommandPASV()
		if err != nil {
			return nil, err
		}

		// parse pasv string
		addr, err = hostPortToAddr(msg)
		if err != nil {
			return nil, err
		}
	}
	return newPassiveDataConn(addr)
}

// closeAndExit closes the connection to the server and exits
func (c *Client) closeAndExit(msg string) {
	if msg != "" {
		fmt.Println(msg)
	}

	c.control.Close()
	os.Exit(1)
}
