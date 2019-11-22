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
	"github.com/opencord/device-management/proto"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/Shopify/sarama"
	log "github.com/sirupsen/logrus"
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
const RF_DEFAULT_PROTOCOL = "https://"
const RF_DATA_COLLECT_THRESHOLD = 5
const RF_DATA_COLLECT_DUMMY_INTERVAL = 1000
const CONTENT_TYPE = "application/json"

var (
	importerTopic = "importer"
)

var DataProducer sarama.AsyncProducer

var redfish_resources = [...]string{"/redfish/v1/Chassis", "/redfish/v1/Systems","/redfish/v1/EthernetSwitches"}
var pvmount = os.Getenv("DEVICE_MANAGEMENT_PVMOUNT")
var subscriptionListPath string

type scheduler struct  {
	getdata *time.Ticker
	quit chan bool
	getdataend chan bool
}

type device struct  {
	Subscriptions	map[string]string	`json:"ss"`
	Freq		uint32			`json:"freq"`
	Datacollector	scheduler		`json:"-"`
	Freqchan	chan uint32		`json:"-"`
	Eventtypes	[]string		`json:"eventtypes"`
	Datafile	*os.File		`json:"-"`
}

type Server struct {
	devicemap	map[string]*device
	gRPCserver	*grpc.Server
	dataproducer	sarama.AsyncProducer
	httpclient	*http.Client
}

func (s *Server) ClearCurrentEventList(c context.Context, info *importer.Device) (*empty.Empty, error) {
	fmt.Println("Received ClearCurrentEventList\n")
	ip_address := info.IpAddress
	_, found := s.devicemap[ip_address]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Device not registered")
	}
	for event, _ := range s.devicemap[ip_address].Subscriptions {
		rtn := s.remove_subscription(ip_address, event)
		if !rtn {
			log.WithFields(log.Fields{
				"Event": event,
			}).Info("Error removing event")
		}
	}
	s.update_data_file(ip_address)
	return &empty.Empty{}, nil
}

func (s *Server) GetCurrentEventList(c context.Context, info *importer.Device) (*importer.EventList, error) {
	fmt.Println("Received GetCurrentEventList\n")
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

func (s *Server) GetEventList(c context.Context, info *importer.Device) (*importer.EventList, error) {
	fmt.Println("Received GetEventList\n")
	eventstobesubscribed := new(importer.EventList)
//	eventstobesubscribed.Events = s.devicemap[info.IpAddress].Eventtypes
	eventstobesubscribed.Events = s.get_event_types(info.IpAddress)
	if eventstobesubscribed.Events == nil {
		return nil, status.Errorf(codes.NotFound, "No events found")
	}
	return eventstobesubscribed, nil
}

func (s *Server) SetFrequency(c context.Context, info *importer.FreqInfo) (*empty.Empty, error) {
	fmt.Println("Received SetFrequency")
	_, found := s.devicemap[info.IpAddress]
	if !found {
		return nil, status.Errorf(codes.NotFound, "Device not registered")
	}
	if info.Frequency > 0 && info.Frequency < RF_DATA_COLLECT_THRESHOLD {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid frequency")
	}
	s.devicemap[info.IpAddress].Freqchan <- info.Frequency
	s.devicemap[info.IpAddress].Freq = info.Frequency
	s.update_data_file(info.IpAddress)
	return &empty.Empty{}, nil
}

func (s *Server) SubsrcribeGivenEvents(c context.Context, subeventlist *importer.GivenEventList) (*empty.Empty, error) {
	errstring := ""
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
	for _, event := range subeventlist.Events {
		if _, ok := s.devicemap[ip_address].Subscriptions[event]; !ok {
			rtn := s.add_subscription(ip_address, event)
			if !rtn {
				errstring = errstring + "failed to subscribe event " + ip_address + " " + event + "\n"
				log.WithFields(log.Fields{
					"Event": event,
				}).Info("Error adding  event")
			}
		} else {
			errstring = errstring + "event " + event + " already subscribed\n"
			log.WithFields(log.Fields{
				"Event": event,
			}).Info("Already Subscribed")
		}
	}
	s.update_data_file(ip_address)
	if errstring != "" {
		return &empty.Empty{}, status.Errorf(codes.InvalidArgument, errstring)
	}
	return &empty.Empty{}, nil
}

func (s *Server) UnSubsrcribeGivenEvents(c context.Context, unsubeventlist *importer.GivenEventList) (*empty.Empty, error) {
	errstring := ""
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
	for _, event := range unsubeventlist.Events {
		if _, ok := s.devicemap[ip_address].Subscriptions[event]; ok {
			rtn := s.remove_subscription(ip_address, event)
			if !rtn {
				errstring = errstring + "failed to unsubscribe event " + ip_address + " " + event + "\n"
				log.WithFields(log.Fields{
					"Event": event,
				}).Info("Error removing event")
			}
		} else {
			errstring = errstring + "event " + event + " not found\n"
			log.WithFields(log.Fields{
				"Event": event,
			}).Info("was not Subscribed")
		}
	}
	s.update_data_file(ip_address)

	if errstring != "" {
		return &empty.Empty{}, status.Errorf(codes.InvalidArgument, errstring)
	}
	return &empty.Empty{}, nil
}

func (s *Server) update_data_file(ip_address string) {
	f := s.devicemap[ip_address].Datafile
	if f != nil {
		b, err := json.Marshal(s.devicemap[ip_address])
		if err != nil {
			fmt.Println(err)
		} else {
			f.Truncate(0)
			f.Seek(0, 0)
			n, err := f.Write(b)
			if err != nil {
				fmt.Println("err wrote", n, "bytes")
				fmt.Println(err)
			}
		}
	} else {
		fmt.Println("file handle is nil", ip_address)
	}
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
				s.devicemap[ip_address].Datacollector.getdata = ticker
			}
		case err := <-s.dataproducer.Errors():
			fmt.Println("Failed to produce message:", err)
		case <-ticker.C:
			for _, resource := range redfish_resources {
				data := s.get_status(ip_address, resource)
				for _, str := range data {
					str = "Device IP: " + ip_address + " " + str
					fmt.Printf("collected data  %s\n ...", str)
					b := []byte(str)
					msg := &sarama.ProducerMessage{Topic: importerTopic, Value: sarama.StringEncoder(b)}
					select {
					case  s.dataproducer.Input() <- msg:
						fmt.Println("Produce message")
					default:
					}
				}
			}
		case <-donechan:
			ticker.Stop()
			fmt.Println("getdata ticker stopped")
			s.devicemap[ip_address].Datacollector.getdataend <- true
			return
		}
	}
}

