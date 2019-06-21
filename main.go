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
	"net"
	"net/http"
	"os"
	"os/signal"
	"io/ioutil"
	"github.com/Shopify/sarama"
	"google.golang.org/grpc"
	"golang.org/x/net/context"
	empty "github.com/golang/protobuf/ptypes/empty"
        importer "./proto"
)

var (
	importerTopic = "importer"

)

var DataProducer sarama.AsyncProducer

type device struct  {
	subscription []string
	freq uint32
}

type Server struct {
	devicemap  map[string]*device
	gRPCserver   *grpc.Server
	dataproducer sarama.AsyncProducer
	devicechan   chan *importer.DeviceInfo
}

func (s *Server) SendDeviceInfo(c context.Context,  info *importer.DeviceInfo) (*empty.Empty, error) {
	d := device {
		freq:	info.Frequency,
	}
	s.devicemap[info.IpAddress] = &d
	s.devicechan <- info
	return &empty.Empty{}, nil
}
func(s *Server) subscribeevents() {
	for {
		select {
			case info:= <-s.devicechan:
				ip_address:= info.IpAddress
			fmt.Println("Configuring  %s ...", ip_address)
			// call subscription function with info.IpAddress
		}
	}
}
func NewGrpcServer(grpcport string) (l net.Listener, g *grpc.Server, e error) {
        fmt.Println("Listening %s ...", grpcport)
        g = grpc.NewServer()
        l, e = net.Listen("tcp", grpcport)
        return
}
func (s *Server) startgrpcserver()error {
	fmt.Println("starting gRPC Server")
	grpcport := ":50051"
	listener, gserver, err := NewGrpcServer(grpcport)
	if err != nil {
		fmt.Println("Failed to create gRPC server: %v", err)
		return err
	}
	s.gRPCserver = gserver
	importer.RegisterDeviceManagementServer(gserver, s)
	if err := gserver.Serve(listener); err != nil {
		fmt.Println("Failed to run gRPC server: %v", err)
		return err
	}
	return nil

}
func (s *Server) kafkaInit() {
	fmt.Println("Starting kafka init to Connect to broker: ")
	config := sarama.NewConfig()
        config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 10
	config.Producer.Return.Successes = true
	producer, err := sarama.NewAsyncProducer([]string{"cord-kafka.default.svc.cluster.local:9092"}, config)
	if err != nil {
		panic(err)
	}
	s.dataproducer = producer
	defer func() {
		if err := producer.Close(); err != nil {
			panic(err)
		}
	}()
}

func (s *Server) handle_events(w http.ResponseWriter, r *http.Request) {
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
		case s.dataproducer.Input() <- message:

		case <-signals:
	        s.dataproducer.AsyncClose() // Trigger a shutdown of the producer.
		}
	}
}

func (s *Server) runServer() {
	fmt.Println("Starting HTTP Server")
	http.HandleFunc("/", s.handle_events)
	http.ListenAndServe(":8080", nil)
}

func init() {
	fmt.Println("Connecting to broker: ")
	fmt.Println("Listening to  http server")
}


func main() {
	fmt.Println("Starting Device-management Container")
	s := Server {
		devicemap:	make(map[string]*device),
		devicechan:	make(chan *importer.DeviceInfo),
	}
	go s.kafkaInit()
	go s.runServer()
	go s.startgrpcserver()
	go s.subscribeevents()
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	select {
	case sig := <-quit:
		fmt.Println("Shutting down:", sig)
	}
}
