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
	"bytes"
	"encoding/json"
	"fmt"
	logrus "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
)

const RF_EVENTSERVICE = "/redfish/v1/EventService/"
const RF_SUBSCRIPTION = RF_EVENTSERVICE + "Subscriptions/"

func (s *Server) add_subscription(ip string, event string) (rtn bool) {
	rtn = false

	destip := os.Getenv("EVENT_NOTIFICATION_DESTIP") + ":" + os.Getenv("DEVICE_MANAGEMENT_DESTPORT")
	subscrpt_info := map[string]interface{}{"Context": "TBD-" + destip, "Protocol": "Redfish"}
	subscrpt_info["Name"] = event + " event subscription"
	subscrpt_info["Destination"] = RF_DEFAULT_PROTOCOL + destip
	subscrpt_info["EventTypes"] = []string{event}
	sRequestJson, err := json.Marshal(subscrpt_info)
	if err != nil {
		logrus.Errorf("Error JasonMarshal %s", err)
		return
	}
	uri := RF_DEFAULT_PROTOCOL + ip + RF_SUBSCRIPTION
	client := s.httpclient
	resp, err := client.Post(uri, CONTENT_TYPE, bytes.NewBuffer(sRequestJson))
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logrus.Errorf("client post error %s", err)
		return
	}

	if resp.StatusCode != 201 {
		result := make(map[string]interface{})
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&result); err != nil {
			logrus.Errorf("ERROR while adding event subscription:%s " + err.Error())
			return
		}
		logrus.Infof("Result Decode %s", result)
		fmt.Println(result["data"])
		logrus.Errorf("Add %s subscription failed. HTTP response status:%s ", event, resp.Status)
		return
	}
	rtn = true
	loc := resp.Header["Location"]
	re := regexp.MustCompile(`/(\w+)$`)
	match := re.FindStringSubmatch(loc[0])
	s.devicemap[ip].Subscriptions[event] = match[1]

	logrus.Infof("Subscription %s id %s was successfully added", event, match[1])
	return
}

func (s *Server) remove_subscription(ip string, event string) bool {
	id := s.devicemap[ip].Subscriptions[event]
	uri := RF_DEFAULT_PROTOCOL + ip + RF_SUBSCRIPTION + id
	req, _ := http.NewRequest("DELETE", uri, nil)
	resp, err := http.DefaultClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logrus.Errorf("Error DefaultClient.Do %s", err)
		return false
	}

	if code := resp.StatusCode; code < 200 && code > 299 {
		result := make(map[string]interface{})
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&result); err != nil {
			logrus.Errorf("ERROR while removing event subscription: %s ", err.Error())
			return false
		}
		logrus.Infof("Result %s", result)
		fmt.Println(result["data"])
		logrus.Errorf("Remove subscription failed. HTTP response status:%s", resp.Status)
		return false
	}
	delete(s.devicemap[ip].Subscriptions, event)

	logrus.Infof("Subscription id %s was successfully removed", id)
	return true
}

func (s *Server) get_event_types(ip string) (eventtypes []string) {
	resp, err := http.Get(RF_DEFAULT_PROTOCOL + ip + RF_EVENTSERVICE)
	logrus.Info("get_event_types")
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logrus.Errorf("http get Error %s", err)
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorf("Read error %s", err)
		return
	}

	m := map[string]interface{}{}
	err = json.Unmarshal([]byte(body), &m)
	if err != nil {
		logrus.Errorf("ErrorUnmarshal %s", err)
		return
	}
	e := m["EventTypesForSubscription"].([]interface{})
	logrus.Infof("supported event types %v\n", e)
	for _, val := range e {
		eventtypes = append(eventtypes, val.(string))
	}
	return
}
