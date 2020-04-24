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

SHELL = /bin/bash
DEMOTESTDIR = ..
DEMOTESTLOG = demotest.log
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
ADDR1 := $(shell printf '${IP1}:${PORT1}\n${IP2}:${PORT2}' | sort | sed -n '1 p')
ADDR2 := $(shell printf '${IP1}:${PORT1}\n${IP2}:${PORT2}' | sort | sed -n '2 p')
I1 := $(shell echo ${ADDR1} | cut -d ':' -f 1)
P1 := $(shell echo ${ADDR1} | cut -d ':' -f 2)
I2 := $(shell echo ${ADDR2} | cut -d ':' -f 1)
P2 := $(shell echo ${ADDR2} | cut -d ':' -f 2)

demotest:
	@cd ${DEMOTESTDIR}; make demotest || exit 1; ./demotest > ${DEMOTESTLOG} 2>&1 & \
	sleep 2

dm: test_cli.go
	go build -i -v -o $@

test: check-arg dm demotest $(TESTS) clean
	@echo "All tests passed!"

$(TESTS): ${TESTSDIR}/%.${TESTTAG}:
	@sed -e '/^\/\//d' -e 's/ip1/${I1}/g; s/port1/${P1}/g; s/ip2/${I2}/g; s/port2/${P2}/g' ${TESTSDIR}/$*.expected > ${TESTSDIR}/$*.e
#	@sed -i '/^\/\//d' ${TESTSDIR}/$*.e
	@echo "===== Running test $@"
	@./$@ $(I1) $(P1) $(I2) $(P2) > /dev/null > ${TESTSDIR}/$*.out
	@diff -q ${TESTSDIR}/$*.out ${TESTSDIR}/$*.e || (pkill demotest; exit 1)
	@rm ${TESTSDIR}/*.out ${TESTSDIR}/*.e

clean:
	@pkill demotest
	@rm ${DEMOTESTDIR}/${DEMOTESTLOG}
