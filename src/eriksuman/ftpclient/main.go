package main

import (
	"fmt"
	"os"
	
	"eriksuman/ftp"
)

func main() {
	var host, log string
	port := "21"
	if len(os.Args) == 3 {
		host = os.Args[1]
		log = os.Args[2]
	} else if len(os.Args) == 4 {
		host = os.Args[1]
		log = os.Args[2]
		port = os.Args[3]
	} else {
		fmt.Println("Usage: ftpclient <host> <logfile> [port]")
		return
	}
	
	if err := ftp.StartClient(host, port, log); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}