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
	"time"
	"log"
	"strings"

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
	url, err := amqp.ParseURL(a.SourceURI)
	fatalIf(err)
	log.Printf("Dial")
	c, err := container.Dial("tcp", url.Host) // NOTE: Dial takes just the Host part of the URL
	fatalIf(err)
	addr := strings.TrimPrefix(url.Path, "/")
	opts := []electron.LinkOption{electron.Source(addr)}
	arbitrary_prefetch := 10 // TODO: something sane/configurable
	opts = append(opts, electron.Capacity(arbitrary_prefetch), electron.Prefetch(true))
	log.Printf("Create receiver")
	r, err := c.Receiver(opts...)
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
	//c.Close(nil)  where does this go? ZZZ
	log.Printf("NOTREACHED reached")
	return nil
}

func (a *Adapter) postMessage(m *amqp.Message) error {
	logger := logging.FromContext(context.TODO())

	ctx := cloudevents.EventContext{
		CloudEventsVersion: cloudevents.CloudEventsVersion,
		EventType:          "amqp.delivery",
		EventID:            fmt.Sprintf("%v", msgCount), //ZZZ
		EventTime:          time.Now(),  // TODO: revisit
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

func fatalIf(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
