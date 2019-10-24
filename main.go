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
	importer "./proto"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/Shopify/sarama"
	log "github.com/Sirupsen/logrus"
	empty "github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"time"
)

//globals
const REDFISH_ROOT = "/redfish/v1"
const CONTENT_TYPE = "application/json"

var (
	importerTopic = "importer"
)

var DataProducer sarama.AsyncProducer

var vendor_default_events = map[string][]string{
	"edgecore": {"ResourceAdded", "ResourceRemoved", "Alert"},
}
var redfish_services = [...]string{"/Chassis", "/Systems", "/EthernetSwitches"}
var pvmount = os.Getenv("DEVICE_MANAGEMENT_PVMOUNT")
var subscriptionListPath string

type scheduler struct {
	getdata *time.Ticker
	quit    chan bool
}

type device struct {
	Subscriptions map[string]string `json:"ss"`
	Freq          uint32            `json:"freq"`
	Datacollector scheduler         `json:"-"`
	Freqchan      chan uint32       `json:"-"`
	Vendor        string            `json:"vendor"`
	Protocol      string            `json:"protocol"`
}

type Server struct {
	devicemap    map[string]*device
	gRPCserver   *grpc.Server
	dataproducer sarama.AsyncProducer
	httpclient   *http.Client
	devicechan   chan *importer.DeviceInfo
}

func (s *Server) ClearCurrentEventList(c context.Context, info *importer.Device) (*empty.Empty, error) {
	fmt.Println("Received GetCurrentEventList\n")
	ip_address := info.IpAddress
	_, found := s.devicemap[ip_address]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Device not registered")
	}
	f := get_subscription_list(ip_address)
	for event, _ := range s.devicemap[ip_address].Subscriptions {
		rtn := s.remove_subscription(ip_address, event, f)
		if !rtn {
			log.WithFields(log.Fields{
				"Event": event,
			}).Info("Error removing event")
		}
	}
	if f != nil {
		f.Close()
	}
	return &empty.Empty{}, nil
}

func (s *Server) GetCurrentEventList(c context.Context, info *importer.Device) (*importer.EventList, error) {
	fmt.Println("Received ClearCurrentEventList\n")
	_, found := s.devicemap[info.IpAddress]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Device not registered")
	}
	currentevents := new(importer.EventList)
	for event, _ := range s.devicemap[info.IpAddress].Subscriptions {
		currentevents.Events = append(currentevents.Events, event)
	}
	return currentevents, nil
}

func (s *Server) GetEventList(c context.Context, info *importer.VendorInfo) (*importer.EventList, error) {
	fmt.Println("Received GetEventList\n")
	_, found := vendor_default_events[info.Vendor]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Invalid Vendor Provided")
	}
	eventstobesubscribed := new(importer.EventList)
	eventstobesubscribed.Events = vendor_default_events[info.Vendor]
	return eventstobesubscribed, nil
}

func (s *Server) SetFrequency(c context.Context, info *importer.FreqInfo) (*empty.Empty, error) {
	fmt.Println("Received SetFrequency")
	_, found := s.devicemap[info.IpAddress]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Device not registered")
	}

	s.devicemap[info.IpAddress].Freqchan <- info.Frequency
	return &empty.Empty{}, nil
}

func (s *Server) SubsrcribeGivenEvents(c context.Context, subeventlist *importer.GivenEventList) (*empty.Empty, error) {
	fmt.Println("Received SubsrcribeEvents\n")
	//Call API to subscribe events
	ip_address := subeventlist.EventIpAddress
	_, found := s.devicemap[ip_address]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Device not registered")
	}
	if len(subeventlist.Events) <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Event list is empty")
	}
	f := get_subscription_list(ip_address)
	for _, event := range subeventlist.Events {
		if _, ok := s.devicemap[ip_address].Subscriptions[event]; !ok {
			rtn := s.add_subscription(ip_address, event, f)
			if !rtn {
				log.WithFields(log.Fields{
					"Event": event,
				}).Info("Error adding  event")
			}
		} else {
			log.WithFields(log.Fields{
				"Event": event,
			}).Info("Already Subscribed")
		}
	}
	if f != nil {
		f.Close()
	}
	return &empty.Empty{}, nil
}

func (s *Server) UnSubsrcribeGivenEvents(c context.Context, unsubeventlist *importer.GivenEventList) (*empty.Empty, error) {
	fmt.Println("Received UnSubsrcribeEvents\n")
	ip_address := unsubeventlist.EventIpAddress
	_, found := s.devicemap[ip_address]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Device not registered")
	}

	if len(unsubeventlist.Events) <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Event list is empty")
	}
	//Call API to unsubscribe events
	f := get_subscription_list(ip_address)
	for _, event := range unsubeventlist.Events {
		if _, ok := s.devicemap[ip_address].Subscriptions[event]; ok {
			rtn := s.remove_subscription(ip_address, event, f)
			if !rtn {
				log.WithFields(log.Fields{
					"Event": event,
				}).Info("Error removing event")
			}
		} else {
			log.WithFields(log.Fields{
				"Event": event,
			}).Info("was not Subscribed")
		}
	}
	if f != nil {
		f.Close()
	}

	return &empty.Empty{}, nil
}

