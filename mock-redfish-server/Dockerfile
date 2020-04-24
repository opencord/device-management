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

# To generate mockup, run this inside container:
# cd /Redfish-Mockup-Creator && mkdir mockup && python ./redfishMockupCreate.py -v -r 10.90.0.114:8888 -A None -S -D /Redfish-Mockup-Creator/mockup && tar -czf olt-redfish-mockup.tar.gz mockup/redfish

# Deployment example to deploy two mock OLTs:
# docker run -d -p 127.0.0.1:8401:8001 opencord/redfish-mockup-server:latest
# docker run -d -p 127.0.0.1:8402:8001 opencord/redfish-mockup-server:latest

FROM python:3.5.9
RUN git clone --depth=1 --branch 1.0.9 https://github.com/DMTF/Redfish-Mockup-Server.git
RUN git clone --depth=1 --branch 1.0.5 https://github.com/DMTF/Redfish-Mockup-Creator.git

WORKDIR /Redfish-Mockup-Server
RUN pip install -r requirements.txt

COPY mockup ./mockup

RUN openssl req -newkey rsa:2048 -nodes -keyout key.pem -x509 -days 365 -out certificate.pem -subj "/C=NL/ST=CA/L=Menlo/O=ONF/OU=Development/CN=redfish.mock.server"

CMD python redfishMockupServer.py -H 0.0.0.0 -p 8001 -T -s --cert certificate.pem --key key.pem -D /Redfish-Mockup-Server/mockup
