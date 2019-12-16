# Copyright 2018-present Open Networking Foundation
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

SHELL = /bin/bash
DEMOTESTDIR = ..
TESTSDIR = tests
TESTTAG = tc
TESTS := $(wildcard ${TESTSDIR}/*.$(TESTTAG))

.PHONY : check-arg test demotest $(TESTS) clean

.DEFAULT_GOAL := dm

check-arg:
ifeq ($(strip $(TESTS)),)
	$(error no test found)
endif
ifeq "$(and $(IP1),$(PORT1),$(IP2),$(PORT2))" "" 
	$(error IP1, PORT1, IP2, and PORT2 are not all defined)
endif

demotest:
	@cd ${DEMOTESTDIR}; make demotest || exit 1; ./demotest & \
	sleep 2

dm: test_cli.go
	go build -i -v -o $@

test: check-arg dm demotest $(TESTS) clean
	@echo "Alll tests passed!"

$(TESTS): ${TESTSDIR}/%.${TESTTAG}:
	@sed -e '/^\/\//d' -e 's/ip1/${IP1}/g; s/port1/${PORT1}/g; s/ip2/${IP2}/g; s/port2/${PORT2}/g' ${TESTSDIR}/$*.expected > ${TESTSDIR}/$*.e
#	@sed -i '/^\/\//d' ${TESTSDIR}/$*.e
	@echo "===== Running test $@"
	@./$@ $(IP1) $(PORT1) $(IP2) $(PORT2) > /dev/null > ${TESTSDIR}/$*.out
	@diff -q ${TESTSDIR}/$*.out ${TESTSDIR}/$*.e || (pkill demotest; exit 1)
	@rm ${TESTSDIR}/*.out ${TESTSDIR}/*.e

clean:
	@pkill demotest
