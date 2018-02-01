export GOPATH=${PWD}
build:
	go install eriksuman/ftpserver

run: build
	./bin/ftpserver 8080
