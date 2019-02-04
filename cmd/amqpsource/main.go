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

package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/alanconway/lightning/pkg/amqp"
	lhttp "github.com/alanconway/lightning/pkg/http"
	"github.com/alanconway/lightning/pkg/lightning"
	"go.uber.org/zap"
)

func getRequiredEnv(envKey string) string {
	val, ok := os.LookupEnv(envKey)
	if !ok {
		log.Fatalf("required environment variable not defined '%s'", envKey)
	}
	return val
}

func getRequiredURL(envKey string, scheme string) *url.URL {
	v := getRequiredEnv(envKey)
	u, err := url.Parse(v)
	if err != nil {
		log.Fatalf("invaid URL for %v: %#v", envKey, v)
	}
	return u
}

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Unable to create logger: %v", err)
	}

	sourceURL := getRequiredURL("AMQP_URI", "amqp")
	amqpServer, _ := strconv.ParseBool(os.Getenv("AMQP_SERVER"))
	s := getRequiredEnv("AMQP_CREDIT")
	credit, err := strconv.Atoi(s)
	if err != nil || credit <= 0 {
		log.Fatalf("Bad AMQP credit value: %#v", s)
	}
	// FIXME aconway 2019-01-31: restore credentials functionality
	// credsPath, _ := os.LookupEnv("AMQP_CREDENTIALS")
	var source lightning.Source
	if amqpServer {
		source, err = amqp.NewServerSource("tcp", sourceURL.Host, credit, logger)
	} else {
		source, err = amqp.NewClientSource(sourceURL, credit, logger)
	}
	if err != nil {
		log.Fatalf("Can't start AMQP source: %v", err)
	}

	sinkURL := getRequiredURL("SINK_URI", "http")
	sink := lhttp.NewSink(sinkURL, http.DefaultClient, logger)

	m, err := source.Receive()
	for err == nil {
		if err = sink.Send(m); err == nil {
			m, err = source.Receive()
		}
	}
	if err != nil && err != io.EOF {
		log.Fatalf("Error forwarding cloud-event messages: %v", err)
	}
	return
}