func (s *Server) DeleteDeviceList(c context.Context, list *importer.DeviceListByIp) (*empty.Empty, error) {
	fmt.Println("DeleteDeviceList received")
	errstring := ""
	for _, ip := range list.Ip {
		if _, ok := s.devicemap[ip]; !ok {
			fmt.Printf("Device not found ", ip)
			errstring = errstring + "Device " + ip + " not found\n"
			continue
		}
		for event, _ := range s.devicemap[ip].Subscriptions {
			rtn := s.remove_subscription(ip, event)
			if !rtn {
				log.WithFields(log.Fields{
					"Event": event,
				}).Info("Error removing event")
			}
		}
		fmt.Println("deleting device", ip)
		s.devicemap[ip].Datacollector.quit <- true

		f := s.devicemap[ip].Datafile
		if f != nil {
			fmt.Println("deleteing file", f.Name())
			err := f.Close()
			if err != nil {
				fmt.Println("error closing file ", f.Name(), err)
				errstring = errstring + "error closing file " + f.Name() + "\n"
			}
			err = os.Remove(f.Name())
			if err != nil {
				fmt.Println("error deleting file ", f.Name(), err)
			}
		} else {
			errstring = errstring + "file " + ip + " not found\n"
		}
		<-s.devicemap[ip].Datacollector.getdataend
		delete(s.devicemap, ip)
	}
	if errstring != "" {
		return &empty.Empty{}, status.Errorf(codes.InvalidArgument, errstring)
	}
	return &empty.Empty{}, nil
}

