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
	"encoding/json"
	"fmt"
	"net/http"
	"bytes"
	"regexp"
	"strconv"
	"time"
	"os"
)

const RF_SUBSCRIPTION = "/EventService/Subscriptions"

func add_subscription(ip string, event string) (rtn bool, id uint) {
	rtn = false
	id = 0

	destip := os.Getenv("DEVICE_MANAGEMENT_DESTIP") + ":" + os.Getenv("DEVICE_MANAGEMENT_DESTPORT")
	subscrpt_info := map[string]interface{}{"Context":"TBD","Protocol":"Redfish"}
	subscrpt_info["Name"] = event + " event subscription"
	subscrpt_info["Destination"] = "https://" + destip
	subscrpt_info["EventTypes"] = []string{event}
	sRequestJson, err := json.Marshal(subscrpt_info)
	uri := ip + REDFISH_ROOT + RF_SUBSCRIPTION
	client := http.Client{Timeout: 10 * time.Second}
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
		fmt.Println("Add ", event, " subscription failed. HTTP response status: ",  resp.Status)
		return
	}

	rtn = true
	loc := resp.Header["Location"]
	re := regexp.MustCompile(`/(\w+)$`)
	match := re.FindStringSubmatch(loc[0])
	idint, _ := strconv.Atoi(match[1])
	id = uint(idint)
	fmt.Println("Subscription", event, "id", id, "was successfully added")
	return
}

func remove_subscription(ip string, id uint) bool {
	uri := ip + REDFISH_ROOT + RF_SUBSCRIPTION + strconv.Itoa(int(id))
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
	fmt.Println("Subscription id", id, "was successfully removed")
	return true
}

