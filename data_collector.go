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
	"io/ioutil"
	"net/http"
)

/* parse_map() parses the json structure, amap, and returns all sub-folder paths found at the 2nd level of the multiplayer structure
 */
func parse_map(amap map[string]interface{}, level uint, archive map[string]bool) (paths []string) {
	level = level + 1
	for key, val := range amap {
		switch val.(type) {
		case map[string]interface{}:
			p := parse_map(val.(map[string]interface{}), level, archive)
			paths = append(paths, p...)
		case []interface{}:
			p := parse_array(val.([]interface{}), level, archive)
			paths = append(paths, p...)
		default:
			if level == 2 && key == "@odata.id" {
				/* sub-folder path of a resource can be found as the value of the key '@odata.id' showing up at the 2nd level of the data read from a resource. When a path is found, it's checked against the array 'archive' to avoid duplicates. */
				if _, ok := archive[val.(string)]; !ok {
					archive[val.(string)] = true
					paths = append(paths, val.(string))
				}
			}
		}
	}
	return paths
}

/* parse_array() parses any vlaue, if in the form of an array, of a key-value pair found in the json structure, and returns any paths found.
 */
func parse_array(anarray []interface{}, level uint, archive map[string]bool) (paths []string) {
	for _, val := range anarray {
		switch val.(type) {
		case map[string]interface{}:
			p := parse_map(val.(map[string]interface{}), level, archive)
			paths = append(paths, p...)
		}
	}
	return paths
}

/* read_resource() reads data from the specified Redfish resource, including its sub-folders, of the specified device ip and rerutnrs the data read.

Based on careful examination of the data returned from several resources sampled, it was determined that sub-folder paths can be found as the value to the key '@odata.id' showing up at the 2nd level of the data read from a resource.
*/
func read_resource(ip string, resource string, archive map[string]bool) (data []string) {
	resp, err := http.Get(ip + resource)
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

	data = append(data, string(body))

	m := map[string]interface{}{}
	err = json.Unmarshal([]byte(body), &m)
	if err != nil {
		fmt.Println(err)
		return
	}

	resources := parse_map(m, 0, archive)

	for _, resource := range resources {
		d := read_resource(ip, resource, archive)
		data = append(data, d...)
	}
	return data
}

/* sample JSON files can be found in the samples folder */
func (s *Server) get_status(ip string, resource string) (data []string) {
	archive := make(map[string]bool)
	base_ip := RF_DEFAULT_PROTOCOL + ip
	/* 'archive' maintains a list of all resources that will be/have been visited to avoid duplicates */
	archive[resource] = true
	data = read_resource(base_ip, resource, archive)
	return data
}
