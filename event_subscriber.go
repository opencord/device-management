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
	"net/http"
	"os"
	"regexp"
	"io/ioutil"
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
	uri := RF_DEFAULT_PROTOCOL + ip + RF_SUBSCRIPTION
	client := s.httpclient
	resp, err := client.Post(uri, CONTENT_TYPE, bytes.NewBuffer(sRequestJson))
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		fmt.Println(err)
		return
	}

	if resp.StatusCode != 201 {
		result := make(map[string]interface{})
		json.NewDecoder(resp.Body).Decode(&result)
		fmt.Println(result)
		fmt.Println(result["data"])
		fmt.Println("Add ", event, " subscription failed. HTTP response status: ", resp.Status)
		return
	}
	rtn = true
	loc := resp.Header["Location"]
	re := regexp.MustCompile(`/(\w+)$`)
	match := re.FindStringSubmatch(loc[0])
	s.devicemap[ip].Subscriptions[event] = match[1]

	fmt.Println("Subscription", event, "id", match[1], "was successfully added")
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
		fmt.Println(err)
		return false
	}

	if code := resp.StatusCode; code < 200 && code > 299 {
		result := make(map[string]interface{})
		json.NewDecoder(resp.Body).Decode(&result)
		fmt.Println(result)
		fmt.Println(result["data"])
		fmt.Println("Remove subscription failed. HTTP response status:", resp.Status)
		return false
	}
	delete(s.devicemap[ip].Subscriptions, event)

	fmt.Println("Subscription id", id, "was successfully removed")
	return true
}

func (s *Server) get_event_types(ip string) (eventtypes []string ) {
	resp, err := http.Get(RF_DEFAULT_PROTOCOL + ip + RF_EVENTSERVICE)
	fmt.Println("get_event_types")
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	m := map[string]interface{}{}
	err = json.Unmarshal([]byte(body), &m)
	if err != nil {
		fmt.Println(err)
		return
	}
	e := m["EventTypesForSubscription"].([]interface{})
	fmt.Printf("supported event types %v\n", e)
	for _, val := range e {
		eventtypes = append(eventtypes, val.(string))
	}
	return
}
