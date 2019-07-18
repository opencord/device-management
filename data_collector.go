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
	"net/http"
	"fmt"
	"encoding/json"
	"regexp"
	"strings"
	"io/ioutil"
)

func get_status(ip string, service string) (rtn bool, data []string) {
	rtn = false

	uri := ip + REDFISH_ROOT + service
	resp, err := http.Get(uri)
	if err != nil {
		fmt.Println(err)
		return
	}
	body := make(map[string]interface{})
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	if members, ok := body["Members"]; ok {
		re := regexp.MustCompile(`\[([^\[\]]*)\]`)
		memberstr := fmt.Sprintf("%v", members)
		matches := re.FindAllString(memberstr, -1)
		for _, match := range matches {
			m := strings.Trim(match, "[]")
			uri = ip + strings.TrimPrefix(m, "@odata.id:")
			resp, err = http.Get(uri)
		        if err != nil {
			        fmt.Println(err)
			} else {
				b, err := ioutil.ReadAll(resp.Body)
			        if err != nil {
				        fmt.Println(err)
				} else {
					data = append(data, string(b))
					rtn = true
				}
			}
			defer resp.Body.Close()
		}
	}
	return 
}
