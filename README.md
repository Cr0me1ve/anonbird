
<div align="center">
  <p align="center">
    <img width="234" src="docs/media/logo-full.png" alt="AnonBird logo"/>
  </p>
  <p align="center">
    <a href="https://github.com/Cr0me1ve/netbird/blob/main/LICENSE">
      <img src="https://img.shields.io/badge/license-BSD--3-blue" alt="BSD-3 License"/>
    </a>
    <a href="docs/leak-map.md">
      <img src="https://img.shields.io/badge/privacy-leak%20map-orange" alt="AnonBird leak map"/>
    </a>
  </p>
</div>

<p align="center">
  <strong>
    AnonBird is a NetBird fork focused on anonymous private mesh networking over Tor and I2P.
    <br/>
    Start with the <a href="docs/leak-map.md">leak map</a>, <a href="docs/anonbird-i2p-operations.md">I2P operations guide</a>, and <a href="docs/anonbird-release-hardening.md">release hardening notes</a>.
  </strong>
</p>

**AnonBird keeps the familiar WireGuard mesh, management, signal, relay, ACL and dashboard model, but adds anonymous transports and hardens the fork so anonymous deployments do not silently call upstream package, metrics, update, debug-upload, geolocation or cloud endpoints.**

**Tor mode.** `tor-relay-only` forces management, signal and relay traffic through a SOCKS5 Tor path, disables STUN/ICE/direct UDP, and uses userspace WireGuard over relay streams.

**I2P mode.** `i2p-datagram` uses I2P SAM for control and peer data transport, exchanges public I2P destinations through management, and keeps private destination keys local to the client profile.

**AnonBird UX.** The CLI command is `anonbird`, the dashboard uses anonymous-aware install flows, and release packages install into AnonBird paths such as `/etc/anonbird`, `/var/lib/anonbird`, `/var/log/anonbird` and `/var/run/anonbird`.

### Key features

| Anonymous transport | Management | Security | Operations | Platforms |
|---|---|---|---|---|
| ✓ Tor SOCKS5 control plane | ✓ Anonymous-aware dashboard | ✓ STUN/ICE/direct UDP kill-switch | ✓ Fork release images and packages | ✓ Linux |
| ✓ Tor relay data plane | ✓ Setup-key bootstrap | ✓ IP/location/serial redaction | ✓ Self-host scripts | ✓ macOS |
| ✓ Tor stream multipath | ✓ Internal DNS and ACLs | ✓ Debug/upload/geolite fail-closed defaults | ✓ Systemd units | ✓ Windows |
| ✓ I2P SAM STREAM control plane | ✓ Device approval support | ✓ Anonymous update checks disabled by default | ✓ Docker/Compose templates | ✓ Containers |
| ✓ I2P SAM DATAGRAM peer transport | ✓ Setup invite tokens | ✓ Runtime anonymous checks | ✓ Release hardening audit commands | ✓ FreeBSD package helper |

### Quickstart

AnonBird is self-hosted-first. The quickstart scripts expect a Linux VM with:

- A Linux VM with at least **1 CPU** and **2 GB** of memory.
- Docker with the Compose plugin.
- A DNS name pointing to the VM for clearnet bootstrap or reverse proxy testing.
- Tor and/or i2pd available on clients for anonymous transports.

```bash
export NETBIRD_DOMAIN=anonbird.example.com
curl -fsSL https://github.com/Cr0me1ve/netbird/releases/latest/download/getting-started.sh | bash
```

The `NETBIRD_*` environment names are still accepted in deployment scripts for compatibility with the inherited configuration contract. New generated artifacts use AnonBird images, commands and filesystem paths.

### Anonymous Client Examples

```bash
anonbird up \
  --management-url http://examplehiddenservice.onion \
  --setup-key "$SETUP_KEY" \
  --anonymous-mode \
  --anonymous-transport tor-relay-only \
  --tor-socks5 127.0.0.1:9050
```

```bash
anonbird up \
  --management-url http://example.b32.i2p \
  --setup-key "$SETUP_KEY" \
  --anonymous-mode \
  --anonymous-transport i2p-datagram \
  --i2p-sam 127.0.0.1:7656
```

Run the local safety audit any time:

```bash
anonbird debug anonymous-check
```

### Internals

- Every machine runs the [AnonBird agent](client/), which manages userspace WireGuard in anonymous mode.
- Every agent connects to the [Management Service](management/) and [Signal Service](signal/) through the configured anonymous transport.
- Tor mode uses relay WebSockets over SOCKS5 and disables direct candidate discovery.
- I2P mode uses SAM STREAM for control and SAM DATAGRAM for direct peer transport when possible.
- The [Relay Service](relay/) remains encrypted transport infrastructure, not a trust anchor.

### Acknowledgements

AnonBird builds on the NetBird codebase and open-source technologies like [WireGuard®](https://www.wireguard.com/), [Pion ICE](https://github.com/pion/ice), I2P SAM, Tor, and Rosenpass.

### Legal
This repository is licensed under the BSD-3-Clause license, which applies to all parts of the repository except for the directories management/, signal/ and relay/.
Those directories are licensed under the GNU Affero General Public License version 3.0 (AGPLv3). See the respective LICENSE files inside each directory.

_WireGuard_ and the _WireGuard_ logo are [registered trademarks](https://www.wireguard.com/trademark-policy/) of Jason A. Donenfeld.
 
