package ftp

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var configPath = "ftpserver.config"

type config struct {
	logDir string
	nLogFiles int
	usersFile string
	port bool
	pasv bool
}

func loadConfig(path string) (*config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	s := bufio.NewScanner(f)
	c := &config {
		logDir: "/var/spool/logfiles",
		nLogFiles: 5,
		pasv: true,
	}
	for s.Scan() {
		line := s.Text()
		if line[0] == '#' {
			continue
		}

		setting := strings.Split(line, "=")
		if len(setting) != 2 {
			continue
		}
		
		switch setting[0] {
		case "logdirectory":
			c.logDir = setting[1]
		case "numlogfiles":
			_, err := fmt.Sscanf(setting[1], "%d", &c.nLogFiles)
			if err != nil {
				fmt.Printf("logger: reading log file: %v\n", err)
				c.nLogFiles = 5
				continue
			}
		case "usernamefile":
			c.usersFile = setting[1]
		case "port_mode":
			b, err := parseBool(setting[1])
			if err != nil {
				fmt.Println(err)
				continue
			}
			c.port = b
		case "pasv_mode":
			b, err := parseBool(setting[1])
			if err != nil {
				fmt.Println(err)
				continue
			}
			c.pasv = b
		default:
			fmt.Printf("config.go: unrecognized setting %s\n", line)
		}
	}
	
	if err := s.Err(); err != nil {
		return nil, err
	}

	return c, nil
}

func parseBool(b string) (bool, error) {
	switch strings.ToUpper(b) {
	case "YES":
		return true, nil
	case "NO":
		return false, nil
	default:
		return false, fmt.Errorf("config.go: unrecognized boolean value %s", b)
	}
}