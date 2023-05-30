# Qrystal

[Website/Docs](https://nyiyui.ca/qrystal) /
[Github.com](https://github.com/nyiyui/qrystal)

Qrystal /kristl/ sets up several WireGuard tunnels between servers.
In addition, it provides centralised configuration management.
Nodes and tokens can be dynamically added (and removed, in a future
version).

## Installation

Make sure to open the appropriate ports (defaults listed below):
- CS: 39252/tcp for Nodes and 39253 for utilities
- The WireGuard ports for UDP (from the expected peers)

### Most Linux distros

```sh
$ git clone https://github.com/nyiyui/qrystal
$ cd qrystal
$ mkdir build && cd build
$ make src=.. -f ../Makefile
# make src=.. -f ../Makefile install
```

Then, enable/start `qrystal-runner.service` (Node) and/or `qrystal-cs.service` (CS)
(depending on what you want to run).

### NixOS

Flakes are recommended. See `flake.nix` for options.

## Installation from Generic Archive

```
# make pre_install # if Qrystal services are already running
# make src=. install
# systemctl start qrystal-runner # for Node
# systemctl start qrystal-cs # for CS
```

## TODO

- node: wg config persistent over reboot, etc (change only when cs requests so, or when wg conn fails?)
- confine qrystal-node and qrystal-cs (using systemd's options)
- configure existing interfaces without disrupting connections (as much as possible)
- support multiple hosts
  - e.g. specify VPC network IP address first, and then public IP address
  - heuristics for a successful wg connection?
