package ftp

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"
)

// common errors
var errInvalidAddrFamily = errors.New("unrecognized address family identifer")

// HandleUSER handles commands setting the username
func (h *handler) HandleUSER(username string) {
	//check args
	if username == "" {
		h.writeError501Args()
		return
	} else if username == h.username && h.isLoggedIn {
		h.writeReply(newReply("230", "User already logged in."))
		return
	}

	h.username = username

	h.writeReply(newReply("331", fmt.Sprintf("Username %v accepted, please provide the password.", username)))
}

// HandlePASS takes a password and checks to see if it is valid for the current user
func (h *handler) HandlePASS(password string) {
	if h.username == "" {
		h.writeReply(newReply("503", "Log in with USER first."))
		return
	}

	// check if user exists and password is vaild.
	pass, exists := h.users[h.username]
	if !exists || password != pass {
		h.writeReply(newReply("530", "Login incorrect."))
		h.username = ""
		return
	}

	h.logMessage(fmt.Sprintf("User %s logged in.", h.username))
	h.initCommandTableLoggedIn()
	h.isLoggedIn = true

	h.writeReply(newReply("230", "Login successful."))
}

// HandlePWD prints the current directory name on the control connection
func (h *handler) HandlePWD(arg string) {
	if arg != "" {
		h.writeError501Args()
		return
	}

	h.writeReply(newReply("257", fmt.Sprintf("\"%s\" is the current directory.", h.dir)))
}

// HandleCWD changes the current directory to dir
func (h *handler) HandleCWD(dir string) {
	// convert to absolute path
	p := dir
	if !path.IsAbs(dir) {
		p = path.Join(h.dir, dir)
	}

	// ensure path is valid
	info, err := os.Lstat(p)
	if err != nil {
		h.logError(err)
		h.writeReply(newReply("550", "Directory change failed."))
		return
	}

	// ensure path is directory
	if !info.IsDir() {
		h.writeReply(newReply("550", fmt.Sprintf("%s: Not a directory.", dir)))
		return
	}

	h.dir = p

	h.writeReply(newReply("250", "Directory change successful."))
}

// HandleCDUP changes to the parent directory
func (h *handler) HandleCDUP(arg string) {
	if arg != "" {
		h.writeError501Args()
		return
	}

	h.HandleCWD("..")
}

// HandlePORT handles port commands
func (h *handler) HandlePORT(args string) {
	if !h.config.port {
		h.writeReply(newReply("550", "PORT mode not available."))
		return
	}

	// convert arg to addr
	addr, err := hostPortToAddr(args)
	if err != nil {
		h.logError(err)
		h.writeError501Args()
		return
	}

	// set up active connection
	h.initActiveDataConn(addr)
	h.writeReply(newReply("200", "PORT command accepted."))
}

// HandleEPRT handles eprt commands
func (h *handler) HandleEPRT(args string) {
	if !h.config.port {
		h.writeReply(newReply("550", "EPRT mode not available"))
		return
	}

	// convert arg to addr
	addr, err := parseEPRTArg(args)
	if err != nil {
		h.logError(err)
		if err == errInvalidAddrFamily {
			h.writeReply(newReply("522", "Unrecognized address family identifier."))
			return
		}

		h.writeError501Args()
		return
	}

	// set up active data conn
	h.initActiveDataConn(addr)
	h.writeReply(newReply("200", "EPRT command accepted."))
}

// HandlePASV handles pasv commands
func (h *handler) HandlePASV(arg string) {
	if !h.config.pasv {
		h.writeReply(newReply("550", "PASV mode not available"))
		return
	}

	if arg != "" {
		h.writeError501Args()
		return
	}

	// set up passive connection
	addr, err := h.initPassiveDataConn()
	if err != nil {
		h.logError(err)
		h.writeError421Server()
		return
	}

	// get host and port
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		h.logError(err)
		h.writeError421Server()
		return
	}

	ip := net.ParseIP(addr)

	// port can only be used with IPv4 addresses
	if ip.To4() == nil {
		h.writeReply(newReply("421", "PASV failed, use EPSV."))
		return
	}

	// make proper reply message
	msg, err := getPORTString(host, port)
	if err != nil {
		h.logError(err)
		h.writeError421Server()
		return
	}

	h.writeReply(newReply("227", fmt.Sprintf("Entering Passive Mode (%s).", msg)))
}

