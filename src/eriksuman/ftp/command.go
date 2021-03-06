package ftp

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"os"
	"path"
	"strings"
)

// CommandCode is the character code representing a command
type CommandCode string

// Command code enumeration
const (
	CommandUSER CommandCode = "USER"
	CommandPASS CommandCode = "PASS"
	CommandCWD  CommandCode = "CWD"
	CommandCDUP CommandCode = "CDUP"
	CommandQUIT CommandCode = "QUIT"
	CommandPASV CommandCode = "PASV"
	CommandEPSV CommandCode = "EPSV"
	CommandPORT CommandCode = "PORT"
	CommandEPRT CommandCode = "EPRT"
	CommandRETR CommandCode = "RETR"
	CommandPWD  CommandCode = "PWD"
	CommandLIST CommandCode = "LIST"
	CommandHELP CommandCode = "HELP"
)

// Command is a PDU containing a command to be sent to the server
type Command struct {
	Code     CommandCode
	Arugment string
}

func newCommand(code CommandCode, arg string) *Command {
	return &Command{
		Code:     code,
		Arugment: arg,
	}
}

// String constructs the PDU for a command. \r\n is added by the control connection
func (c Command) String() string {
	if c.Arugment == "" {
		return string(c.Code)
	}

	return fmt.Sprintf("%s %s", c.Code, c.Arugment)
}

// StatusCode is the status code generated by a reply from the FTP server
type StatusCode string

// Reply is the PDU for a reply to a command from an FTP server
type Reply struct {
	StatusCode StatusCode
	Message    string
}

func newReply(s StatusCode, msg string) *Reply {
	return &Reply{
		StatusCode: s,
		Message:    msg,
	}
}

func (r Reply) String() string {
	msg := strings.Trim(r.Message, "\n")
	// check if message contains embedded newlines
	if strings.Contains(msg, "\n") {
		// split on newlines, insert tabs
		a := strings.Split(msg, "\n")
		for i := 0; i < len(a); i++ {
			a[i] = "	" + a[i]
		}

		msg = strings.Join(a, "\r\n") + "\r\n"
		return string(r.StatusCode) + "-\r\n" + msg + string(r.StatusCode) + " Erik's FTP Server"
	}

	return fmt.Sprintf("%s %s", r.StatusCode, r.Message)
}

// Client commands

// CommandCD changes directory to path on the FTP server
func (c *Client) CommandCD(path string) {
	rply, err := c.control.getReplyForCommand(newCommand(CommandCWD, path))
	if err != nil {
		fmt.Printf("An unknown error occurred: %v\n", err)
		return
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "250":
		// success, noop
	case "500", "502", "550":
		// software error
		fmt.Println("Command failed.")
	case "501":
		// user error
		fmt.Println("Error in parameters.")
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}
}

// CommandCDUP switches to the parent directory on the FTP server
func (c *Client) CommandCDUP() {
	rply, err := c.control.getReplyForCommand(newCommand(CommandCDUP, ""))
	if err != nil {
		fmt.Printf("An unknown error occurred: %v\n", err)
		return
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "200", "250":
		//success, noop
	case "500", "502", "550":
		// software error
		fmt.Println("Command failed.")
	case "501":
		// user error
		fmt.Println("Error in parameters.")
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}
}

// CommandPWD requests the current directory from the server
func (c *Client) CommandPWD() {
	rply, err := c.control.getReplyForCommand(newCommand(CommandPWD, ""))
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "257":
		// success, noop
	case "500", "502", "550":
		// software error
		fmt.Println("Command failed.")
	case "501":
		// user error
		fmt.Println("Error in parameters.")
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}
}

// CommandPORT tells the server to connect to host:port for data transmission
func (c *Client) CommandPORT(host, port string) error {
	// build argument for port command
	portArg, err := getPORTString(host, port)
	if err != nil {
		return err
	}

	rply, err := c.control.getReplyForCommand(newCommand(CommandPORT, portArg))
	if err != nil {
		return err
	}

	// check status code
	switch rply.StatusCode {
	case "200":
		// okay, return
		return nil
	case "500", "501", "530":
		// software error
		fmt.Println(rply)
		return errors.New("port command failed")
	case "421":
		// server closed connection
		fmt.Println(rply)
		c.closeAndExit("Exiting.")
	default:
		fmt.Println(rply)
		c.closeAndExit("Unrecognized response. Exiting.")
	}

	return errors.New("unexpected error")
}

