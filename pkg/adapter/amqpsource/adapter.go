/*
Copyright 2018 The Knative Authors

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

package amqpsource

import (
	"context"
	"fmt"
	"github.com/knative/pkg/logging"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"os"
	"log"
	"strings"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"net"
	"encoding/base64"

	"github.com/knative/pkg/cloudevents"

	// Imports the Qpid AMQP Go client
	"qpid.apache.org/amqp"
	"qpid.apache.org/electron"
)

type Adapter struct {
	// URI-eske connection and address info to attach to the AMQP endpoint
	// (confusingly also a "source") via AMQP or AMQPS protocol.
	// TODO(cliffjansen): json in env or file for full control and auth capabilities
	SourceURI string
	// SinkURI is the URI messages will be forwarded to as CloudEvents via HTTP(S).
	SinkURI string
	Credit uint
	InsecureTlsConnection bool
	// Only needed if using TLS and default root CAs in container do not suffice.
	RootCA string
}

var msgCount = int64(0)

// Run creates a single AMQP connection/session/receiver to read messages, convert each to
// a cloudevent and delivers to the sink.
func (a *Adapter) Start() error {
	//logger := logging.FromContext(context.TODO())
	// ZZZ ??? do we fail fast if problems connecting/receiving or do our own backoff retry loop?
	// set up signals so we handle the first shutdown signal gracefully

	log.Printf("Start with : %s", a.SourceURI)
	// ZZZ getpid not unique enough...
	container := electron.NewContainer(fmt.Sprintf("amqp_event_source_002_[%v]", os.Getpid()))
	u, err := amqp.ParseURL(a.SourceURI)
	fatalIf(err)
	log.Printf("Dial")
	tcpconn, err := a.dial(u)// ZZZ container.Dial("tcp", u.Host) // NOTE: Dial takes just the Host part of the URL
	fatalIf(err)
	amqpconn, err := container.Connection(tcpconn)
	fatalIf(err)

	addr := strings.TrimPrefix(u.Path, "/")
	opts := []electron.LinkOption{electron.Source(addr)}
	opts = append(opts, electron.Capacity(int(a.Credit)), electron.Prefetch(true))
	log.Printf("Create receiver")
	r, err := amqpconn.Receiver(opts...)
	fatalIf(err)
	log.Printf("Receive")
	for {
		if rm, err := r.Receive(); err == nil {
			log.Printf("Got message: %s", rm.Message)
			err = a.postMessage(&rm.Message)
			if (err == nil) {
				log.Printf("Message posted")
				rm.Accept()
			} else {
				log.Printf("Failed to post message: %s", err)
				rm.Reject()
				fatalIf(err)
			}
		} else {
			log.Printf("Failed to receive: %s", err)
			fatalIf(err)
		}
	}
	//amqpconn.Close(nil)  where does this go? ZZZ
	log.Printf("NOTREACHED reached")
	return nil
}

func (a *Adapter) postMessage(m *amqp.Message) error {
	logger := logging.FromContext(context.TODO())

	ctx := cloudevents.EventContext{
		CloudEventsVersion: cloudevents.CloudEventsVersion,
		EventType:          "amqp.message.delivery",
		EventID:            messageIdString(m),
		EventTime:          (*m).CreationTime(),
		Source:             "some_canon_amqpaddr_rep_TODO", // Expose no secrets
	}
	req, err := cloudevents.Binary.NewRequest(a.SinkURI, m, ctx)
	if err != nil {
		log.Printf("Failed to marshal the message: %+v : %s", m, err)
		return err
	}

	logger.Debug("posting to SinkURI", zap.Any("SinkURI", a.SinkURI))
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to do POST", zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	logger.Debug("response", zap.Any("status", resp.Status), zap.Any("body", string(body)))
	return nil
}

func (a *Adapter) dial(u *url.URL) (conn net.Conn, err error) {
	if u.Scheme == "amqp" {
		return net.Dial("tcp", u.Host)
	}
	var roots *x509.CertPool = nil
	if a.RootCA != "" {
		roots = x509.NewCertPool()    // override container's root CAs
		log.Println("ZZZ1")
		ok := roots.AppendCertsFromPEM([]byte(a.RootCA))
		if !ok {
			err = fmt.Errorf("adapter.dial: bad Root CA encoding") // Any other possible reason?
			return
		}
	}

	return tls.Dial("tcp", u.Host, &tls.Config{
		RootCAs: roots,
		InsecureSkipVerify: a.InsecureTlsConnection,
	})
}

func fatalIf(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func messageIdString(m *amqp.Message) string {
	if m == nil {
		return ""
	}
	msgid := (*m).MessageId()
	// AMQP specifies four legal Message ID data types, mapped to the following Go types by Proton.
	// CloudEvents requires the Message ID as string type only.
	switch msgid.(type) {
	case string:
		return msgid.(string)
	case uint64:
		return fmt.Sprintf("%d", msgid)
	case amqp.UUID:
		s := msgid.(amqp.UUID).String()
		// s formatted as "UUID(c4b04c04-8a8e-4a7d-948a-5e5843433b4d)" , strip enclosing "UUID()" notation
		return s[5:len(s)-1]
	case amqp.Binary:
		return base64.StdEncoding.EncodeToString([]byte(msgid.(amqp.Binary)))
	default:
		return ""
	}

}