// HandleEPSV handles epsv commands
func (h *handler) HandleEPSV(arg string) {
	if !h.config.pasv {
		h.writeReply(newReply("550", "PASV mode not available"))
		return
	}

	if arg != "" {
		h.writeError501Args()
		return
	}

	// set up passive connection
	addr, err := h.initPassiveDataConn()
	if err != nil {
		h.logError(err)
		h.writeReply(newReply("421", "EPSV command failed."))
		return
	}

	// get port
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		h.logError(err)
		h.writeReply(newReply("421", "EPSV command failed."))
		return
	}

	h.writeReply(newReply("229", fmt.Sprintf("Entering Extended Passive Mode (|||%s|).", port)))
}

// HandleLIST writes the given directory listing to the data connection
func (h *handler) HandleLIST(dir string) {
	// make sure path is absolute
	var p string
	if dir == "" {
		p = h.dir
	} else {
		if path.IsAbs(dir) {
			p = dir
		} else {
			p = path.Join(h.dir, dir)
		}
	}

	// make sure directory exists
	f, err := os.Lstat(p)
	if err != nil {
		h.logError(err)
		h.writeReply(newReply("550", "Directory listing failed."))
		return
	}

	// make sure it is a directory
	if !f.IsDir() {
		h.writeReply(newReply("550", fmt.Sprintf("%s: not a directory", dir)))
		return
	}

	// execute ls command to get directory listing
	list, err := exec.Command("ls", "-l", p).Output()
	if err != nil {
		h.logError(err)
		h.writeReply(newReply("550", "Directory listing failed."))
		return
	}

	// replace bare newlines with <CRLF>
	data := strings.Replace(string(list), "\n", "\r\n", -1)

	h.writeReply(newReply("150", "Here comes the directory listing."))

	// write listing to data connection
	if err := h.dataConn.write([]byte(data)); err != nil {
		h.logError(err)
		h.writeReply(newReply("451", "Failed to open data connection."))
		return
	}

	h.writeReply(newReply("226", "Listing successfully transfered."))
}

// HandleRETR writes the given file to the data connection
func (h *handler) HandleRETR(file string) {
	// make sure path is absolute
	if !path.IsAbs(file) {
		file = path.Join(h.dir, file)
	}

	// make sure file exists
	f, err := os.Lstat(file)
	if err != nil {
		h.writeError550FileAction()
		return
	}

	// make sure its a file
	if !f.Mode().IsRegular() {
		h.writeError550FileAction()
		return
	}

	// read file
	data, err := ioutil.ReadFile(file)
	if err != nil {
		h.logError(err)
		h.writeError550FileAction()
		return
	}

	// replace bare newlines with <CRLF>
	data = []byte(strings.Replace(string(data), "\n", "\r\n", -1))

	h.writeReply(newReply("150", "Here comes the file."))

	// write to data connection
	if err = h.dataConn.write(data); err != nil {
		h.logError(err)
		h.writeReply(newReply("451", "Error occurred in transfer."))
		return
	}

	h.writeReply(newReply("226", "File transfered successfully."))
}

// CommandHELP writes a multi line help message
func (h *handler) HandleHELP(arg string) {
	if arg != "" {
		h.writeError501Args()
		return
	}

	msg := "The following commands are recogized:\n" +
		"USER   PASS   CWD    CDUP   PWD\n" +
		"PASV   EPSV   PORT   EPRT   RETR\n" +
		"LIST   HELP   QUIT"

	h.writeReply(newReply("214", msg))
}

// HandleQUIT closes the connecction and writes a goodbye message
func (h *handler) HandleQUIT(arg string) {
	h.writeReply(newReply("221", "Goodbye."))
}

// parseEPRTArg creates an address out of an eprt command argument
func parseEPRTArg(arg string) (string, error) {
	// figure out delimiter, split argument
	delim := string(arg[0])
	params := strings.Split(strings.Trim(arg, delim), "|")
	if len(params) != 3 {
		return "", fmt.Errorf("invalid EPRT string: %s", arg)
	}

	// check addr family
	if params[0] != "1" && params[0] != "2" {
		return "", errInvalidAddrFamily
	}

	return net.JoinHostPort(params[1], params[2]), nil
}