// CommandEPRT tells the server to connect to host:port for data transmissions
func (c *Client) CommandEPRT(host, port string) error {
	// build argument for eprt command
	eprtArg, err := getEPRTString(host, port)
	if err != nil {
		return err
	}

	rply, err := c.control.getReplyForCommand(newCommand(CommandEPRT, eprtArg))
	if err != nil {
		return err
	}

	// check status code
	switch rply.StatusCode {
	case "200":
		// okay, return
		return nil
	case "500", "501", "530", "522":
		// software error
		fmt.Println(rply)
		return errors.New("eprt command failed")
	case "421":
		// server closed connection
		fmt.Println(rply)
		c.closeAndExit("Exiting.")
	default:
		fmt.Println(rply)
		c.closeAndExit("Unrecognized response. Exiting.")
	}

	return errors.New("unexpected error")
}

// CommandPASV tells the server to listen on a port for data connections. The message
// returned by the server is returned to the caller
func (c *Client) CommandPASV() (string, error) {
	rply, err := c.control.getReplyForCommand(newCommand(CommandPASV, ""))
	if err != nil {
		return "", err
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "227":
		// okay, return message
		return rply.Message, nil
	case "500", "502", "530":
		// software error
		return "", fmt.Errorf("command failed")
	case "501":
		// user error
		return "", fmt.Errorf("error in parameters")
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}

	return "", errors.New("unexpected error")
}

// CommandEPSV tells the server to listen on a port for data connections. The
// message returned by the server is returned to the caller.
func (c *Client) CommandEPSV() (string, error) {
	rply, err := c.control.getReplyForCommand(newCommand(CommandEPSV, ""))
	if err != nil {
		return "", err
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "229":
		// okay, return message
		return rply.Message, nil
	case "500", "501", "530", "522":
		// software error
		return "", errors.New("epsv command failed")
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}

	return "", errors.New("unexpected error")
}

// CommandLS opens a data connection and issues a command for a directory listing
// to the server. The listing is then pritned to standard out.
func (c *Client) CommandLS(path string) {
	data, err := c.openDataConn()
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
		return
	}

	rply, err := c.control.getReplyForCommand(newCommand(CommandLIST, path))
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
		return
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "125", "150":
		// okay, read from data connection
		msg, err := data.read()
		if err != nil {
			fmt.Printf("Reading from data connection: %v\n", err)
			return
		}
		fmt.Print(string(msg))
	case "450", "500", "502", "530":
		// software error
		fmt.Println("Command failed.")
		return
	case "501":
		// user error
		fmt.Println("Error in parameters.")
		return
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}

	// read a reply from server
	rply, err = c.control.readReply()
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
		return
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "226", "250":
		// success, noop
	case "425", "426", "451":
		// software error
		fmt.Println("Command failed.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}
}

// CommandGet retrieves file from the server using the RETR command. The file is
// saved to the local current directory.
func (c *Client) CommandGet(file string) {
	data, err := c.openDataConn()
	if err != nil {
		fmt.Printf("An unexpected error occurred: %s", err)
		return
	}

	rply, err := c.control.getReplyForCommand(newCommand(CommandRETR, file))
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
		return
	}

	fmt.Println(rply)
	var bytes []byte
	switch rply.StatusCode {
	case "125", "150":
		//success, read from data connection
		bytes, err = data.read()
		if err != nil {
			fmt.Printf("An unexpected error occurred: %s\n", err)
			return
		}
	case "450", "550", "500", "502", "530":
		//software error
		fmt.Println("Command failed.")
		return
	case "501":
		// user error
		fmt.Println("Invalid parameters.")
		return
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}

	// read a reply from the server
	rply, err = c.control.readReply()
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
		return
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "226", "250":
		// retr complete, continue
	case "425", "426", "550":
		// software error
		fmt.Println("Command failed.")
		return
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}

	// write file
	if err := ioutil.WriteFile(path.Base(file), bytes, 0644); err != nil {
		fmt.Printf("Failed to write file: %v\n", err)
		return
	}
}

