// Copyright 2018-present Open Networking Foundation
// Copyright 2018-present Edgecore Networks Corporation
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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/Shopify/sarama"
	empty "github.com/golang/protobuf/ptypes/empty"
	"github.com/opencord/device-management/proto"
	logrus "github.com/sirupsen/logrus"
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
	"strconv"
	"strings"
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

var redfish_resources = [...]string{"/redfish/v1/Chassis", "/redfish/v1/Systems", "/redfish/v1/EthernetSwitches"}
var pvmount = os.Getenv("DEVICE_MANAGEMENT_PVMOUNT")
var subscriptionListPath string

type scheduler struct {
	getdata    *time.Ticker
	quit       chan bool
	getdataend chan bool
}

type device struct {
	Subscriptions map[string]string `json:"ss"`
	Freq          uint32            `json:"freq"`
	Datacollector scheduler         `json:"-"`
	Freqchan      chan uint32       `json:"-"`
	Eventtypes    []string          `json:"eventtypes"`
	Datafile      *os.File          `json:"-"`
}

type Server struct {
	devicemap    map[string]*device
	gRPCserver   *grpc.Server
	dataproducer sarama.AsyncProducer
	httpclient   *http.Client
}

func (s *Server) ClearCurrentEventList(c context.Context, info *importer.Device) (*empty.Empty, error) {
	logrus.Info("Received ClearCurrentEventList")
	ip_address := info.IpAddress
	if msg, ok := s.validate_ip(ip_address, true, true); !ok {
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	for event := range s.devicemap[ip_address].Subscriptions {
		rtn := s.remove_subscription(ip_address, event)
		if !rtn {
			logrus.WithFields(logrus.Fields{
				"Event": event,
			}).Info("Error removing event")
		}
	}
	s.update_data_file(ip_address)
	return &empty.Empty{}, nil
}

func (s *Server) GetCurrentEventList(c context.Context, info *importer.Device) (*importer.EventList, error) {
	logrus.Info("Received GetCurrentEventList")
	if msg, ok := s.validate_ip(info.IpAddress, true, true); !ok {
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	currentevents := new(importer.EventList)
	for event := range s.devicemap[info.IpAddress].Subscriptions {
		currentevents.Events = append(currentevents.Events, event)
	}
	return currentevents, nil
}

func (s *Server) GetEventList(c context.Context, info *importer.Device) (*importer.EventList, error) {
	logrus.Info("Received GetEventList")
	if msg, ok := s.validate_ip(info.IpAddress, true, true); !ok {
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	eventstobesubscribed := new(importer.EventList)
	eventstobesubscribed.Events = s.devicemap[info.IpAddress].Eventtypes
	if eventstobesubscribed.Events == nil {
		return nil, status.Errorf(codes.NotFound, "No events found\n")
	}
	return eventstobesubscribed, nil
}

func (s *Server) SetFrequency(c context.Context, info *importer.FreqInfo) (*empty.Empty, error) {
	logrus.Info("Received SetFrequency")
	if msg, ok := s.validate_ip(info.IpAddress, true, true); !ok {
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	if info.Frequency > 0 && info.Frequency < RF_DATA_COLLECT_THRESHOLD {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid interval\n")
	}
	s.devicemap[info.IpAddress].Freqchan <- info.Frequency
	s.devicemap[info.IpAddress].Freq = info.Frequency
	s.update_data_file(info.IpAddress)
	return &empty.Empty{}, nil
}

func (s *Server) SubscribeGivenEvents(c context.Context, subeventlist *importer.GivenEventList) (*empty.Empty, error) {
	errstring := ""
	logrus.Info("Received SubsrcribeEvents")
	//Call API to subscribe events
	ip_address := subeventlist.EventIpAddress
	if msg, ok := s.validate_ip(ip_address, true, true); !ok {
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	if len(subeventlist.Events) <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Event list is empty\n")
	}
	for _, event := range subeventlist.Events {
		if _, ok := s.devicemap[ip_address].Subscriptions[event]; !ok {
			supported := false
			for _, e := range s.devicemap[ip_address].Eventtypes {
				if e == event {
					supported = true
					rtn := s.add_subscription(ip_address, event)
					if !rtn {
						errstring = errstring + "failed to subscribe event " + ip_address + " " + event + "\n"
						logrus.WithFields(logrus.Fields{
							"Event": event,
						}).Info("Error adding  event")
					}
					break
				}
			}
			if !supported {
				errstring = errstring + "event " + event + " not supported\n"
				logrus.WithFields(logrus.Fields{
					"Event": event,
				}).Info("not supported")
			}
		} else {
			errstring = errstring + "event " + event + " already subscribed\n"
			logrus.WithFields(logrus.Fields{
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

func (s *Server) UnsubscribeGivenEvents(c context.Context, unsubeventlist *importer.GivenEventList) (*empty.Empty, error) {
	errstring := ""
	logrus.Info("Received UnSubsrcribeEvents")
	ip_address := unsubeventlist.EventIpAddress
	if msg, ok := s.validate_ip(ip_address, true, true); !ok {
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}

	if len(unsubeventlist.Events) <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Event list is empty\n")
	}
	//Call API to unsubscribe events
	for _, event := range unsubeventlist.Events {
		if _, ok := s.devicemap[ip_address].Subscriptions[event]; ok {
			rtn := s.remove_subscription(ip_address, event)
			if !rtn {
				errstring = errstring + "failed to unsubscribe event " + ip_address + " " + event + "\n"
				logrus.WithFields(logrus.Fields{
					"Event": event,
				}).Info("Error removing event")
			}
		} else {
			errstring = errstring + "event " + event + " not found\n"
			logrus.WithFields(logrus.Fields{
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
			logrus.Errorf("Update_data_file %s", err)
		} else {
			err := f.Truncate(0)
			if err != nil {
				logrus.Errorf("err Trunate %s", err)
				return
			}
			pos, err := f.Seek(0, 0)
			if err != nil {
				logrus.Errorf("err Seek %s", err)
				return
			}
			fmt.Println("moved back to", pos)
			n, err := f.Write(b)
			if err != nil {
				logrus.Errorf("err wrote %d bytes", n)
				logrus.Errorf("write error to file %s", err)
			}
		}
	} else {
		logrus.Errorf("file handle is nil %s", ip_address)
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
			logrus.Errorf("Failed to produce message:%s", err)
		case <-ticker.C:
			for _, resource := range redfish_resources {
				data := s.get_status(ip_address, resource)
				for _, str := range data {
					str = "Device IP: " + ip_address + " " + str
					logrus.Infof("collected data  %s", str)
					b := []byte(str)
					msg := &sarama.ProducerMessage{Topic: importerTopic, Value: sarama.StringEncoder(b)}
					s.dataproducer.Input() <- msg
					logrus.Info("Produce message")
				}
			}
		case <-donechan:
			ticker.Stop()
			logrus.Info("getdata ticker stopped")
			s.devicemap[ip_address].Datacollector.getdataend <- true
			return
		}
	}
}

func (s *Server) DeleteDeviceList(c context.Context, list *importer.DeviceListByIp) (*empty.Empty, error) {
	logrus.Info("DeleteDeviceList received")
	errstring := ""
	for _, ip := range list.Ip {
		if _, ok := s.devicemap[ip]; !ok {
			logrus.Infof("Device not found %s ", ip)
			errstring = errstring + "Device " + ip + " not found\n"
			continue
		}
		for event := range s.devicemap[ip].Subscriptions {
			rtn := s.remove_subscription(ip, event)
			if !rtn {
				logrus.WithFields(logrus.Fields{
					"Event": event,
				}).Info("Error removing event")
			}
		}
		logrus.Infof("deleting device %s", ip)
		s.devicemap[ip].Datacollector.quit <- true

		f := s.devicemap[ip].Datafile
		if f != nil {
			logrus.Infof("deleteing file %s", f.Name())
			err := f.Close()
			if err != nil {
				logrus.Errorf("error closing file %s %s", f.Name(), err)
				errstring = errstring + "error closing file " + f.Name() + "\n"
			}
			err = os.Remove(f.Name())
			if err != nil {
				logrus.Errorf("error deleting file %s Error:%s ", f.Name(), err)
			}
		} else {
			logrus.Errorf("File not found %s", errstring+"file "+ip+" not found")
		}
		<-s.devicemap[ip].Datacollector.getdataend
		delete(s.devicemap, ip)
	}
	if errstring != "" {
		return &empty.Empty{}, status.Errorf(codes.InvalidArgument, errstring)
	}
	return &empty.Empty{}, nil
}

func (s *Server) SendDeviceList(c context.Context, list *importer.DeviceList) (*empty.Empty, error) {
	errstring := ""
	for _, dev := range list.Device {
		ip_address := dev.IpAddress
		if msg, ok := s.validate_ip(ip_address, false, false); !ok {
			errstring = errstring + msg
			continue
		}
		if dev.Frequency > 0 && dev.Frequency < RF_DATA_COLLECT_THRESHOLD {
			logrus.Errorf("Device %s data collection interval %d out of range", ip_address, dev.Frequency)
			errstring = errstring + "Device " + ip_address + " data collection frequency out of range\n"
			continue
		}
		d := device{
			Subscriptions: make(map[string]string),
			Freq:          dev.Frequency,
			Datacollector: scheduler{
				quit:       make(chan bool),
				getdataend: make(chan bool),
			},
			Freqchan: make(chan uint32),
		}
		s.devicemap[ip_address] = &d
		logrus.Infof("Configuring  %s", ip_address)

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
		for _, event := range eventtypes {
			s.devicemap[ip_address].Eventtypes = append(s.devicemap[ip_address].Eventtypes, event)
			if !s.add_subscription(ip_address, event) {
				errstring = errstring + "failed to subscribe event " + ip_address + " " + event + "\n"
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
	logrus.Infof("In Received GetCurrentDevices")

	if len(s.devicemap) == 0 {
		return nil, status.Errorf(codes.NotFound, "No Device found\n")
	}
	dl := new(importer.DeviceListByIp)
	for k, v := range s.devicemap {
		if v != nil {
			logrus.Infof("IpAdd[%s] \n", k)
			dl.Ip = append(dl.Ip, k)
		}
	}
	return dl, nil
}

func NewGrpcServer(grpcport string) (l net.Listener, g *grpc.Server, e error) {
	logrus.Infof("Listening %s\n", grpcport)
	g = grpc.NewServer()
	l, e = net.Listen("tcp", grpcport)
	return
}
func (s *Server) startgrpcserver() {
	logrus.Info("starting gRPC Server")
	listener, gserver, err := NewGrpcServer(GlobalConfig.LocalGrpc)
	if err != nil {
		logrus.Errorf("Failed to create gRPC server: %s ", err)
		panic(err)
	}
	s.gRPCserver = gserver
	importer.RegisterDeviceManagementServer(gserver, s)
	if err := gserver.Serve(listener); err != nil {
		logrus.Errorf("Failed to run gRPC server: %s ", err)
		panic(err)
	}

}
func (s *Server) kafkaCloseProducer() {
	if err := s.dataproducer.Close(); err != nil {
		panic(err)
	}

}
func (s *Server) kafkaInit() {
	logrus.Info("Starting kafka init to Connect to broker: ")
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 10
	producer, err := sarama.NewAsyncProducer([]string{GlobalConfig.Kafka}, config)
	if err != nil {
		panic(err)
	}
	s.dataproducer = producer
}

func (s *Server) handle_events(w http.ResponseWriter, r *http.Request) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	logrus.Info(" IN Handle Event  ")
	if r.Method == "POST" {
		Body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logrus.Errorf("Error getting HTTP data %s", err)
		}
		defer r.Body.Close()
		logrus.Info("Received Event Message ")
		fmt.Printf("%s\n", Body)
		message := &sarama.ProducerMessage{
			Topic: importerTopic,
			Value: sarama.StringEncoder(Body),
		}
		s.dataproducer.Input() <- message
	}
}

func (s *Server) runServer() {
	logrus.Info("Starting HTTP Server")
	http.HandleFunc("/", s.handle_events)
	err := http.ListenAndServeTLS(GlobalConfig.Local, "https-server.crt", "https-server.key", nil)
	if err != nil {
		panic(err)
	}
}

/* validate_ip() verifies if the ip and port are valid and already registered then return the truth value of the desired state specified by the following 2 switches,
   want_registered: 'true' if the fact of an ip is registered is the desired state
   include_port: 'true' further checks if <ip>:<port#> does exist in the devicemap in case an ip is found registered
*/
func (s *Server) validate_ip(ip_address string, want_registered bool, include_port bool) (msg string, ok bool) {
	msg = ""
	ok = false
	if !strings.Contains(ip_address, ":") {
		logrus.Errorf("Incorrect IP address %s, expected format <ip>:<port #>", ip_address)
		msg = "Incorrect IP address format (<ip>:<port #>)\n"
		return
	}
	splits := strings.Split(ip_address, ":")
	ip, port := splits[0], splits[1]
	if net.ParseIP(ip) == nil {
		// also check to see if it's a valid hostname
		if _, err := net.LookupIP(ip); err != nil {
			logrus.Errorf("Invalid IP address %s", ip)
			msg = "Invalid IP address " + ip + "\n"
			return
		}
	}
	if _, err := strconv.Atoi(port); err != nil {
		logrus.Errorf("Port # %s is not an integer", port)
		msg = "Port # " + port + " needs to be an integer\n"
		return
	}
	for k := range s.devicemap {
		if strings.HasPrefix(k, ip) {
			if !want_registered {
				logrus.Errorf("Device ip %s already registered", ip)
				msg = "Device ip " + ip + " already registered\n"
				return
			} else if include_port {
				if _, found := s.devicemap[ip_address]; found {
					ok = true
					return
				} else {
					logrus.Errorf("Device %s not registered", ip_address)
					msg = "Device " + ip_address + " not registered\n"
					return
				}
			} else {
				ok = true
				return
			}
		}
	}
	if want_registered {
		logrus.Errorf("Device %s not registered", ip_address)
		msg = "Device " + ip_address + " not registered\n"
		return
	}
	ok = true
	return
}

func (s *Server) init_data_persistence() {
	logrus.Info("Retrieving persisted data")
	subscriptionListPath = pvmount + "/subscriptions"
	if err := os.MkdirAll(subscriptionListPath, 0777); err != nil {
		logrus.Errorf("MkdirAll %s", err)
	} else {
		files, err := ioutil.ReadDir(subscriptionListPath)
		if err != nil {
			logrus.Errorf("ReadDir %s", err)
		} else {
			for _, f := range files {
				b, err := ioutil.ReadFile(path.Join(subscriptionListPath, f.Name()))
				if err != nil {
					logrus.Errorf("Readfile %s", err)
				} else if f.Size() > 0 {
					ip := f.Name()
					d := device{}
					err := json.Unmarshal(b, &d)
					if err != nil {
						logrus.Errorf("Unmarshal %s", err)
						return
					}
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
	Formatter := new(logrus.TextFormatter)
	Formatter.TimestampFormat = "02-01-2006 15:04:05"
	Formatter.FullTimestamp = true
	logrus.SetFormatter(Formatter)
	logrus.Info("log Connecting to broker:")
	logrus.Info("log Listening to  http server ")
	//sarama.Logger = log.New()
}

func get_data_file(ip string) *os.File {
	logrus.Info("get_data_file")
	if pvmount == "" {
		return nil
	}
	f, err := os.OpenFile(subscriptionListPath+"/"+ip, os.O_CREATE|os.O_RDWR, 0664)
	if err != nil {
		logrus.Errorf("Openfile err %s", err)
	}
	return f
}

func (s *Server) close_data_files() {
	for ip := range s.devicemap {
		s.devicemap[ip].Datafile.Close()
	}
}

func main() {
	logrus.Info("Starting Device-management Container")

	ParseCommandLine()
	ProcessGlobalOptions()
	ShowGlobalOptions()

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	s := Server{
		devicemap:  make(map[string]*device),
		httpclient: client,
	}

	s.kafkaInit()
	go s.runServer()
	go s.startgrpcserver()

	if pvmount != "" {
		s.init_data_persistence()
	}

	quit := make(chan os.Signal, 10)
	signal.Notify(quit, os.Interrupt)

	sig := <-quit
	logrus.Infof("Shutting down:%d", sig)
	s.kafkaCloseProducer()
	s.close_data_files()
}
