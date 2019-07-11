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
	"crypto/tls"
	empty "github.com/golang/protobuf/ptypes/empty"
        importer "./proto"
)

var (
	importerTopic = "importer"

)

var DataProducer sarama.AsyncProducer

var default_events = [...]string{"ResourceAdded","ResourceRemoved","Alert"}

type device struct  {
	subscriptions map[string]uint
	freq uint32
}

type Server struct {
	devicemap  map[string]*device
	gRPCserver   *grpc.Server
	dataproducer sarama.AsyncProducer
	devicechan   chan *importer.DeviceInfo
	freqchan   chan uint32
}


func (s *Server) GetEventList(c context.Context, info *importer.DeviceInfo) (*importer.EventList, error) {
        fmt.Println("Received GetEventList\n")
       eventstobesubscribed:= new(importer.EventList)
       eventstobesubscribed.EventIpAddress = info.IpAddress
       eventstobesubscribed.Events = append(eventstobesubscribed.Events,"ResourceAdded","ResourceRemoved","Alert")
       return eventstobesubscribed, nil
}

func (s *Server) SetFrequency(c context.Context,  info *importer.DeviceInfo) (*empty.Empty, error) {
        fmt.Println("Received SetFrequency\n")
       device := s.devicemap[info.IpAddress]
       device.freq = info.Frequency
       //Inform scheduler frquency has changed
       s.freqchan <- info.Frequency
       return &empty.Empty{}, nil
}

func (s *Server) SubsrcribeGivenEvents(c context.Context,  subeventlist *importer.EventList) (*empty.Empty, error) {
        fmt.Println("Received SubsrcribeEvents\n")
       //Call API to subscribe events
       ip_address := subeventlist.EventIpAddress
        for _, event := range subeventlist.Events {
                rtn, id := add_subscription(ip_address, event)
                if rtn {
                        s.devicemap[ip_address].subscriptions[event] = id
                        fmt.Println("subscription added", event, id)
                }
        }
       return &empty.Empty{}, nil
}

func (s *Server) UnSubsrcribeGivenEvents(c context.Context,  unsubeventlist *importer.EventList) (*empty.Empty, error) {
       fmt.Println("Received UnSubsrcribeEvents\n")
       ip_address := unsubeventlist.EventIpAddress
       //Call API to unsubscribe events
        for _, event := range unsubeventlist.Events {
                rtn := remove_subscription(ip_address, s.devicemap[ip_address].subscriptions[event] )
                if rtn {
                        delete(s.devicemap[ip_address].subscriptions, event)
                        fmt.Println("subscription removed", event)
                }
        }

       return &empty.Empty{}, nil
}


func (s *Server) SendDeviceInfo(c context.Context,  info *importer.DeviceInfo) (*empty.Empty, error) {
	d := device {
		subscriptions: make(map[string]uint),
		freq:	info.Frequency,
	}
	s.devicemap[info.IpAddress] = &d
	ip_address:= info.IpAddress
	fmt.Println("Configuring  %s ...", ip_address)
	// call subscription function with info.IpAddress
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	for _, event := range default_events {
		rtn, id := add_subscription(ip_address, event)
		if rtn {
			s.devicemap[ip_address].subscriptions[event] = id
			fmt.Println("subscription added", event, id)
		}
	}
	return &empty.Empty{}, nil
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
		freqchan:        make(chan uint32),
	}
	go s.kafkaInit()
	go s.runServer()
	go s.startgrpcserver()
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	select {
	case sig := <-quit:
		fmt.Println("Shutting down:", sig)
	}
}
