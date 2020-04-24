# Copyright 2019-present Open Networking Foundation
# Copyright 2019-present Edgecore Networks Corporation
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

# Configure shell
SHELL = bash -eu -o pipefail

ROOT_DIR  := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

# Variables
VERSION                  ?= $(shell cat ./VERSION)
CONTAINER_NAME           ?= $(notdir $(abspath .))

## Docker related
DOCKER_REGISTRY          ?=
DOCKER_REPOSITORY        ?=
DOCKER_BUILD_ARGS        ?=
DOCKER_TAG               ?= ${VERSION}
DOCKER_IMAGENAME         := ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}${CONTAINER_NAME}:${DOCKER_TAG}

## Docker labels. Only set ref and commit date if committed
DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url $(shell git remote))
DOCKER_LABEL_VCS_REF     ?= $(shell git diff-index --quiet HEAD -- && git rev-parse HEAD || echo "unknown")
DOCKER_LABEL_COMMIT_DATE ?= $(shell git diff-index --quiet HEAD -- && git show -s --format=%cd --date=iso-strict HEAD || echo "unknown" )
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

ROBOT_MOCK_OLT_FILE             ?= $(ROOT_DIR)/demo_test/functional_test/robot-mock-olt.yaml
ROBOT_DEBUG_LOG_OPT             ?=
ROBOT_MISC_ARGS                 ?=

help:
	@echo "Usage: make [<target>]"
	@echo "where available targets are:"
	@echo
	@echo "proto/importer.pb.go : Build importer.pb.go for go build ./.."
	@echo "build                : Build the docker images."
	@echo "                         - If this is the first time you are building, choose 'make build' option."
	@echo "clean                : Remove files created by the build and tests"
	@echo "docker-push          : Push the docker images to an external repository"
	@echo "lint-dockerfile      : Perform static analysis on Dockerfiles"
	@echo "lint-style           : Verify code is properly gofmt-ed"
	@echo "lint-sanity          : Verify that 'go vet' doesn't report any issues"
	@echo "lint-mod             : Verify the integrity of the 'mod' files"
	@echo "lint                 : Shorthand for lint-style & lint-sanity"
	@echo "test                 : Generate reports for all go tests"
	@echo

all: test

# target to invoke mock-olt robot tests
functional-mock-test: ROBOT_MISC_ARGS += $(ROBOT_DEBUG_LOG_OPT)
functional-mock-test: ROBOT_CONFIG_FILE := $(ROBOT_MOCK_OLT_FILE)
functional-mock-test: ROBOT_FILE := $(ROOT_DIR)/demo_test/functional_test/importer.robot
functional-mock-test: importer-functional-test

proto/importer.pb.go: proto/importer.proto
	mkdir -p proto
	protoc --proto_path=proto \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
	--go_out=plugins=grpc:proto/ \
	proto/importer.proto

build: docker-build

docker-build: docker-build-importer
	make -C demo_test docker-build
	make -C mock-redfish-server docker-build

docker-push: docker-push-importer
	make -C demo_test docker-push
	make -C mock-redfish-server docker-push

docker-build-importer:
	docker build $(DOCKER_BUILD_ARGS) \
	-t ${DOCKER_IMAGENAME} \
	--build-arg org_label_schema_version="${VERSION}" \
	--build-arg org_label_schema_vcs_url="${DOCKER_LABEL_VCS_URL}" \
	--build-arg org_label_schema_vcs_ref="${DOCKER_LABEL_VCS_REF}" \
	--build-arg org_label_schema_build_date="${DOCKER_LABEL_BUILD_DATE}" \
	--build-arg org_opencord_vcs_commit_date="${DOCKER_LABEL_COMMIT_DATE}" \
	-f Dockerfile .

docker-push-importer:
	docker push ${DOCKER_IMAGENAME}


PATH:=$(GOPATH)/bin:$(PATH)
HADOLINT=$(shell PATH=$(GOPATH):$(PATH) which hadolint)

lint-dockerfile:
ifeq (,$(shell PATH=$(GOPATH):$(PATH) which hadolint))
	mkdir -p $(GOPATH)/bin
	curl -o $(GOPATH)/bin/hadolint -sNSL https://github.com/hadolint/hadolint/releases/download/v1.17.1/hadolint-$(shell uname -s)-$(shell uname -m)
	chmod 755 $(GOPATH)/bin/hadolint
endif
	@echo "Running Dockerfile lint check ..."
	@hadolint $$(find .  -type f -not -path "./vendor/*"  -name "Dockerfile")
	@echo "Dockerfile lint check OK"

lint-style:
ifeq (,$(shell which gofmt))
	go get -u github.com/golang/go/src/cmd/gofmt
endif
	@echo "Running style check..."
	@gofmt_out="$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))" ;\
	if [ ! -z "$$gofmt_out" ]; then \
	  echo "$$gofmt_out" ;\
	  echo "Style check failed on one or more files ^, run 'go fmt' to fix." ;\
	  exit 1 ;\
	fi
	@echo "Style check OK"

lint-sanity:proto/importer.pb.go
	@echo "Running sanity check..."
	@go vet -mod=vendor ./...
	@echo "Sanity check OK"

lint-mod:
	@echo "Running dependency check..."
	@go mod verify
	@echo "Dependency check OK"
lint: lint-style  lint-dockerfile lint-sanity lint-mod

# Rules to automatically install golangci-lint
GOLANGCI_LINT_TOOL?=$(shell which golangci-lint)
ifeq (,$(GOLANGCI_LINT_TOOL))
GOLANGCI_LINT_TOOL=$(GOPATH)/bin/golangci-lint
golangci_lint_tool_install:
	# Same version as installed by Jenkins ci-management
	# Note that install using `go get` is not recommended as per https://github.com/golangci/golangci-lint
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(GOPATH)/bin v1.17.0
else
golangci_lint_tool_install:
endif

sca: golangci_lint_tool_install
	rm -rf ./sca-report
	@mkdir -p ./sca-report
	$(GOLANGCI_LINT_TOOL) run --out-format junit-xml ./... 2>&1 | tee ./sca-report/sca-report.xml

test: docker-build

clean:
	@echo "No cleanup available"

# Check out a copy of voltha-system-tests. We will reuse its robot configuration
# and its robot libraries
voltha-system-tests:
	git clone https://github.com/opencord/voltha-system-tests.git

# virtualenv for the robot tools
# VOL-2724 Invoke pip via python3 to avoid pathname too long on QA jobs
vst_venv: voltha-system-tests
	virtualenv -p python3 $@ && \
	VIRTUAL_ENV_DISABLE_PROMPT=true source ./$@/bin/activate && \
	python -m pip install -r voltha-system-tests/requirements.txt

importer-functional-test: vst_venv
	VIRTUAL_ENV_DISABLE_PROMPT=true source ./$</bin/activate ; set -u ;\
	cd demo_test/functional_test ;\
	robot -V $(ROBOT_CONFIG_FILE) $(ROBOT_MISC_ARGS) $(ROBOT_FILE)