func (s *Server) SendDeviceList(c context.Context,  list *importer.DeviceList) (*empty.Empty, error) {
	errstring := ""
	for _, dev := range list.Device {
		ip_address:= dev.IpAddress
		if _, ok := s.devicemap[dev.IpAddress]; ok {
			fmt.Printf("Device %s already exists", ip_address)
			errstring = errstring + "Device " + ip_address + " already exists\n"
			continue
		}

		if dev.Frequency > 0 && dev.Frequency < RF_DATA_COLLECT_THRESHOLD {
			fmt.Printf("Device %s data collection frequency %d out of range", ip_address, dev.Frequency)
			errstring = errstring + "Device " + ip_address + " data collection frequency out of range\n"
			continue
		}
		d := device {
			Subscriptions: make(map[string]string),
			Freq: dev.Frequency,
			Datacollector: scheduler{
				quit: make(chan bool),
				getdataend: make(chan bool),
			},
			Freqchan: make(chan uint32),
		}
		s.devicemap[ip_address] = &d
		fmt.Printf("Configuring  %s\n", ip_address)

		/* if initial interval is 0, create a dummy ticker, which is stopped right away, so getdata is not nil */
		freq := dev.Frequency
		if freq == 0 {
			freq = RF_DATA_COLLECT_DUMMY_INTERVAL
		}
		s.devicemap[ip_address].Datacollector.getdata = time.NewTicker(time.Duration(freq) * time.Second)
		if dev.Frequency == 0 {
			s.devicemap[ip_address].Datacollector.getdata.Stop()
		}

		eventtypes := s.get_event_types(ip_address)
		if eventtypes != nil {
			for _, event := range eventtypes {
				s.devicemap[ip_address].Eventtypes = append(s.devicemap[ip_address].Eventtypes, event)
				if s.add_subscription(ip_address, event) == false {
					errstring = errstring + "failed to subscribe event " + ip_address + " " + event + "\n"
				}
			}
		}
		go s.collect_data(ip_address)
		s.devicemap[ip_address].Datafile = get_data_file(ip_address)
		s.update_data_file(ip_address)
	}
	if errstring != "" {
		return &empty.Empty{}, status.Errorf(codes.InvalidArgument, errstring)
	}
	return &empty.Empty{}, nil
}

func (s *Server) GetCurrentDevices(c context.Context, e *importer.Empty) (*importer.DeviceListByIp, error) {
	fmt.Println("In Received GetCurrentDevices\n")

	if len(s.devicemap) == 0 {
		return nil, status.Errorf(codes.NotFound, "Devices not registered")
	}
	dl := new(importer.DeviceListByIp)
	for k, v := range s.devicemap {
		if v != nil {
			fmt.Printf("IpAdd[%s] \n", k)
			dl.Ip = append(dl.Ip, k)
		}
	}
	return dl, nil
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
	fmt.Println("Retrieving persisted data")
	subscriptionListPath = pvmount + "/subscriptions"
	if err := os.MkdirAll(subscriptionListPath, 0777); err != nil {
		fmt.Println(err)
	} else {
		files, err := ioutil.ReadDir(subscriptionListPath)
		if err != nil {
			fmt.Println(err)
		} else {
			for _, f := range files {
				b, err := ioutil.ReadFile(path.Join(subscriptionListPath, f.Name()))
				if err != nil {
					fmt.Println(err)
				} else if f.Size() > 0 {
					ip := f.Name()
					d := device{}
					json.Unmarshal(b, &d)
					s.devicemap[ip] = &d
					freq := s.devicemap[ip].Freq

					/* if initial interval is 0, create a dummy ticker, which is stopped right away, so getdata is not nil */
					if freq == 0 {
						freq = RF_DATA_COLLECT_DUMMY_INTERVAL
					}
					s.devicemap[ip].Datacollector.getdata = time.NewTicker(time.Duration(freq) * time.Second)
					if s.devicemap[ip].Freq == 0 {
						s.devicemap[ip].Datacollector.getdata.Stop()
					}

					s.devicemap[ip].Datacollector.quit = make(chan bool)
					s.devicemap[ip].Datacollector.getdataend = make(chan bool)
					s.devicemap[ip].Freqchan = make(chan uint32)
					s.devicemap[ip].Datafile = get_data_file(ip)
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

func get_data_file(ip string) *os.File {
	if pvmount == "" {
		return nil
	}
	f, err := os.OpenFile(subscriptionListPath+"/"+ip, os.O_CREATE|os.O_RDWR, 0664)
	if err != nil {
		fmt.Println(err)
	}
	return f
}

func (s *Server) close_data_files() {
	for ip, _ := range s.devicemap {
		s.devicemap[ip].Datafile.Close()
	}
}

func main() {
	fmt.Println("Starting Device-management Container")

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	s := Server {
		devicemap:	make(map[string]*device),
		httpclient:	client,
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
		s.close_data_files()
	}
}
