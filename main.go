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
        log "github.com/Sirupsen/logrus"
	"time"
)

//globals
const REDFISH_ROOT = "/redfish/v1"
const CONTENT_TYPE = "application/json"

var (
	importerTopic = "importer"

)

var DataProducer sarama.AsyncProducer


var    vendor_default_events = map[string][]string{
        "edgecore": {"ResourceAdded","ResourceRemoved","Alert"},
    }
var redfish_services = [...]string{"/Chassis", "/Systems","/EthernetSwitches"}

type scheduler struct  {
	getdata time.Ticker
	quit chan bool
}

type device struct  {
	subscriptions map[string]uint
	datacollector scheduler
	freqchan   chan uint32
        vendor string
}

type Server struct {
	devicemap  map[string]*device
	gRPCserver   *grpc.Server
	dataproducer sarama.AsyncProducer
	devicechan   chan *importer.DeviceInfo
}

func (s *Server) GetEventList(c context.Context, info *importer.DeviceInfo) (*importer.SupportedEventList, error) {
	fmt.Println("Received GetEventList\n")
	eventstobesubscribed:= new(importer.SupportedEventList)
	eventstobesubscribed.Events = vendor_default_events[info.Vendor]
	return eventstobesubscribed, nil
}

func (s *Server) SetFrequency(c context.Context,  info *importer.DeviceInfo) (*empty.Empty, error) {
	fmt.Println("Received SetFrequency")
	s.devicemap[info.IpAddress].freqchan <- info.Frequency
	return &empty.Empty{}, nil
}

func (s *Server) SubsrcribeGivenEvents(c context.Context,  subeventlist *importer.EventList) (*empty.Empty, error) {
	fmt.Println("Received SubsrcribeEvents\n")
	//Call API to subscribe events
	ip_address := subeventlist.EventIpAddress
	for _, event := range subeventlist.Events {
                if _, ok := s.devicemap[ip_address].subscriptions[event]; !ok {
                 rtn, id := add_subscription(ip_address, event)
                        if rtn {
                                 s.devicemap[ip_address].subscriptions[event] = id
                                fmt.Println("subscription added", event, id)
                        }
                } else {
                                log.WithFields(log.Fields{
                                  "Event": event,
                                }).Info("Already Subscribed")
                }
	}
	return &empty.Empty{}, nil
}

func (s *Server) UnSubsrcribeGivenEvents(c context.Context,  unsubeventlist *importer.EventList) (*empty.Empty, error) {
       fmt.Println("Received UnSubsrcribeEvents\n")
       ip_address := unsubeventlist.EventIpAddress
       //Call API to unsubscribe events
        for _, event := range unsubeventlist.Events {
                if _, ok := s.devicemap[ip_address].subscriptions[event]; ok {
                 rtn := remove_subscription(ip_address, s.devicemap[ip_address].subscriptions[event] )
                        if rtn {
                                delete(s.devicemap[ip_address].subscriptions, event)
                                fmt.Println("subscription removed", event)
                        }
                } else {
                                log.WithFields(log.Fields{
                                  "Event": event,
                                }).Info("was not Subscribed")
                        }
        }

       return &empty.Empty{}, nil
}

func (s *Server) collect_data(ip_address string) {
	freqchan := s.devicemap[ip_address].freqchan
	ticker := s.devicemap[ip_address].datacollector.getdata
	donechan := s.devicemap[ip_address].datacollector.quit
	for {
		select {
		case freq := <-freqchan:
			ticker.Stop()
			ticker = *time.NewTicker(time.Duration(freq) * time.Second)
		case <-ticker.C:
			for _, service := range redfish_services {
				rtn, data := get_status(ip_address, service)
				if rtn {
					for _, str := range data {
						str = "Device IP: " + ip_address + " " + str
						b := []byte(str)
						s.dataproducer.Input() <- &sarama.ProducerMessage{Topic: importerTopic, Value: sarama.StringEncoder(b)}
					}
				}
			}
		case <-donechan:
			ticker.Stop()
			fmt.Println("getdata ticker stopped")
			return
		}
	}
}

func (s *Server) SendDeviceInfo(c context.Context,  info *importer.DeviceInfo) (*empty.Empty, error) {
	d := device {
		subscriptions: make(map[string]uint),
		datacollector: scheduler{
			getdata: *time.NewTicker(time.Duration(info.Frequency) * time.Second),
			quit: make(chan bool),
		},
		freqchan:        make(chan uint32),
                vendor: info.Vendor,
	}
        //default_events := [...]string{}
	s.devicemap[info.IpAddress] = &d
	ip_address:= info.IpAddress
	fmt.Println("Configuring  %s ...", ip_address)
	// call subscription function with info.IpAddress
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	default_events := vendor_default_events[info.Vendor]
	for _, event := range default_events {
		rtn, id := add_subscription(ip_address, event)
		if rtn {
			s.devicemap[ip_address].subscriptions[event] = id
			fmt.Println("subscription added", event, id)
		}
	}
	go s.collect_data(ip_address)
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
        Formatter := new(log.TextFormatter)
        Formatter.TimestampFormat = "02-01-2006 15:04:05"
        Formatter.FullTimestamp = true
        log.SetFormatter(Formatter)
	fmt.Println("Connecting to broker: ")
	fmt.Println("Listening to  http server")
        log.Info("log Connecting to broker:")
        log.Info("log Listening to  http server ")
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
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	select {
	case sig := <-quit:
		fmt.Println("Shutting down:", sig)
	}
}
