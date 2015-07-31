// Copyright 2015 The rkt Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test_netutils

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/coreos/rkt/Godeps/_workspace/src/github.com/hydrogen18/stoppableListener"
)

func HttpServe(addr string, timeout int) error {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	log.Printf("%v: serving on %v\n", hostname, addr)

	originalListener, err := net.Listen("tcp4", addr)
	if err != nil {
		panic(err)
	}
	sl, err := stoppableListener.New(originalListener)
	if err != nil {
		panic(err)
	}

	c := make(chan string)
	go func() {
		// Wait for either timeout or connect from client
		select {
		case <-time.After(time.Duration(timeout) * time.Second):
			{
				log.Printf("%v: Serve timed out after %v seconds\n", hostname, timeout)
			}
		case client := (<-c):
			{
				log.Printf("%v: Serve got a connection from %v\n", hostname, client)
			}
		}
		sl.Stop()
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("%v: Serve got a connection from %v\n", hostname, r.RemoteAddr)
		fmt.Fprintf(w, "%v", hostname)
		c <- r.RemoteAddr
	})
	server := http.Server{}
	err = server.Serve(sl)
	if err != nil && err.Error() == "Listener stopped" {
		err = nil
	}
	return err
}

func HttpGet(addr string) (string, error) {
	log.Printf("Connecting to %v", addr)
	res, err := http.Get(fmt.Sprintf("%v", addr))
	if err != nil {
		log.Fatal(err)
	}
	text, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	return string(text), err
}
