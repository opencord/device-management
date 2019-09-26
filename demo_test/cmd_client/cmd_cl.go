// Copyright 2018-present Open Networking Foundation
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

import "net"
import "fmt"
import "bufio"
import "os"
import "strings"

var attach_ip string = ""

func main() {
	// connect to this socket
	var message string = ""
	conn, _ := net.Dial("tcp", "127.0.0.1:9999")
	reader := bufio.NewReader(os.Stdin)
	for {
		// read in input from stdin
		if(attach_ip != ""){
			fmt.Printf("[%v] CMD to send :", attach_ip)
		}else{
			fmt.Print("CMD to send :")
		}
		text, _ := reader.ReadString('\n')

		// send to socket
		fmt.Fprintf(conn, text + "\n")

                cmd := strings.TrimSuffix(text, "\n")
                s := strings.Split(cmd, ":")
                cmd = s[0]

		if(cmd == "attach"){
			// listen for reply
			t_attach_ip, _ := bufio.NewReader(conn).ReadString('\n')
			attach_ip = strings.TrimSuffix(t_attach_ip, "\n")
		}else{
			// listen for reply
			message, _ = bufio.NewReader(conn).ReadString('\n')
			fmt.Print("Return from server: " + message)
		}

		if message == "QUIT\n"{
			break
		}
	}
}
