# device-management Repo.

This Repo contains the code for importer and related functionality. Importer is module which collects the
device data from the devices which support REDFISH and publishes onto kafka bus. Exporter is another module
which listens on kafka bus and makes the data available to the dashboard for user.

# Importer

Importer gets the device details from NEM and periodicaly collects data using REDFISH RESTful APIS based on HTTP.
The interface  between NEM andimporter is GRPC.  Importer also registers for events from the device like alerts,
removal/insertion events. It then publishes data on kafka bus to collect the data.
