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

FROM golang:1.12-alpine3.9 AS build-env  
RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:3.9.4
WORKDIR /app/
COPY --from=build-env /app/main .
ENTRYPOINT ["./main"]

