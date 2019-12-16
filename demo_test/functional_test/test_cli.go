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

func main() {
	// connect to this socket
	var message string = ""
	cmdstr := strings.Join(os.Args[1:], " ")
	conn, _ := net.Dial("tcp", "127.0.0.1:9999")
	// send to socket
	fmt.Fprintf(conn, cmdstr + "\n")

	// listen for reply
	message, _ = bufio.NewReader(conn).ReadString(';')
	message = strings.TrimSuffix(message, ";")
	fmt.Print(message)
}