This is a mockup of an OLT Running Redfish.

It may be started as follows:

```bash
docker build -t opendcord/redfish-mockup-server:latest -f Dockerfile.redfish_mockup_server .
docker run -d -p 127.0.0.1:8401:8001 opencord/redfish-mockup-server:latest
```

The mockup was generated by running https://github.com/DMTF/Redfish-Mockup-Creator against
a physical Edgecore OLT.