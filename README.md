# home2grafana

_home2grafana_ provides power consumption metrics in prometheus format. It reads from various devices, like iobroker
power meters, and collects the information on a regular basis. You can then use a Prometheus/Grafana instance to monitor
your power consumption.

## Device Definition

Devices are described by YAML files inside the _setup_ directory.

### Tasmota

    ---
    source:
    provider: iobroker
    metric: energy_watt_total
    interval: 90s
    devices:
      - address: 192.168.160.200
        name: Waschtrockner
        room: Bad
      - address: 192.168.160.201
        room: Waschküche

### Homematic

Each device definition files for homematic devices need a homematic CCUx running and accessible. The definition can
define any number of devices attached to this CCUx. At least a _hm_name_ must be defined. The name and room will be
extract from the meta-data stored in CCUx. You can also overwrite this.

Currently, the followin homematic devices are supported

* HMIP-PSM
* HM-ES-PMSw1-Pl

In case you want to add new devices have a look at _homematic.go_ and add the device.

    ---
    source:
    provider: homematic
    metric: energy_watt_total
    interval: 120s
    address: 192.168.160.21
    devices:
      - hm_name: HmIP-RF.0001DD89971DDD
        name: Server
        room: RZ
      - hm_name: BidCos-RF.LEQ0535163
