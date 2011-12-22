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

// An example showing usage of the runas library.

package main

import (
	"log"
	"syscall"

	"github.com/bradfitz/go-runas/runas"
)

type DemoService struct{}

type WhoAmIResult struct {
	Uid, Gid int
}

func (s *DemoService) WhoAmI(unused *bool, res *WhoAmIResult) error {
	res.Uid = syscall.Getuid()
	res.Gid = syscall.Getgid()
	return nil
}

func main() {
	runas.Server.Register(&DemoService{})
	runas.MaybeRunChildServer()

	for _, user := range []string{"nobody", "daemon", "man", "syslog"} {
		var res WhoAmIResult
		client, err := runas.User(user)
		if err != nil {
			log.Printf("failed to get client for user %s: %v", user, err)
			continue
		}
		client.Call("DemoService.WhoAmI", true, &res)
		log.Printf("for runas user %s, got: %#v", user, res)
	}
}
