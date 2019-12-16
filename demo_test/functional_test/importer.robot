# Copyright 2017-present Open Networking Foundation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

*** Settings ***
Library           Process
Library           OperatingSystem

*** Test Cases ***
List Supported Events
    [Documentation]    This test case excercises the API, GetEventList, which is expected to list all supported Redfish events.
    ${IP}    set variable    192.168.4.26
    ${PORT}    set variable    8888
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ipaddr/${IP}:${PORT}/g' tests/list_supported_events.expected
    Run Process    tests/list_supported_events.tc    ${IP}:${PORT}    shell=yes    alias=myproc
    ${OUTPUT}=    get process result    myproc
    Should Be Equal    ${EXPECTED}    ${OUTPUT.stdout}

