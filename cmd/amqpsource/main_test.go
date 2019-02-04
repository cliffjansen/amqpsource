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
	"net"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/alanconway/lightning/pkg/amqp"
	"github.com/alanconway/lightning/pkg/http"
	"github.com/alanconway/lightning/pkg/lightning"
	"go.uber.org/zap"
	qamqp "qpid.apache.org/amqp"
	"qpid.apache.org/electron"
)

var tlogger *zap.Logger

func init() {
	tlogger, _ = zap.NewDevelopment() // Noisy for test debugging
	// tlogger = zap.NewNop()
	tlogger = tlogger.Named("testing")
}

func makeEvent(data string) lightning.Event {
	return lightning.Event{"specversion": "0.2", "contenttype": "text/plain", "data": []byte(data)}
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

func (c *testCmd) start(amqpURL string, credit int, server bool) error {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	c.httpSrv = http.NewSource(1, tlogger) // main sends to this  HTTP server
	c.httpSrv.Start(l)

	c.cmd = exec.Command("go", "run", "main.go")
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	c.cmd.Env = append(os.Environ(),
		fmt.Sprintf("SINK_URI=http://%v/%v", l.Addr().String(), "httpout"),
		fmt.Sprintf("AMQP_URI=%v", amqpURL),
		fmt.Sprintf("AMQP_CREDIT=%v", credit),
		fmt.Sprintf("AMQP_SERVER=%v", server),
	)
	if err := c.cmd.Start(); err != nil {
		return err
	}
	return nil
}

func (c *testCmd) close() {
	if c.cmd.Process != nil {
		c.cmd.Process.Signal(os.Kill)
		c.cmd.Wait()
	}
	c.httpSrv.Close()
}

func TestClientSource(t *testing.T) {
	// AMQP sink server, main.go will subscribe to this
	amqpSrv, err := amqp.NewServerSink("tcp", ":0", 10, tlogger)
	defer amqpSrv.Close()
	amqpURL := fmt.Sprintf("amqp://%v/%v", amqpSrv.Listeners()[0].Addr().String(), "amqpin")

	// main.go and its HTTP server
	cmd := testCmd{t: t}
	fatalIf(t, cmd.start(amqpURL, 100, false))
	defer cmd.close()

	// Send and receive cmd binary event message
	errorIf(t, amqpSrv.Send(makeEvent("binary")))
	m, err := cmd.httpSrv.Receive()
	errorIf(t, err)
	if s := m.Structured(); s != nil {
		t.Error("expecting binary message: ", m.Structured().Format.Name(), string(m.Structured().Bytes))
	}
	e, err := m.Event()
	errorIf(t, err)
	if !reflect.DeepEqual(makeEvent("binary"), e) {
		t.Errorf("%v != %v", makeEvent("binary"), e)
	}

	// Send and receive cmd structured event message
	se := []byte(`{"specversion": "0.2", "contenttype": "text/plain", "data": data}`)
	sm := lightning.Structured{
		Format: lightning.JSONFormat,
		Bytes:  se,
	}
	err = amqpSrv.Send(&sm)
	errorIf(t, err)
	m, err = cmd.httpSrv.Receive()
	errorIf(t, err)
	if nil == m.Structured() {
		t.Error("expecting structured message")
	}
	if string(se) != string(sm.Bytes) {
		t.Errorf("%s != %s", se, sm.Bytes)
	}
}

// Get an ephemeral listening port that will hopefully still be available when we use it.
func listenAddr() (string, error) {
	if l, err := net.Listen("tcp", ":0"); err != nil {
		return "", err
	} else {
		defer l.Close()
		return l.Addr().String(), nil
	}
}

func TestServerSource(t *testing.T) {
	// Start main.go in AMQP server mode
	addr, err := listenAddr()
	fatalIf(t, err)
	cmd := testCmd{t: t}
	fatalIf(t, cmd.start(fmt.Sprintf("//%v", addr), 100, true))
	defer cmd.close()

	ac, err := electron.Dial("tcp", addr)
	for lightning.IsRefused(err) { // Retry till cmd is listening
		ac, err = electron.Dial("tcp", addr)
	}
	fatalIf(t, err)
	defer ac.Close(nil)
	s, err := ac.Sender()
	fatalIf(t, err)

	// Send and receive binary event message
	am := qamqp.NewMessageWith("binary")
	am.SetContentType("text/plain")
	am.SetProperties(map[string]interface{}{
		"cloudEvents:specversion": "0.2",
		"not-for-cloudevents":     "binary",
	})
	errorIf(t, s.SendSync(am).Error)

	m, err := cmd.httpSrv.Receive()
	errorIf(t, err)
	if s := m.Structured(); s != nil {
		t.Error("expecting binary message: ", m.Structured().Format.Name(), string(m.Structured().Bytes))
	}
	e, err := m.Event()
	errorIf(t, err)
	if !reflect.DeepEqual(makeEvent("binary"), e) {
		t.Errorf("%v != %v", makeEvent("binary"), e)
	}

	// Send and receive cmd structured event message
	se := []byte(`{"specversion": "0.2", "contenttype": "text/plain", "data": data}`)
	am = qamqp.NewMessageWith(se)

	am.SetContentType(lightning.JSONFormat.Name())
	am.SetProperties(map[string]interface{}{
		"not-for-cloudevents": "structured",
	})
	errorIf(t, s.SendSync(am).Error)

	m, err = cmd.httpSrv.Receive()
	errorIf(t, err)
	sm := m.Structured()
	if sm == nil {
		t.Error("expecting structured message")
	}
	if string(se) != string(sm.Bytes) {
		t.Errorf("%s != %s", se, sm.Bytes)
	}
}
