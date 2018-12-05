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
	"log"
	"os"
	"github.com/knative/eventing-sources/pkg/adapter/amqpsource"
	"go.uber.org/zap"
	"flag"
)

func getRequiredEnv(envKey string) string {
	val, defined := os.LookupEnv(envKey)
	if !defined {
		log.Fatalf("required environment variable not defined '%s'", envKey)
	}
	return val
}


var (
	sink      string
	source    string
	credit    uint
	insecure  bool
	rootca    string
)

func init() {
	flag.StringVar(&sink, "sink", "", "the host url to receive the AMQP event")
	flag.StringVar(&source, "amqpurl", "", "the AMQP source. e.g. amqp://host:port/queue_name")
	flag.UintVar(&credit, "credit", 10, "the credit window for the message batching")
	flag.BoolVar(&insecure, "insecureTls", false, "for insecure testing purposes only - disable TLS cerificate checking")
	flag.StringVar(&rootca, "rootCA", "", "The root CA certificate (in PEM format) needed to verify the AMQP host if using TLS")
}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("unable to create logger: %v", err)
	}

	flag.Parse()
	if (len(sink) > 0) {
		// Called via ContainerSource controller with --sink=foo --amqpurl=amqp://host:port/queue
	} else {
		// Called via custom controller, args in the environ
		source = getRequiredEnv("AMQP_URI")
		sink = getRequiredEnv("SINK")
	}

	a := amqpsource.Adapter{
		SourceURI: source,
		SinkURI:   sink,
		Credit:    credit,
		InsecureTlsConnection: insecure,
		RootCA:    rootca,
	}

	logger.Info("Starting AMQP Adapter. %v", zap.Reflect("adapter", a))

	err = a.Start()
	if err != nil {
		logger.Fatal("Failed to start the adapter", zap.Error(err))
	}

	logger.Info("exiting...")
}
