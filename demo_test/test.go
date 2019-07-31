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
        "os"
        "os/signal"
        "os/exec"
        "github.com/Shopify/sarama"
        "google.golang.org/grpc"
        "golang.org/x/net/context"
        importer "./proto"
        log "github.com/Sirupsen/logrus"
	"time"
	"bytes"
	"strings"

)

const (
	 address     = "localhost:31085"
	vendor       = "edgecore"
//	device_ip    = "192.168.3.44:9888"
	device_ip    = "192.168.4.27:8888"
	protocol     = "https"
	freq         = 180
)
var importerTopic = "importer"
var DataConsumer sarama.Consumer

func init() {
        Formatter := new(log.TextFormatter)
        Formatter.TimestampFormat = "02-01-2006 15:04:05"
        Formatter.FullTimestamp = true
        log.SetFormatter(Formatter)
}

func topicListener(topic *string, master sarama.Consumer) {
	log.Info("Starting topicListener for ", *topic)
	consumer, err := master.ConsumePartition(*topic, 0, sarama.OffsetOldest)
	if err != nil {
		log.Error("topicListener panic, topic=[%s]: %s", *topic, err.Error())
		os.Exit(1)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	doneCh := make(chan struct{})
	go func() {
		for {
			select {
			case err := <-consumer.Errors():
				log.Error("Consumer error: %s", err.Err)
			case msg := <-consumer.Messages():
				log.Info("Got message on topic=[%s]: %s", *topic, string(msg.Value))
			case <-signals:
				log.Warn("Interrupt is detected")
				doneCh <- struct{}{}
			}
		}
	}()
	<-doneCh
}

func kafkainit() {
	cmd := exec.Command("/bin/sh","kafka_ip.sh")
	var kafkaIP string
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Info(err)
		os.Exit(1)
	}

	kafkaIP = out.String()
	kafkaIP = strings.TrimSuffix(kafkaIP, "\n")
	kafkaIP = kafkaIP +":9092"
	fmt.Println("IP address of kafka-cord-0:",kafkaIP)
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	master, err := sarama.NewConsumer([]string{kafkaIP}, config)
	if err != nil {
		panic(err)
	}
	DataConsumer = master

	go topicListener(&importerTopic, master)
}
func main() {
	log.Info("kafkaInit starting")
	kafkainit()
	// Set up a connection to the server.
	fmt.Println("Starting connection")
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
	        fmt.Println("could not connect")
		log.Fatal("did not connect: %v", err)
	}
	defer conn.Close()
	c := importer.NewDeviceManagementClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	deviceinfo := new(importer.DeviceInfo)
	deviceinfo.IpAddress  = device_ip
	deviceinfo.Vendor     = vendor
	deviceinfo.Frequency   = freq
	deviceinfo.Protocol    = protocol
	_, err = c.SendDeviceInfo(ctx, deviceinfo)
	if err != nil {
		log.Fatal("could not SendDeviceInfo: %v", err)
	}
        quit := make(chan os.Signal)
        signal.Notify(quit, os.Interrupt)

        select {
        case sig := <-quit:
                fmt.Println("Shutting down:", sig)
                DataConsumer.Close()
                        panic(err)
        }

}
