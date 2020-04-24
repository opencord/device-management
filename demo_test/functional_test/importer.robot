# Copyright 2018-present Open Networking Foundation
# Copyright 2018-present Edgecore Networks Corporation
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
Documentation     Provide the function to perform funtional tests for the Redfish device-management project
Suite Setup       Setup Suite
Suite Teardown    Teardown Suite
Library           Process
Library           OperatingSystem
Library           BuiltIn
Library           String
Library           Collections
Library           ../../voltha-system-tests/libraries/DependencyLibrary.py
Resource           ../../voltha-system-tests/libraries/k8s.robot

*** Variables ***
@{ADDR_LIST}      192.168.4.26:8888    192.168.4.27:8888
${timeout}        60s
${use_mock_redfish}    False
${use_containerized_dm}    False
${voltha_suite_setup}    False
${IMPORTER_POD_NAME}    redfish-importer
${DEMOTEST_POD_NAME}    redfish-demotest

*** Test Cases ***
Add Device to Monitor
    [Documentation]    This test case excercises the API, SendDeviceList, which registers Redfish devices to monitor.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' -e 's/ip2/${IP2}/g' -e 's/port2/${PORT2}/g' tests/add_device_to_monitor.expected
    ${OUTPUT}=    Run Test    tests/add_device_to_monitor.tc    ${IP1}    ${PORT1}    ${IP2}    ${PORT2}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

Clear Subscribed Events
    [Documentation]    This test case excercises the API, ClearCurrentEventList, which clears all Redfish evets currently subscribed to.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' tests/clear_all_subscribed_events.expected
    ${OUTPUT}=    Run Test    tests/clear_all_subscribed_events.tc    ${IP1}    ${PORT1}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

Configure Data Polling Interval
    [Documentation]    This test case excercises the API, SetFrequency, which configures the interval of data polling.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' tests/configure_data_polling_interval.expected
    ${OUTPUT}=    Run Test    tests/configure_data_polling_interval.tc    ${IP1}    ${PORT1}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

Delete Monitored Device
    [Documentation]    This test case excercises the API, DeleteDeviceList, which deletes Redfish devices being monitored.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' -e 's/ip2/${IP2}/g' -e 's/port2/${PORT2}/g' tests/delete_monitored_device.expected
    ${OUTPUT}=    Run Test    tests/delete_monitored_device.tc    ${IP1}    ${PORT1}    ${IP2}    ${PORT2}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

List Devices monitored
    [Documentation]    This test case excercises the API, GetCurrentDevices, which lists all Redfish devices being monitored.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' -e 's/ip2/${IP2}/g' -e 's/port2/${PORT2}/g' tests/list_device_monitored.expected
    ${OUTPUT}=    Run Test    tests/list_device_monitored.tc    ${IP1}    ${PORT1}    ${IP2}    ${PORT2}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

List Subscribed Events
    [Documentation]    This test case excercises the API, GetCurrentEventList, which lists all Redfish evets currently subscribed to.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' tests/list_subscribed_events.expected
    ${OUTPUT}=    Run Test    tests/list_subscribed_events.tc    ${IP1}    ${PORT1}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

List Supported Events
    [Documentation]    This test case excercises the API, GetEventList, which lists all supported Redfish events.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' tests/list_supported_events.expected
    ${OUTPUT}=    Run Test    tests/list_supported_events.tc    ${IP1}    ${PORT1}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

Subscribe Events
    [Documentation]    This test case excercises the API, SubscribeGivenEvents, which subscribes to the specified events.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' tests/subscribe_events.expected
    ${OUTPUT}=    Run Test    tests/subscribe_events.tc    ${IP1}    ${PORT1}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

Unsubscribe Events
    [Documentation]    This test case excercises the API, UnsubscribeGivenEvents, which unsubscribes to the specified events.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' tests/unsubscribe_events.expected
    ${OUTPUT}=    Run Test    tests/unsubscribe_events.tc    ${IP1}    ${PORT1}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

Validate IP
    [Documentation]    This test case validates the format of IP, whcih is expected to be in the form of <ip>:<port>.
    ${EXPECTED}=    RUN    sed -e '/^\\/\\//d' -e 's/ip1/${IP1}/g' -e 's/port1/${PORT1}/g' tests/validate_ip.expected
    ${OUTPUT}=    Run Test    tests/validate_ip.tc    ${IP1}    ${PORT1}
    Should Be Equal    ${EXPECTED}    ${OUTPUT}

*** Keywords ***
Setup Suite
    [Documentation]    Set up the test suite
    # Common voltha-system-test related setup. Only do this with a physical OLT, when called from ONF Jenkins
    Run Keyword If     ${voltha_suite_setup}    Common Test Suite Setup
    # Ensure the redfish import and demotest containers are deployed and running.
    Run Keyword If     ${use_mock_redfish}    Install Mock Redfish Server
    Get IP AND PORT

Teardown Suite
    [Documentation]    Clean up devices if desired
    ...    kills processes and cleans up interfaces on src+dst servers
    Run Keyword If    ${use_mock_redfish}    Clean Up Mock Redfish Server

Install Mock Redfish Server
    Apply Kubernetes Resources    ../../kubernetes/deploy-redfish-importer.yaml    default
    Wait Until Keyword Succeeds    ${timeout}    5s
    ...    Validate Pod Status    ${IMPORTER_POD_NAME}    default     Running
    Wait Until Keyword Succeeds    ${timeout}    5s
    ...    Validate Pod Status    ${DEMOTEST_POD_NAME}    default     Running

Clean Up Mock Redfish Server
    Delete Kubernetes Resources    ../../kubernetes/deploy-redfish-importer.yaml    default

Get IP AND PORT
    Sort List    ${ADDR_LIST}
    ${I1}=    Fetch From LEFT    ${ADDR_LIST}[0]    :
    Set Suite Variable    ${IP1}    ${I1}
    ${P1}=    Fetch From Right    ${ADDR_LIST}[0]    :
    Set Suite Variable    ${PORT1}    ${P1}
    ${I2}=    Fetch From LEFT    ${ADDR_LIST}[1]    :
    Set Suite Variable    ${IP2}    ${I2}
    ${P2}=    Fetch From Right    ${ADDR_LIST}[1]    :
    Set Suite Variable    ${PORT2}    ${P2}

Run Test
    [Arguments]    @{args}
    ${output}=    Run Keyword if    ${use_containerized_dm}
    ...    Run Test In Container     @{args}
    ...    ELSE
    ...    Run Test On Host     @{args}
    [Return]    ${output}

Run Test On Host
    [Arguments]    ${testname}    @{args}
    ${output}=    Run Process    ${testname}    @{args}
    [Return]    ${output.stdout}

Run Test In Container
    [Arguments]    ${testname}    @{args}
    Copy File To Pod    default    ${DEMOTEST_POD_NAME}    ${testname}    "/test.tc"
    ${argList}=    Evaluate  " ".join($args)
    ${output}=    Exec Pod    default    ${DEMOTEST_POD_NAME}    sh /test.tc ${argList}
    [Return]    ${output}
