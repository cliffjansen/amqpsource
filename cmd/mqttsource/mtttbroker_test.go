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
	"context"
	"net"
	"net/url"

	"github.com/DrmagicE/gmqtt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type broker struct {
	server *gmqtt.Server
	Addr   string
}

func startBroker() (*broker, error) {
	b := &broker{server: gmqtt.NewServer()}
	if l, err := net.Listen("tcp", ":0"); err != nil {
		return nil, err
	} else {
		b.server.AddTCPListenner(l)
		b.Addr = l.Addr().String()
		b.server.Run()
	}
	return b, nil
}

func (b *broker) Close() {
	b.server.Stop(context.Background())
}

func newClient(u *url.URL) (mqtt.Client, error) {
	username := u.User.Username()
	password, _ := u.User.Password()
	opts := mqtt.NewClientOptions().SetClientID("test-sender").AddBroker(u.Host)
	opts.SetUsername(username).SetPassword(password)
	c := mqtt.NewClient(opts)
	return c, waitErr(c.Connect())
}

func waitErr(t mqtt.Token) error { t.Wait(); return t.Error() }
