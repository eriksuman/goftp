package ftp

import (
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"
)

const (
	logFileNameBase  = "ftpsrv"
	logFileExtension = ".log"
	currentFileName  = logFileNameBase + logFileExtension
)

type logger interface {
	logMessage(msg string)
	logSend(msg string)
	logReceive(msg string)
	logError(err error)
	close() error
}

type rolledLogger struct {
	currentFile io.WriteCloser
	lock        sync.Locker
}

func newRolledLogger(dirPath string, max int) (*rolledLogger, error) {
	if err := rollFiles(dirPath, 0, max); err != nil {
		return nil, err
	}

	if dir, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.Mkdir(dirPath, 0777); err != nil {
			return nil, err
		}
	} else {
		p := path.Join(dirPath, currentFileName)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			new := path.Join(dirPath, fmt.Sprintf("%s-%03d%s", logFileNameBase, 0, logFileExtension))
			if err := os.Rename(p, new); err != nil {
				return nil, err
			}
		}
	}

	l, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	return &rolledLogger{
		currentFile: l,
		lock:        new(sync.Mutex),
	}, nil
}

// logMessage apends a timestamp and logs a message
func (r *rolledLogger) logMessage(msg string) {
	r.lock.Lock()
	fmt.Fprintf(r.currentFile, "%s: %s\n", time.Now().Format(time.StampMicro), msg)
	r.lock.Unlock()
}

// logSend appends a timestamp and logs a sent message
func (r *rolledLogger) logSend(msg string) {
	r.lock.Lock()
	fmt.Fprintf(r.currentFile, "%s: Sent %s\n", time.Now().Format(time.StampMicro), msg)
	r.lock.Unlock()
}

// logReceive appends a timestamp and logs a received message
func (r *rolledLogger) logReceive(msg string) {
	r.lock.Lock()
	fmt.Fprintf(r.currentFile, "%s: Received %s\n", time.Now().Format(time.StampMicro), msg[:len(msg)-2])
	r.lock.Unlock()
}

// logError appends a timestamp and logs an error
func (r *rolledLogger) logError(err error) {
	r.lock.Lock()
	fmt.Fprintf(r.currentFile, "%s: Error: %v\n", time.Now().Format(time.StampMicro), err)
	r.lock.Unlock()
}

func (r *rolledLogger) close() error {
	return r.currentFile.Close()
}

func rollFiles(dir string, current, max int) error {
	cur := path.Join(dir, fmt.Sprintf("%s-%03d%s", logFileNameBase, current, logFileExtension))
	// base case
	if _, err := os.Stat(cur); os.IsNotExist(err) || current == max {
		return nil
	}

	if err := rollFiles(dir, current+1, max); err != nil {
		return err
	}

	new := path.Join(dir, fmt.Sprintf("%s-%03d%s", logFileNameBase, current+1, logFileExtension))
	return os.Rename(cur, new)
}
