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
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/opencord/device-management/demo_test/proto"
	logrus "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
)

var EVENTS_MAP = map[string]string{
	"add":    "ResourceAdded",
	"rm":     "ResourceRemoved",
	"alert":  "Alert",
	"update": "Update"}

var importerTopic = "importer"
var DataConsumer sarama.Consumer

var cc importer.DeviceManagementClient
var ctx context.Context
var conn *grpc.ClientConn

func GetCurrentDevices() (error, []string) {
	logrus.Info("Testing GetCurrentDevices")
	empty := new(importer.Empty)
	var ret_msg *importer.DeviceListByIp
	ret_msg, err := cc.GetCurrentDevices(ctx, empty)
	if err != nil {
		return err, nil
	} else {
		return err, ret_msg.Ip
	}
}

func init() {
	Formatter := new(logrus.TextFormatter)
	Formatter.TimestampFormat = "02-01-2006 15:04:05"
	Formatter.FullTimestamp = true
	logrus.SetFormatter(Formatter)
}

func topicListener(topic *string, master sarama.Consumer) {
	logrus.Info("Starting topicListener for ", *topic)
	consumer, err := master.ConsumePartition(*topic, 0, sarama.OffsetOldest)
	if err != nil {
		logrus.Errorf("topicListener panic, topic=[%s]: %s", *topic, err.Error())
		os.Exit(1)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	doneCh := make(chan struct{})
	go func() {
		for {
			select {
			case err := <-consumer.Errors():
				logrus.Errorf("Consumer error: %s", err.Err)
			case msg := <-consumer.Messages():
				logrus.Infof("Got message on topic=[%s]: %s", *topic, string(msg.Value))
			case <-signals:
				logrus.Warn("Interrupt is detected")
				os.Exit(1)
			}
		}
	}()
	<-doneCh
}

func kafkainit() {
	var kafkaIP string
	if GlobalConfig.Kafka == "kafka_ip.sh" {
		cmd := exec.Command("/bin/sh", "kafka_ip.sh")
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			logrus.Info(err)
			os.Exit(1)
		}
		kafkaIP = out.String()
		kafkaIP = strings.TrimSuffix(kafkaIP, "\n")
		kafkaIP = kafkaIP + ":9092"
		logrus.Infof("IP address of kafka-cord-0:%s", kafkaIP)
	} else {
		kafkaIP = GlobalConfig.Kafka
	}

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
	ParseCommandLine()
	ProcessGlobalOptions()
	ShowGlobalOptions()

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	logrus.Info("Launching server...")
	logrus.Info("kafkaInit starting")
	kafkainit()

	ln, err := net.Listen("tcp", GlobalConfig.Local)
	if err != nil {
		fmt.Println("could not listen")
		logrus.Fatalf("did not listen: %v", err)
	}
	defer ln.Close()

	conn, err = grpc.Dial(GlobalConfig.Importer, grpc.WithInsecure())
	if err != nil {
		logrus.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	cc = importer.NewDeviceManagementClient(conn)
	ctx = context.Background()

	loop := true

	for loop {
		connS, err := ln.Accept()
		if err != nil {
			logrus.Fatalf("Accept error: %v", err)
		}
		cmdstr, _ := bufio.NewReader(connS).ReadString('\n')
		cmdstr = strings.TrimSuffix(cmdstr, "\n")
		s := strings.Split(cmdstr, " ")
		newmessage := ""
		cmd := string(s[0])

		switch cmd {

		case "attach":
			if len(s) < 2 {
				newmessage = newmessage + "invalid command length" + cmdstr + "\n"
				break
			}
			var devicelist importer.DeviceList
			var ipattached []string
			for _, devinfo := range s[1:] {
				info := strings.Split(devinfo, ":")
				if len(info) != 3 {
					newmessage = newmessage + "invalid command " + devinfo + "\n"
					continue
				}
				deviceinfo := new(importer.DeviceInfo)
				deviceinfo.IpAddress = info[0] + ":" + info[1]
				freq, err := strconv.ParseUint(info[2], 10, 32)
				if err != nil {
					newmessage = newmessage + "invalid command " + devinfo + "\n"
					continue
				}
				deviceinfo.Frequency = uint32(freq)
				devicelist.Device = append(devicelist.Device, deviceinfo)
				ipattached = append(ipattached, deviceinfo.IpAddress)
			}
			if len(devicelist.Device) == 0 {
				break
			}
			_, err := cc.SendDeviceList(ctx, &devicelist)
			if err != nil {
				errStatus, _ := status.FromError(err)
				newmessage = newmessage + errStatus.Message()
				logrus.Errorf("attach error - status code %v message %v", errStatus.Code(), errStatus.Message())
			} else {
				sort.Strings(ipattached)
				ips := strings.Join(ipattached, " ")
				newmessage = newmessage + ips + " attached\n"
			}
		case "delete":
			if len(s) < 2 {
				newmessage = newmessage + "invalid command " + cmdstr + "\n"
				break
			}
			var devicelist importer.DeviceListByIp
			for _, ip := range s[1:] {
				addr := strings.Split(ip, ":")
				if len(addr) != 2 {
					newmessage = newmessage + "invalid address " + ip + "\n"
					continue
				}
				devicelist.Ip = append(devicelist.Ip, ip)
			}
			if len(devicelist.Ip) == 0 {
				break
			}
			_, err := cc.DeleteDeviceList(ctx, &devicelist)
			if err != nil {
				errStatus, _ := status.FromError(err)
				newmessage = newmessage + errStatus.Message()
				logrus.Errorf("delete error - status code %v message %v", errStatus.Code(), errStatus.Message())
			} else {
				sort.Strings(devicelist.Ip)
				ips := strings.Join(devicelist.Ip, " ")
				newmessage = newmessage + ips + " deleted\n"
			}
		case "period":
			if len(s) != 2 {
				newmessage = newmessage + "invalid command " + cmdstr + "\n"
				break
			}
			args := strings.Split(s[1], ":")
			if len(args) != 3 {
				newmessage = newmessage + "invalid command " + s[1] + "\n"
				break
			}
			ip := args[0] + ":" + args[1]
			pv := args[2]
			u, err := strconv.ParseUint(pv, 10, 64)
			if err != nil {
				logrus.Error("ParseUint error!!\n")
			} else {
				freqinfo := new(importer.FreqInfo)
				freqinfo.Frequency = uint32(u)
				freqinfo.IpAddress = ip
				_, err := cc.SetFrequency(ctx, freqinfo)

				if err != nil {
					errStatus, _ := status.FromError(err)
					newmessage = newmessage + errStatus.Message()
					logrus.Errorf("period error - status code %v message %v", errStatus.Code(), errStatus.Message())
				} else {
					newmessage = newmessage + "data collection interval configured to " + pv + " seconds\n"
				}
			}
		case "sub", "unsub":
			if len(s) != 2 {
				newmessage = newmessage + "invalid command " + cmdstr + "\n"
				break
			}
			args := strings.Split(s[1], ":")
			if len(args) < 3 {
				newmessage = newmessage + "invalid command " + s[1] + "\n"
				break
			}
			giveneventlist := new(importer.GivenEventList)
			giveneventlist.EventIpAddress = args[0] + ":" + args[1]
			for _, event := range args[2:] {
				if value, ok := EVENTS_MAP[event]; ok {
					giveneventlist.Events = append(giveneventlist.Events, value)
				}
			}
			if len(giveneventlist.Events) == 0 {
				newmessage = newmessage + "No valid event was given\n"
			}
			var err error
			if cmd == "sub" {
				_, err = cc.SubscribeGivenEvents(ctx, giveneventlist)
			} else {
				_, err = cc.UnsubscribeGivenEvents(ctx, giveneventlist)
			}
			if err != nil {
				errStatus, _ := status.FromError(err)
				newmessage = newmessage + errStatus.Message()
				logrus.Errorf("Un/subscribe error - status code %v message %v", errStatus.Code(), errStatus.Message())
			} else {
				newmessage = newmessage + cmd + " successful\n"
			}
		case "showeventlist":
			if len(s) != 2 {
				newmessage = newmessage + "invalid command " + cmdstr + "\n"
				break
			}
			currentdeviceinfo := new(importer.Device)
			currentdeviceinfo.IpAddress = s[1]
			ret_msg, err := cc.GetEventList(ctx, currentdeviceinfo)
			if err != nil {
				errStatus, _ := status.FromError(err)
				newmessage = errStatus.Message()
				logrus.Errorf("showeventlist error - status code %v message %v", errStatus.Code(), errStatus.Message())
			} else {
				fmt.Print("showeventlist ", ret_msg.Events)
				sort.Strings(ret_msg.Events[:])
				newmessage = strings.Join(ret_msg.Events[:], " ")
				newmessage = newmessage + "\n"
			}
		case "showdeviceeventlist":
			if len(s) != 2 {
				newmessage = newmessage + "invalid command " + s[1] + "\n"
				break
			}
			currentdeviceinfo := new(importer.Device)
			currentdeviceinfo.IpAddress = s[1]
			ret_msg, err := cc.GetCurrentEventList(ctx, currentdeviceinfo)
			if err != nil {
				errStatus, _ := status.FromError(err)
				logrus.Errorf("showdeviceeventlist error - status code %v message %v", errStatus.Code(), errStatus.Message())
				newmessage = newmessage + errStatus.Message()
			} else {
				fmt.Print("showdeviceeventlist ", ret_msg.Events)
				sort.Strings(ret_msg.Events[:])
				newmessage = strings.Join(ret_msg.Events[:], " ")
				newmessage = newmessage + "\n"
			}
		case "cleardeviceeventlist":
			if len(s) != 2 {
				newmessage = newmessage + "invalid command " + s[1] + "\n"
				break
			}
			currentdeviceinfo := new(importer.Device)
			currentdeviceinfo.IpAddress = s[1]
			_, err := cc.ClearCurrentEventList(ctx, currentdeviceinfo)
			if err != nil {
				errStatus, _ := status.FromError(err)
				newmessage = newmessage + errStatus.Message()
				logrus.Errorf("cleardeviceeventlist error - status code %v message %v", errStatus.Code(), errStatus.Message())
			} else {
				newmessage = newmessage + currentdeviceinfo.IpAddress + " events cleared\n"
			}
		case "QUIT":
			loop = false
			newmessage = "QUIT"

		case "showdevices":
			cmd_size := len(s)
			logrus.Infof("cmd is : %s cmd_size: %d", cmd, cmd_size)
			if cmd_size > 2 || cmd_size < 0 {
				logrus.Error("error event showdevices !!")
				newmessage = "error event !!"
			} else {
				err, currentlist := GetCurrentDevices()

				if err != nil {
					errStatus, _ := status.FromError(err)
					logrus.Errorf("GetCurrentDevice error: %s Status code: %d", errStatus.Message(), errStatus.Code())
					newmessage = errStatus.Message()
					fmt.Print("showdevices error!!")
				} else {
					fmt.Print("showdevices ", currentlist)
					sort.Strings(currentlist[:])
					newmessage = strings.Join(currentlist[:], " ")
					newmessage = newmessage + "\n"
				}
			}
		default:
			newmessage = newmessage + "invalid command " + cmdstr + "\n"
		}
		// send string back to client
		n, err := connS.Write([]byte(newmessage + ";"))
		if err != nil {
			logrus.Errorf("err writing to client:%s, n:%d", err, n)
			return
		}
	}
}
