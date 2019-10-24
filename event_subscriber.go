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
)

const RF_SUBSCRIPTION = "/EventService/Subscriptions/"

func (s *Server) add_subscription(ip string, event string, f *os.File) (rtn bool) {
	rtn = false

	destip := os.Getenv("EVENT_NOTIFICATION_DESTIP") + ":" + os.Getenv("DEVICE_MANAGEMENT_DESTPORT")
	subscrpt_info := map[string]interface{}{"Context": "TBD-" + destip, "Protocol": "Redfish"}
	subscrpt_info["Name"] = event + " event subscription"
	subscrpt_info["Destination"] = "https://" + destip
	subscrpt_info["EventTypes"] = []string{event}
	sRequestJson, err := json.Marshal(subscrpt_info)
	uri := s.devicemap[ip].Protocol + "://" + ip + REDFISH_ROOT + RF_SUBSCRIPTION
	client := s.httpclient
	resp, err := client.Post(uri, CONTENT_TYPE, bytes.NewBuffer(sRequestJson))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

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

	if f != nil {
		b, err := json.Marshal(s.devicemap[ip])
		fmt.Println(string(b))
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
		fmt.Println("file handle is nil")
	}

	fmt.Println("Subscription", event, "id", match[1], "was successfully added")
	return
}

func (s *Server) remove_subscription(ip string, event string, f *os.File) bool {
	id := s.devicemap[ip].Subscriptions[event]
	uri := s.devicemap[ip].Protocol + "://" + ip + REDFISH_ROOT + RF_SUBSCRIPTION + id
	req, _ := http.NewRequest("DELETE", uri, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer resp.Body.Close()

	if code := resp.StatusCode; code < 200 && code > 299 {
		result := make(map[string]interface{})
		json.NewDecoder(resp.Body).Decode(&result)
		fmt.Println(result)
		fmt.Println(result["data"])
		fmt.Println("Remove subscription failed. HTTP response status:", resp.Status)
		return false
	}
	delete(s.devicemap[ip].Subscriptions, event)

	if f != nil {
		b, err := json.Marshal(s.devicemap[ip])
		if err != nil {
			fmt.Println(err)
		} else {
			f.Truncate(0)
			f.Seek(0, 0)
			n, err := f.Write(b)
			if err != nil {
				fmt.Println("!!!!! err wrote", n, "bytes")
				fmt.Println(err)
			} else {
				fmt.Println("wrote", n, "bytes")
			}
		}
	}
	fmt.Println("Subscription id", id, "was successfully removed")
	return true
}
