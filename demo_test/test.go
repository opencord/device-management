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
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/Shopify/sarama"
	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
)

var REDFISH_ROOT = "/redfish/v1"
var CONTENT_TYPE = "application/json"
var EVENTS_MAP = map[string]string{
	"add":    "ResourceAdded",
	"rm":     "ResourceRemoved",
	"alert":  "Alert",
	"update": "Update"}

var default_address string = "localhost:31085"
var default_port string = "8888"
var default_vendor string = "edgecore"
var default_freq uint64 = 180
var attach_device_ip string = ""
var importerTopic = "importer"
var DataConsumer sarama.Consumer

var cc importer.DeviceManagementClient
var ctx context.Context
var conn *grpc.ClientConn

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to register the device for data collection and frequency.
//
//
/*///////////////////////////////////////////////////////////////////////*/
func Attach(aip string, avendor string, afreq uint32) error {
	fmt.Println("Received Attach\n")
	var default_protocol string = "https"
	deviceinfo := new(importer.DeviceInfo)
	deviceinfo.IpAddress = aip
	deviceinfo.Vendor = avendor
	deviceinfo.Frequency = afreq
	deviceinfo.Protocol = default_protocol
	_, err := cc.SendDeviceInfo(ctx, deviceinfo)

	return err
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to change the frequency of data collection
//
//
/*///////////////////////////////////////////////////////////////////////*/
func UpdateFreq(ip_address string, wd uint32) error {
	fmt.Println("Received Period\n")
	freqinfo := new(importer.FreqInfo)
	freqinfo.Frequency = wd
	freqinfo.IpAddress = ip_address
	_, err := cc.SetFrequency(ctx, freqinfo)

	return err
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to unsubscribe events
//
//
/*///////////////////////////////////////////////////////////////////////*/
func Subscribe(ip_address string, Giveneventlist []string) error {
	fmt.Println("Received Subscribe\n")
	giveneventlist := new(importer.GivenEventList)
	giveneventlist.Events = Giveneventlist
	giveneventlist.EventIpAddress = ip_address
	_, err := cc.SubsrcribeGivenEvents(ctx, giveneventlist)

	return err
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to unsubscribe events
//
//
/*///////////////////////////////////////////////////////////////////////*/
func UnSubscribe(ip_address string, Giveneventlist []string) error {
	fmt.Println("Received UnSubscribe\n")
	giveneventlist := new(importer.GivenEventList)
	giveneventlist.Events = Giveneventlist
	giveneventlist.EventIpAddress = ip_address
	_, err := cc.UnSubsrcribeGivenEvents(ctx, giveneventlist)

	return err
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to get the events supported by device
//
//
/*///////////////////////////////////////////////////////////////////////*/
func GetEventSupportList(vendor string) (error, []string) {
	fmt.Println("Received GetEventSupportList\n")
	vendorinfo := new(importer.VendorInfo)
	vendorinfo.Vendor = vendor
	var ret_msg *importer.EventList
	ret_msg, err := cc.GetEventList(ctx, vendorinfo)
	if err != nil {
		return err, nil
	} else {
		return err, ret_msg.Events
	}
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to get the current events subscribed by device
//
//
/*///////////////////////////////////////////////////////////////////////*/
func GetEventCurrentDeviceList(ip_address string) (error, []string) {
	fmt.Println("Received GetEventCurrentDeviceList\n")
	currentdeviceinfo := new(importer.Device)
	currentdeviceinfo.IpAddress = ip_address
	var ret_msg *importer.EventList
	ret_msg, err := cc.GetCurrentEventList(ctx, currentdeviceinfo)
	if err != nil {
		return err, nil
	} else {
		return err, ret_msg.Events
	}
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to get the current events subscribed by device
//
//
/*///////////////////////////////////////////////////////////////////////*/
func ClearCurrentDeviceEventList(ip_address string) error {
	fmt.Println("Received ClearCurrentDeviceEventList\n")
	currentdeviceinfo := new(importer.Device)
	currentdeviceinfo.IpAddress = ip_address
	_, err := cc.ClearCurrentEventList(ctx, currentdeviceinfo)

	return err
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to get the current devices that are monitored
//
//
/*///////////////////////////////////////////////////////////////////////*/
func GetCurrentDevices() (error, []string) {
	fmt.Println("Testing GetCurrentDevices\n")
	empty := new(importer.Empty)
	var ret_msg *importer.DeviceList
	ret_msg, err := cc.GetCurrentDevices(ctx, empty)
	if err != nil {
		return err, nil
	} else {
		return err, ret_msg.Ip
	}
}

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
				os.Exit(1)
			}
		}
	}()
	<-doneCh
}

func kafkainit() {
	cmd := exec.Command("/bin/sh", "kafka_ip.sh")
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
	kafkaIP = kafkaIP + ":9092"
	fmt.Println("IP address of kafka-cord-0:", kafkaIP)
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
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	fmt.Println("Launching server...")
	log.Info("kafkaInit starting")
	kafkainit()

	ln, err := net.Listen("tcp", ":9999")
	if err != nil {
		fmt.Println("could not listen")
		log.Fatal("did not listen: %v", err)
	}
	defer ln.Close()

	connS, err := ln.Accept()
	if err != nil {
		fmt.Println("Accept error")
		log.Fatal("Accept error: %v", err)
	} else {

		conn, err = grpc.Dial(default_address, grpc.WithInsecure())
		if err != nil {
			fmt.Println("could not connect")
			log.Fatal("did not connect: %v", err)
		}
		defer conn.Close()

		cc = importer.NewDeviceManagementClient(conn)
		ctx = context.Background()

		loop := true

		for loop == true {
			cmd, _ := bufio.NewReader(connS).ReadString('\n')

			cmd = strings.TrimSuffix(cmd, "\n")
			s := strings.Split(cmd, ":")
			newmessage := "cmd error!!"
			cmd = s[0]

			switch string(cmd) {

			case "attach":
				cmd_size := len(s)
				var err error
				var uafreq uint64
				if cmd_size == 5 {
					aip := s[1]
					aport := s[2]
					avendor := s[3]
					afreq := s[4]
					uafreq, err = strconv.ParseUint(afreq, 10, 64)

					if err != nil {
						fmt.Print("ParseUint error!!\n")
					}

					attach_device_ip = aip + ":" + aport

					err = Attach(attach_device_ip, avendor, uint32(uafreq))
					if err != nil {
						errStatus, _ := status.FromError(err)
						fmt.Println(errStatus.Message())
						fmt.Println(errStatus.Code())
						fmt.Print("attach error!!\n")
						newmessage = errStatus.Message()

					} else {
						fmt.Print("attatch IP:\n", attach_device_ip)
						newmessage = attach_device_ip
					}
				} else {
					fmt.Print("Need IP addres,port,vendor,freqs !!\n")
					newmessage = "Need IP address !!"

				}

			case "period":
				cmd_size := len(s)
				fmt.Print("cmd_size period %d", cmd_size)
				if cmd_size == 4 {
					fip := s[1]
					fport := s[2]
					pv := s[3]
					fmt.Print("pv:", pv)
					u, err := strconv.ParseUint(pv, 10, 64)

					if err != nil {
						fmt.Print("ParseUint error!!\n")
					} else {
						wd := uint32(u)
						ip_address := fip + ":" + fport
						err = UpdateFreq(ip_address, wd)

						if err != nil {
							errStatus, _ := status.FromError(err)
							fmt.Println(errStatus.Message())
							fmt.Println(errStatus.Code())
							newmessage = errStatus.Message()
							fmt.Print("period error!!\n")
						} else {
							newmessage = strings.ToUpper(cmd)
						}
					}
				} else {
					fmt.Print("Need period value !!\n")
					newmessage = "Need period value !!"
				}

			case "sub", "unsub":
				cmd_size := len(s)
				fmt.Print("cmd is :", cmd, cmd_size)
				if cmd_size > 6 || cmd_size < 0 {
					fmt.Print("error event !!")
					newmessage = "error event !!"
				} else {
					ip := s[1]
					port := s[2]
					ip_address := ip + ":" + port
					var events_list []string
					for i := 3; i < cmd_size; i++ {
						if value, ok := EVENTS_MAP[s[i]]; ok {
							events_list = append(events_list, value)
						} else {
							fmt.Println("key not found")
						}
					}

					if string(cmd) == "sub" {
						err = Subscribe(ip_address, events_list)
						if err != nil {
							errStatus, _ := status.FromError(err)
							fmt.Println(errStatus.Message())
							fmt.Println(errStatus.Code())
							newmessage = errStatus.Message()
							fmt.Print("sub error!!")
						} else {
							newmessage = strings.ToUpper(cmd)
						}
					} else {
						err = UnSubscribe(ip_address, events_list)
						if err != nil {
							errStatus, _ := status.FromError(err)
							fmt.Println(errStatus.Message())
							fmt.Println(errStatus.Code())
							newmessage = errStatus.Message()
							fmt.Print("unsub error!!")
						} else {
							newmessage = strings.ToUpper(cmd)
						}
					}
				}

			case "showeventlist":
				cmd_size := len(s)
				fmt.Print("cmd is :", cmd, cmd_size)
				if cmd_size > 3 || cmd_size < 0 {
					fmt.Print("error event !!")
					newmessage = "error event !!"
				} else {
					vendor := s[1]
					err, supportlist := GetEventSupportList(vendor)

					if err != nil {
						errStatus, _ := status.FromError(err)
						fmt.Println(errStatus.Message())
						fmt.Println(errStatus.Code())
						newmessage = errStatus.Message()
						fmt.Print("showeventlist error!!")
					} else {
						fmt.Print("showeventlist ", supportlist)
						newmessage = strings.Join(supportlist[:], ",")
					}
				}

			case "showdeviceeventlist":
				cmd_size := len(s)
				fmt.Print("cmd is :", cmd, cmd_size)
				if cmd_size > 4 || cmd_size < 0 {
					fmt.Print("error event !!")
					newmessage = "error event !!"
				} else {
					eip := s[1]
					eport := s[2]
					ip_address := eip + ":" + eport
					err, currentlist := GetEventCurrentDeviceList(ip_address)

					if err != nil {
						errStatus, _ := status.FromError(err)
						fmt.Println(errStatus.Message())
						fmt.Println(errStatus.Code())
						newmessage = errStatus.Message()
						fmt.Print("showdeviceeventlist error!!")
					} else {
						fmt.Print("showeventlist ", currentlist)
						newmessage = strings.Join(currentlist[:], ",")
					}
				}

			case "cleardeviceeventlist":
				cmd_size := len(s)
				fmt.Print("cmd is :", cmd, cmd_size)
				if cmd_size > 4 || cmd_size < 0 {
					fmt.Print("error event !!")
					newmessage = "error event !!"
				} else {
					clip := s[1]
					clport := s[2]
					ip_address := clip + ":" + clport
					err = ClearCurrentDeviceEventList(ip_address)
					if err != nil {
						errStatus, _ := status.FromError(err)
						fmt.Println(errStatus.Message())
						fmt.Println(errStatus.Code())
						newmessage = errStatus.Message()
						fmt.Print("cleardeviceeventlist  error!!")
					} else {
						newmessage = strings.ToUpper(cmd)
					}
				}

			case "showdevices":
				cmd_size := len(s)
				fmt.Print("cmd is :", cmd, cmd_size)
				if cmd_size > 2 || cmd_size < 0 {
					fmt.Print("error event !!")
					newmessage = "error event !!"
				} else {
					err, currentlist := GetCurrentDevices()

					if err != nil {
						errStatus, _ := status.FromError(err)
						fmt.Println(errStatus.Message())
						fmt.Println(errStatus.Code())
						newmessage = errStatus.Message()
						fmt.Print("showdevices error!!")
					} else {
						fmt.Print("showdevices ", currentlist)
						newmessage = strings.Join(currentlist[:], ", ")
					}
				}
			case "QUIT":
				loop = false
				newmessage = "QUIT"

			default:
			}
			// send string back to client
			connS.Write([]byte(newmessage + "\n"))
		}
	}
}