func (s *Server) collect_data(ip_address string) {
	freqchan := s.devicemap[ip_address].Freqchan
	ticker := s.devicemap[ip_address].Datacollector.getdata
	donechan := s.devicemap[ip_address].Datacollector.quit
	for {
		select {
		case freq := <-freqchan:
			ticker.Stop()
			if freq > 0 {
				ticker = time.NewTicker(time.Duration(freq) * time.Second)
			}
		case err := <-s.dataproducer.Errors():
			fmt.Println("Failed to produce message:", err)
		case <-ticker.C:
			for _, service := range redfish_services {
				rtn, data := s.get_status(ip_address, service)
				if rtn {
					for _, str := range data {
						str = "Device IP: " + ip_address + " " + str
						fmt.Printf("collected data  %s\n ...", str)
						b := []byte(str)
						msg := &sarama.ProducerMessage{Topic: importerTopic, Value: sarama.StringEncoder(b)}
						select {
						case s.dataproducer.Input() <- msg:
							fmt.Println("Produce message")
						default:
						}
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

func (s *Server) SendDeviceInfo(c context.Context, info *importer.DeviceInfo) (*empty.Empty, error) {
	d := device{
		Subscriptions: make(map[string]string),
		Freq:          info.Frequency,
		Datacollector: scheduler{
			getdata: time.NewTicker(time.Duration(info.Frequency) * time.Second),
			quit:    make(chan bool),
		},
		Freqchan: make(chan uint32),
		Vendor:   info.Vendor,
		Protocol: info.Protocol,
	}
	_, found := s.devicemap[info.IpAddress]
	if found {
		return nil, status.Errorf(codes.AlreadyExists, "Device Already registered")
	}

	_, vendorfound := vendor_default_events[info.Vendor]
	if !vendorfound {
		return nil, status.Errorf(codes.NotFound, "Vendor Not Found")
	}

	//default_events := [...]string{}
	s.devicemap[info.IpAddress] = &d
	fmt.Printf("size of devicemap %d\n", len(s.devicemap))
	ip_address := info.IpAddress
	fmt.Printf("Configuring  %s\n", ip_address)
	// call subscription function with info.IpAddress

	default_events := vendor_default_events[info.Vendor]

	f := get_subscription_list(ip_address)
	for _, event := range default_events {
		s.add_subscription(ip_address, event, f)
	}
	if f != nil {
		f.Close()
	}
	go s.collect_data(ip_address)
	return &empty.Empty{}, nil
}

func NewGrpcServer(grpcport string) (l net.Listener, g *grpc.Server, e error) {
	fmt.Printf("Listening %s\n", grpcport)
	g = grpc.NewServer()
	l, e = net.Listen("tcp", grpcport)
	return
}
func (s *Server) startgrpcserver() error {
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
func (s *Server) kafkaCloseProducer() {
	if err := s.dataproducer.Close(); err != nil {
		panic(err)
	}

}
func (s *Server) kafkaInit() {
	fmt.Println("Starting kafka init to Connect to broker: ")
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 10
	producer, err := sarama.NewAsyncProducer([]string{"cord-kafka.default.svc.cluster.local:9092"}, config)
	if err != nil {
		panic(err)
	}
	s.dataproducer = producer
}

func (s *Server) handle_events(w http.ResponseWriter, r *http.Request) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	fmt.Println(" IN Handle Event  ")
	if r.Method == "POST" {
		Body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Println("Error getting HTTP data", err)
		}
		defer r.Body.Close()
		fmt.Println("Received Event Message ")
		fmt.Printf("%s\n", Body)
		message := &sarama.ProducerMessage{
			Topic: importerTopic,
			Value: sarama.StringEncoder(Body),
		}
		s.dataproducer.Input() <- message
	}
}

func (s *Server) runServer() {
	fmt.Println("Starting HTTP Server")
	http.HandleFunc("/", s.handle_events)
	http.ListenAndServeTLS(":8080", "https-server.crt", "https-server.key", nil)
}

func (s *Server) init_data_persistence() {
	subscriptionListPath = pvmount + "/subscriptions"
	if err := os.MkdirAll(subscriptionListPath, 0777); err != nil {
		fmt.Println(err)
	} else {
		lists, err := ioutil.ReadDir(subscriptionListPath)
		if err != nil {
			fmt.Println(err)
		} else {
			for _, list := range lists {
				b, err := ioutil.ReadFile(path.Join(subscriptionListPath, list.Name()))
				if err != nil {
					fmt.Println(err)
				} else {
					ip := list.Name()
					d := device{}
					json.Unmarshal(b, &d)
					s.devicemap[ip] = &d
					s.devicemap[ip].Datacollector.getdata = time.NewTicker(time.Duration(s.devicemap[ip].Freq) * time.Second)
					s.devicemap[ip].Datacollector.quit = make(chan bool)
					s.devicemap[ip].Freqchan = make(chan uint32)
					go s.collect_data(ip)
				}
			}
		}
	}
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
	//sarama.Logger = log.New()
}

func get_subscription_list(ip string) *os.File {
	if pvmount == "" {
		return nil
	}
	f, err := os.OpenFile(subscriptionListPath+"/"+ip, os.O_CREATE|os.O_RDWR, 0664)
	if err != nil {
		fmt.Println(err)
	}
	return f
}

func main() {
	fmt.Println("Starting Device-management Container")

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	s := Server{
		devicemap:  make(map[string]*device),
		devicechan: make(chan *importer.DeviceInfo),
		httpclient: client,
	}

	s.kafkaInit()
	go s.runServer()
	go s.startgrpcserver()

	if pvmount != "" {
		s.init_data_persistence()
	}

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	select {
	case sig := <-quit:
		fmt.Println("Shutting down:", sig)
		s.kafkaCloseProducer()
	}
}