// CommandHELP asks the server to return it's supported commands
func (c *Client) CommandHELP() {
	rply, err := c.control.getReplyForCommand(newCommand(CommandHELP, ""))
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
		return
	}

	// check status code
	fmt.Println(rply)
	switch rply.StatusCode {
	case "211", "214":
		// success, noop
	case "500", "502":
		// software error
		fmt.Println("Command failed.")
	case "501":
		// user error
		fmt.Println("Error in parameters.")
	case "421":
		// server closed connection
		c.closeAndExit("Exiting.")
	default:
		c.closeAndExit("Unrecognized reply, exiting.")
	}
}

// CommandExit issues a goodbye command to the server and exits the process
func (c *Client) CommandExit() {
	rply, err := c.control.getReplyForCommand(newCommand(CommandQUIT, ""))
	if err != nil {
		fmt.Printf("An unexpected error occurred: %v\n", err)
	} else {
		fmt.Println(rply)
	}

	os.Exit(0)
}

// getPORTString transforms host and port into an argument string for the PORT command
func getPORTString(host, port string) (string, error) {
	hostBytes := strings.Split(host, ".")

	// ensure host is in proper format
	if len(hostBytes) != 4 {
		return "", fmt.Errorf("Invalid address: %s:%s", host, port)
	}

	// make sure port is in range
	var intPort uint16
	fmt.Sscanf(port, "%d", &intPort)
	if intPort > math.MaxUint16 {
		return "", fmt.Errorf("Invalid port: %s:%s", host, port)
	}

	// calculate port bytes
	portBytes := new([2]uint16)
	portBytes[0] = intPort & 255
	portBytes[1] = intPort >> 8

	//convert to string
	portStrs := new([2]string)
	portStrs[0] = fmt.Sprintf("%d", portBytes[0])
	portStrs[1] = fmt.Sprintf("%d", portBytes[1])

	// builld string
	addrString := ""
	for _, s := range hostBytes {
		addrString += s + ","
	}

	addrString += portStrs[1] + "," + portStrs[0]
	return addrString, nil
}

// getEPRTString transforms host and port into an argument string for the EPRT command
func getEPRTString(host, port string) (string, error) {
	// get ip type
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("unrecognized IP address: %s", host)
	}

	// determing protocol type
	var proto string
	if ip.To4() != nil {
		proto = "1"
	} else {
		proto = "2"
		// ftp servers seem to not like the IPv6 localhost address (::1)
		if ip.IsLoopback() {
			proto = "1"
			host = "127.0.0.1"
		}
	}

	// build string
	return "|" + proto + "|" + host + "|" + port + "|", nil
}

// parsePASVString takes a return message from a PASV command and returns the
// address to connect to.
func parsePASVString(msg string) (string, error) {
	// according to RFC, data is of the form (datadatadata)
	// find the index of the '(' and ')'
	strt := strings.IndexByte(msg, '(')
	end := strings.IndexByte(msg, ')')
	if strt == -1 || end == -1 {
		return "", fmt.Errorf("Invalid PASV message: %s", msg)
	}

	// split message on ',' character
	return hostPortToAddr(msg[strt+1 : end])
}

func hostPortToAddr(hostPort string) (string, error) {
	// split message on ',' character
	data := strings.Split(hostPort, ",")
	if len(data) != 6 {
		return "", fmt.Errorf("invalid argument: %s", hostPort)
	}

	// build ip address
	host := data[0] + "." + data[1] + "." + data[2] + "." + data[3]

	// convert port parameters to numeric values
	portData := new([2]uint16)
	fmt.Sscanf(data[4], "%d", &portData[0])
	fmt.Sscanf(data[5], "%d", &portData[1])

	// calculate actual port
	port := portData[0]*256 + portData[1]
	if port > math.MaxUint16 {
		return "", fmt.Errorf("port out of range: %d", port)
	}

	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}

// parseEPSVString takes a message returned by a EPSV command and returns
// the port specified by the server
func parseEPSVString(msg string) (string, error) {
	// according to the RFC, data is of the form (datadatadata)
	// get index of '(' and ')'
	strt := strings.IndexByte(msg, '(')
	end := strings.IndexByte(msg, ')')
	if strt == -1 || end == -1 {
		return "", fmt.Errorf("Invalid EPSV message: %s", msg)
	}

	// trim off the '|'s surrounding port number
	return strings.Trim(msg[strt+1:end], "|"), nil
}
