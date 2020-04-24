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

FROM golang:1.12 AS build-env
SHELL ["/bin/bash", "-o",  "pipefail", "-c"]
RUN /bin/true | cat
RUN apt-get update && apt-get install --no-install-recommends -y --allow-downgrades \
	git=1:2.20.1-2+deb10u1 \
	gcc=4:8.3.0-1 \
	curl=7.64.0-4 \
	unzip=6.0-23+deb10u1
# Install these prereqs now so they get built into a container layer and cached
# TODO: Revisit once demo-test implements mod=vendor
RUN go get -v google.golang.org/grpc \
	&& go get -v github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway \
	&& go get -v github.com/golang/protobuf/protoc-gen-go \
	&& go get github.com/sirupsen/logrus \
	&& go get github.com/Shopify/sarama
# Install protoc, same steps as in main importer dockerfile
ENV PROTOC_VERSION="3.7.0"
ENV PROTOC_SHA256SUM="a1b8ed22d6dc53c5b8680a6f1760a305b33ef471bece482e92728f00ba2a2969"
RUN curl -L -o /tmp/protoc-${PROTOC_VERSION}-linux-x86_64.zip https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip \
 && mkdir /tmp/protoc3 \
 && echo "$PROTOC_SHA256SUM  /tmp/protoc-${PROTOC_VERSION}-linux-x86_64.zip" | sha256sum -c - \
 && unzip /tmp/protoc-${PROTOC_VERSION}-linux-x86_64.zip -d /tmp/protoc3 \
 && mv /tmp/protoc3/bin/* /usr/local/bin/ \
 && mv /tmp/protoc3/include/* /usr/local/include/
RUN mkdir /app
COPY . /app/
WORKDIR /app
ENV GO111MODULE=on
ENV PROTOC_VERSION="3.7.0"
ENV PROTOC_SHA256SUM="a1b8ed22d6dc53c5b8680a6f1760a305b33ef471bece482e92728f00ba2a2969"

WORKDIR /app/demo_test
RUN make prereq
RUN make proto/importer.pb.go
# Note: "make demotest" produces an executable that throws "No such file" errors. Investigate.
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -o demotest .

# Also build the client, as having it inside the container may facilitate testing without
# having to setup port forwardin
WORKDIR /app/demo_test/functional_test
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -o dm .

FROM alpine:3.9.4
WORKDIR /app/demo_test
COPY --from=build-env /app/demo_test/demotest .
COPY --from=build-env /app/demo_test/functional_test/dm .
ENTRYPOINT ["./demotest"]

