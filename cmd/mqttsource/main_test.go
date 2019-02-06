/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package main

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/alanconway/lightning/pkg/http"
	"go.uber.org/zap"
)

var tlogger *zap.Logger

func init() {
	//tlogger, _ = zap.NewDevelopment() // Noisy for test debugging
	tlogger = zap.NewNop() // Queit for CI tests
	tlogger = tlogger.Named("testing")
}

func fatalIf(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func errorIf(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}

// Test adapter with a HTTP server (use a lightning sink) to receive the events
type testCmd struct {
	t       *testing.T
	cmd     *exec.Cmd    // Process running main.go
	httpSrv *http.Source // HTTP server - also a HTTP source for the test
}

func (c *testCmd) Start(mqttURL string) error {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	c.httpSrv = http.NewSource(10, tlogger) // main sends to this  HTTP server
	c.httpSrv.Start(l)

	c.cmd = exec.Command("go", "run", "main.go")
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	c.cmd.Env = append(os.Environ(),
		fmt.Sprintf("SINK_URI=http://%v/%v", l.Addr().String(), "httpout"),
		fmt.Sprintf("MQTT_URI=%v", mqttURL),
	)
	return c.cmd.Start()
}

func (c *testCmd) Close() {
	if c.cmd.Process != nil {
		c.cmd.Process.Signal(os.Kill)
		c.cmd.Wait()
	}
	c.httpSrv.Close()
}

func TestClientSource(t *testing.T) {
	// MQTT broker and client for test
	broker, err := startBroker()
	fatalIf(t, err)
	defer broker.Close()
	u := &url.URL{Scheme: "tcp", Host: broker.Addr, Path: "#",
		User: url.UserPassword("testuser", "testpassword")}
	client, err := newClient(u)
	fatalIf(t, err)

	// Start main.go and its HTTP server
	cmd := testCmd{t: t}
	fatalIf(t, cmd.Start(u.String()))
	defer cmd.Close()

	b := []byte(`{"specversion": "0.2", "contenttype": "text/plain", "data": data}`)
	done := make(chan struct{})
	// Retry send till received - cmd needs time to start up.
	go func() {
		for {
			log.Printf("FIXME sending")
			fatalIf(t, waitErr(client.Publish("/test", 0, true, b)))
			select {
			case <-time.After(time.Second):
			case <-done:
				return
			}
		}
	}()
	m, err := cmd.httpSrv.Receive()
	close(done)
	sm := m.Structured()
	if sm == nil {
		t.Error("expecting structured message")
	}
	if string(b) != string(sm.Bytes) {
		t.Errorf("%s != %s", b, sm.Bytes)
	}
}
