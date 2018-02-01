package main

import (
	"eriksuman/ftp"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: ftpserver <port>")
		return
	}

	port := os.Args[1]
	if err := ftp.StartServer(port); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
}
