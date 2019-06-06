// Copyright 2018 Open Networking Foundation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"io/ioutil"
	"github.com/Shopify/sarama"
)

var (
//	broker  = [2]string{"voltha-kafka.default.svc.cluster.local","9092"}
	importerTopic = "importer"

)

var DataProducer sarama.AsyncProducer

func kafkaInit() {
	config := sarama.NewConfig()
        config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 10
	config.Producer.Return.Successes = true
	producer, err := sarama.NewAsyncProducer([]string{"cord-kafka.default.svc.cluster.local:9092"}, config)
	if err != nil {
		panic(err)
	}
	DataProducer = producer
	defer func() {
		if err := producer.Close(); err != nil {
			panic(err)
		}
	}()
}

func handle_events(w http.ResponseWriter, r *http.Request) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	if(r.Method ==  "POST"){
		Body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Println("Error getting HTTP data",err)
		}
		defer  r.Body.Close()
		fmt.Printf("%s\n",Body)
		message :=&sarama.ProducerMessage{
                        Topic: importerTopic,
                        Value: sarama.StringEncoder(Body),
                }
		select {
		case DataProducer.Input() <- message:

		case <-signals:
	        DataProducer.AsyncClose() // Trigger a shutdown of the producer.
		}
	}
}

func runServer() {
	fmt.Println("Starting HTTP Server")
	http.HandleFunc("/", handle_events)
	http.ListenAndServe(":8080", nil)
}

func init() {
	fmt.Println("Connecting to broker: ")
	fmt.Println("Listening to  http server")
}


func main() {
	fmt.Println("Starting Device-management Container")
	go kafkaInit()
	go runServer()
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	select {
	case sig := <-quit:
		fmt.Println("Shutting down:", sig)
	}
}
