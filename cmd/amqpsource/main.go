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
	"strconv"
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
	credit    int
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("unable to create logger: %v", err)
	}

	sink = getRequiredEnv("SINK_URI")
	source = getRequiredEnv("AMQP_URI")
	credit, _ = strconv.Atoi(getRequiredEnv("AMQP_CREDIT"))
	if credit <= 0 {
		log.Fatalf("bad AMQP credit value: %v", credit)
	}

	credsPath, _ := os.LookupEnv("AMQP_CREDENTIALS")

	a := amqpsource.Adapter{
		SourceURI: source,
		SinkURI:   sink,
		Credit:    credit,
		CredsPath: credsPath,
	}

	logger.Info("Starting AMQP Adapter. %v", zap.Reflect("adapter", a))

	err = a.Start()
	if err != nil {
		logger.Fatal("Failed to start the adapter", zap.Error(err))
	}

	logger.Info("exiting...")
}
