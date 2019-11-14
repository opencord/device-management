# copyright 2018-present Open Networking Foundation
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

# docker build -t opencord/device-management:latest .
# docker build -t 10.90.0.101:30500/opencord/kafka-topic-exporter:latest .

FROM golang:1.12 AS build-env
SHELL ["/bin/bash", "-o",  "pipefail", "-c"]
RUN /bin/true | cat
RUN mkdir /app
COPY . /app/
WORKDIR /app
ENV PROTOC_VERSION 3.6.1
ENV PROTOC_SHA256SUM 6003de742ea3fcf703cfec1cd4a3380fd143081a2eb0e559065563496af27807
RUN apt-get update && apt-get install --no-install-recommends -y \
	git=1:2.20.1-2 \
	gcc=4:8.3.0-1 \
	curl=7.64.0-4 \
	unzip=6.0-23+deb10u1
RUN curl -L -o /tmp/protoc-${PROTOC_VERSION}-linux-x86_64.zip https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip 
RUN mkdir /tmp/protoc3
RUN   echo "$PROTOC_SHA256SUM  /tmp/protoc-${PROTOC_VERSION}-linux-x86_64.zip" | sha256sum -c - \
 &&  unzip /tmp/protoc-${PROTOC_VERSION}-linux-x86_64.zip -d /tmp/protoc3 \
 && mv /tmp/protoc3/bin/* /usr/local/bin/ \
 && mv /tmp/protoc3/include/* /usr/local/include/
RUN go get -u "google.golang.org/grpc" \
    && go get "github.com/golang/protobuf/proto" \
    && go get -v "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway" \
    && go get -v "github.com/golang/protobuf/protoc-gen-go" \
    && go get "github.com/Shopify/sarama" \
    && go get "github.com/Sirupsen/logrus"\
    &&  protoc -I proto -I"${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis" --go_out=plugins=grpc:proto/  proto/*.proto
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:3.9.4
WORKDIR /app/
COPY https-server.crt .
COPY https-server.key .
COPY --from=build-env /app/main .
ENTRYPOINT ["./main"]

