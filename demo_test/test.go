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
        "bufio"
        "os"
        "os/signal"
        "os/exec"
        "github.com/Shopify/sarama"
        "google.golang.org/grpc"
        "golang.org/x/net/context"
        importer "./proto"
        log "github.com/Sirupsen/logrus"
	"bytes"
	"strings"
        "net/http"
        "crypto/tls"
        "strconv"
)

var REDFISH_ROOT		= "/redfish/v1"
var CONTENT_TYPE		= "application/json"
var EVENTS_MAP = map[string]string{
"add":"ResourceAdded",
"rm":"ResourceRemoved",
"alert":"Alert",
"update":"Update"}

var default_address string	= "localhost:31085"
var default_port    string	= "8888"
var default_vendor  string	= "edgecore"
var default_freq    uint64	= 180
var attach_device_ip string	= ""
var importerTopic		= "importer"
var DataConsumer sarama.Consumer

var cc	   importer.DeviceManagementClient
var ctx	   context.Context
var conn   * grpc.ClientConn

type Device struct {
	deviceinfo * importer.DeviceInfo
	eventlist  * importer.EventList
}

var devicemap map[string]* Device

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to register the device for data collection and frequency.
//
//
/*///////////////////////////////////////////////////////////////////////*/
func (s * Device) Attach(aip string, avendor string, afreq uint32) (error, string) {
        fmt.Println("Received Attach\n")
	var default_protocol string	= "https"

	s.deviceinfo = new(importer.DeviceInfo)
	s.eventlist  = new(importer.EventList)
	s.deviceinfo.IpAddress  = aip
	s.deviceinfo.Vendor     = avendor
	s.deviceinfo.Frequency  = afreq
	s.deviceinfo.Protocol   = default_protocol
	_, err := cc.SendDeviceInfo(ctx, s.deviceinfo)

	if err != nil {
		return err ,"attach error!!"
	}else{
		return nil,""
	}
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to change the frequency of data collection
//
//
/*///////////////////////////////////////////////////////////////////////*/
func (s * Device) UpdateFreq(wd uint32)(error, string) {
        fmt.Println("Received Period\n")
	s.deviceinfo.Frequency  = wd
	_, err := cc.SetFrequency(ctx, s.deviceinfo)

	if err != nil {
		return err, "period error!!"
	}else{
		return nil,""
	}
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to unsubscribe events
//
//
/*///////////////////////////////////////////////////////////////////////*/
func (s * Device) Subscribe(eventlist []string) (error, string) {
        fmt.Println("Received Subscribe\n")
	s.eventlist.Events = eventlist
	s.eventlist.EventIpAddress = s.deviceinfo.IpAddress
	_, err := cc.SubsrcribeGivenEvents(ctx, s.eventlist)

	if err != nil {
		return err, "sub error!!"
	}else{
		return nil,""
	}
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to unsubscribe events
//
//
/*///////////////////////////////////////////////////////////////////////*/
func (s * Device) UnSubscribe(eventlist []string) (error, string) {
        fmt.Println("Received UnSubscribe\n")
	s.eventlist.Events = eventlist
	s.eventlist.EventIpAddress = s.deviceinfo.IpAddress
	_, err := cc.UnSubsrcribeGivenEvents(ctx, s.eventlist)

	if err != nil {
		return err, "unsub error!!"
	}else{
		return nil,""
	}
}

/*///////////////////////////////////////////////////////////////////////*/
// Allows user to get the events supported by device
//
//
/*///////////////////////////////////////////////////////////////////////*/
func (s * Device) GetEventSupportList() (error, []string) {
        fmt.Println("Received GetEventSupportList\n")
	var ret_msg * importer.SupportedEventList
	ret_msg, err :=cc.GetEventList(ctx, devicemap[s.deviceinfo.IpAddress].deviceinfo);
	if err != nil {
		return err,ret_msg.Events
	}else{
		fmt.Println("show all event subs:", ret_msg)
		return nil , ret_msg.Events
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
	}else{

		conn, err = grpc.Dial(default_address, grpc.WithInsecure())
		if err != nil {
		        fmt.Println("could not connect")
			log.Fatal("did not connect: %v", err)
		}
		defer conn.Close()

		cc = importer.NewDeviceManagementClient(conn)
		ctx = context.Background()

		devicemap = make(map[string] *Device)
		loop := true

		for loop == true {
			cmd, _ := bufio.NewReader(connS).ReadString('\n')

			cmd = strings.TrimSuffix(cmd, "\n")
			s := strings.Split(cmd, ":")
			newmessage := "cmd error!!"
			cmd = s[0]

			switch string(cmd) {

			case "attach" :
				cmd_size := len(s)
				var err	error
				var aport   string = default_port
				var avendor string = default_vendor
				var uafreq  uint64 = default_freq

				if (cmd_size == 2 || cmd_size == 5){
					aip    := s[1]
					if(cmd_size == 5){
						aport	= s[2]
						avendor	= s[3]
						afreq	:= s[4]
						uafreq, err = strconv.ParseUint(afreq, 10, 64)

						if err != nil {
							fmt.Print("ParseUint error!!")
						}

						attach_device_ip = aip + ":" + aport
					}else{
						attach_device_ip = aip + ":" + default_port
					}

					if (devicemap[attach_device_ip] == nil){
						dev := new (Device)
						err, newmessage = dev.Attach(attach_device_ip, avendor, uint32(uafreq))
						if err != nil {
							fmt.Print("attach error!!")
						}else{
							fmt.Print("attatch IP:", attach_device_ip)
							newmessage = attach_device_ip
							devicemap[attach_device_ip] = dev
						}
					}else{
						fmt.Print("Change attach IP to %v", attach_device_ip)
						newmessage = attach_device_ip
					}
				}else{
					fmt.Print("Need IP address !!")
					newmessage = "Need IP address !!"

				}
			break

			case "period" :
				cmd_size := len(s)
				if (cmd_size == 2 ){
					if (devicemap[attach_device_ip] != nil){
						pv  := s[1]
						fmt.Print("pv:", pv)
						u, err := strconv.ParseUint(pv, 10, 64)

						if err != nil {
							fmt.Print("ParseUint error!!")
						}else{
							wd := uint32(u)
							dev := devicemap[attach_device_ip]
							err, newmessage =  dev.UpdateFreq(wd)

							if err != nil {
								fmt.Print("period error!!")
							}else{
								newmessage = strings.ToUpper(cmd)
							}
						}
					}else{
						fmt.Print("need attach first!!")
						newmessage = "need attach first!!"
					}
				}else{
					fmt.Print("Need period value !!")
					newmessage = "Need period value !!"
				}

			break

			case "sub","unsub" :
				cmd_size := len(s)
				fmt.Print("cmd is :", cmd)
				if(cmd_size > 4 || cmd_size <0){
					fmt.Print("error event !!")
					newmessage = "error event !!"
				}else{
					var events_list []string
					for i := 1; i < cmd_size; i++ {
						if value, ok := EVENTS_MAP[s[i]]; ok {
							events_list = append(events_list,value)
						} else {
							fmt.Println("key not found")
						}
					}

					if (devicemap[attach_device_ip] != nil){
						dev := devicemap[attach_device_ip]
						if(string(cmd) == "sub"){
							err, newmessage =  dev.Subscribe(events_list)
							if err != nil {
								fmt.Print("sub error!!")
								newmessage = "sub error!!"
							}else{
								newmessage = strings.ToUpper(cmd)
							}
						}else{
							err, newmessage =  dev.UnSubscribe(events_list)
							if err != nil {
								fmt.Print("unsub error!!")
								newmessage = "unsub error!!"
							}else{
								newmessage = strings.ToUpper(cmd)
							}
						}
					}else{
						fmt.Print("need attach first !!")
						newmessage = "need attach first !!"
					}
				}
			break

			case "showeventlist" :
				if (devicemap[attach_device_ip] != nil){
					dev := devicemap[attach_device_ip]
					err, supportlist :=  dev.GetEventSupportList()

					if err != nil {
						fmt.Print("showeventlist error!!")
					}else{
						fmt.Print("showeventlist ", supportlist)
						newmessage = strings.Join(supportlist[:],",")
					}
				}else{
					fmt.Print("need attach first !!")
					newmessage = "need attach first !!"
				}

			break

			case "QUIT" :
				loop = false
	                        newmessage="QUIT"
			break

			default :
			break
			}
			// send string back to client
			connS.Write([]byte(newmessage + "\n"))
	        }
	}
}
