/*
Copyright 2011 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package runas wraps package rpc, calling methods in child processes
// which drop root.
package runas

import (
	"fmt"
	"io"
	"log"
	"net/rpc"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"syscall"
)

var _ = log.Printf

// Server is the RPC server that is run in the child process.
// Services needed to be exported on Server before
// runas.MaybeRunChildServer() is called, typically early in your main
// package's main().
var Server = rpc.NewServer()

type splitReadWrite struct {
	io.Reader
	io.Writer
}

func (s *splitReadWrite) Close() error {
	if c, ok := s.Reader.(io.Closer); ok {
		c.Close()
	}
	if c, ok := s.Writer.(io.Closer); ok {
		c.Close()
	}
	return nil
}

var doneInit = false

// MaybeRunChildServer does nothing in your parent process but
// takes over the process in the child process to run the
// root-dropping RPC server.
func MaybeRunChildServer() {
	doneInit = true
	if os.Getenv("BECOME_GO_RUNAS_CHILD") != "1" {
		return
	}
	Server.ServeConn(&splitReadWrite{os.Stdin, os.Stdout})
	os.Exit(0)
}

// User returns an rpc Client suitable for talking to Server
// running as the provided user.
func User(username string) (*rpc.Client, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return nil, err
	}
	return UidGid(u.Uid, u.Gid)
}

// UidGid returns an rpc Client suitable for talking to Server
// running as the provided userid and group id.
func UidGid(uid, gid int) (*rpc.Client, error) {
	if !doneInit {
		panic("runas.MaybeRunChildServer() never called")
	}
	binary, _ := filepath.Abs(os.Args[0])
	cmd := exec.Command(binary)
	cmd.Dir = "/"
	cmd.Env = []string{"BECOME_GO_RUNAS_CHILD=1"}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	err = cmd.Start()
	if err != nil {
		panic(err.Error())
	}
	c := rpc.NewClient(&splitReadWrite{stdout, stdin})

	// These are embedded in structs and named with a capital R to make
	// reflect & rpc happy. That way we don't have to export them
	// in our go doc.
	var res struct{ R internalDropResult }
	var req struct{ R internalDropArg }
	req.R.Uid = uid
	req.R.Gid = gid
	err = c.Call("InternalGoRunAs.DropPrivileges", &req, &res)
	if res.R.UidDropped != true || res.R.GidDropped != true {
		return nil, fmt.Errorf("runas: failed to drop root to %d/%d: %v", uid, gid, res)
	}
	return c, nil
}

type internalService struct {
}

type internalDropArg struct {
	Uid, Gid int
}

type internalDropResult struct {
	UidDropped, GidDropped   bool
	SetuidErrno, SetgidErrno uintptr
}

func (s *internalService) DropPrivileges(arg *struct{ R internalDropArg }, result *struct{ R internalDropResult }) error {
	if rv := syscall.Setgid(arg.R.Gid); rv != nil {
		result.R.SetgidErrno = uintptr(rv.(syscall.Errno))
	} else {
		result.R.GidDropped = true
	}
	if rv := syscall.Setuid(arg.R.Uid); rv != nil {
		result.R.SetuidErrno = uintptr(rv.(syscall.Errno))
	} else {
		result.R.UidDropped = true
	}
	return nil
}

func init() {
	Server.RegisterName("InternalGoRunAs", &internalService{})
}
