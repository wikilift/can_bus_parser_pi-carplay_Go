# can-service

`can-service` is a small Go service that reads data from a **SocketCAN** interface and exposes
vehicle telemetry over a **WebSocket server (port 8080)**.

It is intended to run on Linux systems with USB CAN adapters such as **Candlelight** or compatible
devices, which are auto-detected by the kernel.

---

## CAN frames source

The initial CAN frames and signal mappings used in this project were obtained from:

https://www.loopybunny.co.uk/CarPC/k_can.html

Huge thanks to the author for documenting and sharing this reverse-engineering work.

---

## Communication flow

### 1. CAN input
- Reads raw CAN frames from a SocketCAN interface (e.g. `can0`)
- Decodes basic vehicle telemetry (speed, RPM, temperatures, fuel, etc.)

### 2. WebSocket output
- Starts a WebSocket server on **port 8080**
- Clients can connect without authentication
- On connection, a lightweight handshake is performed:
  - Server sends a capability/status message
  - Client starts receiving telemetry updates immediately

### 3. Update rate
- Telemetry is pushed at a **soft refresh rate of ~20–50 Hz**
- Designed to be smooth for UI rendering without saturating the bus or network

---

## Data format

Telemetry is sent as JSON objects containing decoded CAN values
(e.g. speed, RPM, temperatures, flags, errors).

The protocol is intentionally simple to allow easy integration
from web, desktop, or embedded frontends.

---

## Current limitations / TODO

Planned improvements include:
	•	Per-vehicle CAN mappings
	•	Persistent database (SQLite or similar)
	•	Remote update capability for the service binary

⸻

Status

This project is functional but experimental.
The protocol and data model may evolve as more vehicles are supported.