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
	"io"
	"net/http"
	"os"
	"log"
	"strings"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"net"
	"encoding/base64"
	"encoding/json"

	"github.com/knative/pkg/cloudevents"

	// Imports the Qpid AMQP Go client
	"qpid.apache.org/amqp"
	"qpid.apache.org/electron"
)

type amqpBodyReader struct {
	body string
	offset int
}


type Adapter struct {
	// URI-eske connection and address info to attach to the AMQP endpoint
	// (confusingly also a "source") via AMQP or AMQPS protocol.
	// TODO(cliffjansen): json in env or file for full control and auth capabilities
	SourceURI string
	// SinkURI is the URI messages will be forwarded to as CloudEvents via HTTP(S).
	SinkURI string
	// Link credit to use on the path.
	Credit int
	// Optional connect-config configuration, including password/TLS secrets
	CredsPath string
	// The canonical name for the CloudEvents "source" Context Attribute.
	SpecSource string
	// The CA root(s) in pem format to authenticate the connection
	RootCA []byte
}

var msgCount = int64(0)


// Run creates a single AMQP connection/session/receiver to read messages, converts each
// message to a cloudevent and delivers it to the sink.
func (a *Adapter) Start() error {
	// logger := logging.FromContext(context.TODO())
	// TODO: set up signals so we handle the first shutdown signal gracefully

	// Use Kubernetes PODNAME-uuid as descriptive and unique AMQP container name:
	container := electron.NewContainer(fmt.Sprintf("%s", os.Getenv("HOSTNAME")))

	// Create connect url from address URI + connect-config data + AMQP defaults
	u, err := url.Parse(a.SourceURI)
	fatalIf(err)
	a.applyConfig(u)
	err = amqp.UpdateURL(u)
	fatalIf(err)

	a.SpecSource = fmt.Sprintf("%s://%s:%s/%s", u.Scheme, u.Hostname(), u.Port(), u.Path)
	log.Printf("Dial")
	tcpconn, err := a.dial(u)
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
			}
		} else {
			log.Printf("Failed to receive: %s", err)
			fatalIf(err)
		}
	}
	//amqpconn.Close(nil)  defer... or irrelevant?
	log.Printf("NOTREACHED reached")
	return nil
}

func (a *Adapter) postMessage(m *amqp.Message) error {
	logger := logging.FromContext(context.TODO())

	// TODO: check for existing CloudEvents headers to see if we are just forwarding an existing event.
	// The following code creates a new CloudEvents event from an arbitrary AMQP message.

	var rdr *amqpBodyReader
	ctype := (*m).ContentType()
	var body = (*m).Body()
	log.Printf("body switch time with : %T ... %v", body, body)
	switch body.(type) {
	case string:
		log.Printf("body switch string")
		ctype = "text/plain; charset=utf-8"
		rdr = newAmqpBodyReader(body.(string))
	case amqp.Binary:
		log.Printf("body switch bin")
		if ctype == "" {
			ctype = "application/octet-stream"
			rdr = newAmqpBodyReader(body.(amqp.Binary).String())
		}
	default:
		return fmt.Errorf("AMQP message format not supported")
	}

	ctx := cloudevents.EventContext{
		CloudEventsVersion: cloudevents.CloudEventsVersion,
		EventType:          "amqp.message.delivery",
		EventID:            messageIdString(m),
		EventTime:          (*m).CreationTime(),
		Source:             a.SpecSource,
		ContentType:        ctype,
	}
	var err error
	var req *http.Request
	if rdr != nil {
		req, err = cloudevents.Binary.NewRequest(a.SinkURI, rdr, ctx)
	} else {
		// Binary data section in json.
		// For now, lib just calls json.Marshall(), so pass as string, not reader
		req, err = cloudevents.Binary.NewRequest(a.SinkURI, body.(amqp.Binary).String(), ctx)
	}
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
	respbody, _ := ioutil.ReadAll(resp.Body)
	logger.Debug("response", zap.Any("status", resp.Status), zap.Any("body", string(respbody)))
	return nil
}

func (a *Adapter) dial(u *url.URL) (conn net.Conn, err error) {
	if u.Scheme == "amqp" {
		return net.Dial("tcp", u.Host)
	}
	var roots *x509.CertPool = nil
	if len(a.RootCA) > 0 {
		roots = x509.NewCertPool()    // override container's root CAs
		ok := roots.AppendCertsFromPEM(a.RootCA)
		if !ok {
			err = fmt.Errorf("adapter.dial: bad Root CA encoding") // Any other possible reason?
			return
		}
	}

	return tls.Dial("tcp", u.Host, &tls.Config{
		RootCAs: roots,
		InsecureSkipVerify: false,
	})
}

func fatalIf(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

type ConnectConfig struct {
	Scheme    string `json:"scheme"`
	Host      string `json:"host"`
	Port      string `json:"port"`
	User      string `json:"user"`
	Password  string `json:"password"`
	// TODO: SASL and TLS sub-structs
}

func parseConfigBytes(bytes []byte) *ConnectConfig {
	var config ConnectConfig
	err := json.Unmarshal(bytes, &config)
	fatalIf(err);
	return &config
}

func (a *Adapter) applyConfig(u *url.URL) {
	// values already in url take precedence over connect-config data
	if a.CredsPath == "" {
		return
	}
	cdir := a.CredsPath
	if !strings.HasSuffix(cdir, "/") {
		cdir += "/"
	}
	var b []byte
	var err error
	if b, err = ioutil.ReadFile(cdir + "connect-config"); err == nil {
		if config := parseConfigBytes(b); config != nil {
			if u.Scheme == "" {
				u.Scheme = config.Scheme
			}
			if u.Host == "" {
				u.Host = net.JoinHostPort(config.Host, config.Port)
			}
			if u.User == nil {
				u.User = url.UserPassword(config.User, config.Password)
			}
		}
	}
	if b, err = ioutil.ReadFile(cdir + "tls.ca"); err == nil {
		a.RootCA = b
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

func newAmqpBodyReader (b string) *amqpBodyReader {
	log.Printf("body reader NEW with : %v", b)
	return &amqpBodyReader{
		body:    b,
		offset:  0,
	}
}

func (rdr *amqpBodyReader) Read(out []byte) (n int, err error) {
	if rdr.offset < len(rdr.body) {
		bytes := rdr.body[rdr.offset:]
		n = copy(out, bytes)
		rdr.offset += n
	}
	if rdr.offset >= len(rdr.body) {
		err = io.EOF
	}
	log.Printf("body reader READ with : %d and %v", n, err)
	return
}
