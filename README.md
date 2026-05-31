
<div align="center">
  <p align="center">
    <img width="234" src="docs/media/logo-full.png" alt="AnonBird logo"/>
  </p>
  <p align="center">
    <a href="https://github.com/Cr0me1ve/anonbird/blob/main/LICENSE">
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

### One-command self-host quickstart

AnonBird is self-hosted-first. The recommended open-source quickstart starts a
single-host deployment with the dashboard, embedded IdP, management, signal and
relay combined server, and Traefik TLS routing.

- A Linux VM with at least **1 CPU** and **2 GB** of memory.
- Docker with the Compose plugin.
- A DNS name pointing to the VM.
- Open inbound `80/tcp` and `443/tcp`.
- Tor and/or i2pd available on clients for anonymous transports.

```bash
curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh \
  | bash -s -- --domain anonbird.your-domain.com --email admin@your-domain.com --yes
```

This renders `docker-compose.yml`, `dashboard.env` and `config.yaml`, then starts
the stack. When it finishes, open:

```text
https://anonbird.your-domain.com
```

The one-command installer uses the built-in Traefik mode by default and checks
that the required AnonBird Docker images are available before it starts the
stack. For a dry configuration render without starting containers:

```bash
curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh \
  | bash -s -- --domain anonbird.your-domain.com --email admin@your-domain.com --yes --render-only
```

To check release image availability without writing files or starting
containers:

```bash
curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh \
  | bash -s -- --domain anonbird.your-domain.com --email admin@your-domain.com --yes --preflight-only
```

Release-candidate and private registry tests can override images without
editing the script:

```bash
export ANONBIRD_DASHBOARD_IMAGE=registry.example.com/anonbird-dashboard:rc
export ANONBIRD_SERVER_IMAGE=registry.example.com/anonbird-server:rc
export ANONBIRD_PROXY_IMAGE=registry.example.com/anonbird-reverse-proxy:rc

curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh \
  | bash -s -- --domain anonbird.your-domain.com --email admin@your-domain.com --yes
```

The `NETBIRD_*` environment names are still accepted in deployment scripts for
compatibility with the inherited configuration contract. New generated artifacts
use AnonBird images, commands and filesystem paths.

### Linux client install

The release installer places the `anonbird` command in `PATH`, installs
`anonbird.service`, and uses `/etc/anonbird`, `/var/lib/anonbird`,
`/var/log/anonbird` and `/var/run/anonbird.sock`.

```bash
curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/install.sh \
  | sudo bash -s --
```

For migration dry-runs where old scripts still call `netbird`, add a temporary
compatibility symlink without making it the canonical command:

```bash
curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/install.sh \
  | sudo bash -s -- --compat-symlink --no-start
```

### Dashboard and anonymous peer URLs

The admin dashboard can be exposed on clearnet, a private network, or an onion
service. Anonymous peer privacy depends on the management/signal/relay URL used
by clients, not on where the administrator opens the dashboard.

Common split deployment:

```text
Admin browser:
  https://admin.example.com

AnonBird peers:
  http://managementxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.onion
```

Set the dashboard runtime configuration so browser API calls use the admin API
origin, while generated peer setup commands use the onion/I2P management origin:

```text
NETBIRD_MGMT_API_ENDPOINT=https://admin.example.com
NETBIRD_MGMT_GRPC_API_ENDPOINT=http://managementxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.onion
```

With that split, the administrator's browser can use clearnet, while peers still
join through Tor/I2P and do not publish real endpoint candidates.

### Anonymous Client Examples

Tor relay-only:

```bash
anonbird up \
  --management-url http://examplehiddenservice.onion \
  --setup-key "$SETUP_KEY" \
  --anonymous-transport tor-relay-only \
  --tor-socks5 127.0.0.1:9050
```

I2P datagram:

```bash
anonbird up \
  --management-url http://example.b32.i2p \
  --setup-key "$SETUP_KEY" \
  --anonymous-transport i2p-datagram \
  --i2p-sam 127.0.0.1:7656
```

Anonymous mode is enabled by default for new CLI connections. Non-anonymous
clearnet mode is intentionally hard to invoke: it prints a real-IP leak warning
and requires an explicit override.

```bash
anonbird up \
  --no-anonymous-mode \
  --allow-unsafe-clearnet \
  --yes-i-understand-this-may-leak-my-ip
```

Run the local safety audit any time:

```bash
anonbird debug anonymous-check
```

Expected anonymous output includes:

```text
Anonymous mode: enabled
STUN: disabled
ICE: disabled
Direct UDP: disabled
Clearnet fallback: disabled
Published endpoints: none
Result: OK
```

### Migration From NetBird

Migration defaults to dry-run mode and prints the exact file/service actions
before changing anything.

Client migration:

```bash
anonbird migrate client --dry-run
sudo anonbird migrate client --apply
```

For a clean anonymous re-enrollment during client migration:

```bash
sudo anonbird migrate client --apply --rejoin "anonbird://join?server=http%3A%2F%2Fexample.onion&setup_key=..."
```

Self-hosted server migration uses the packaged AnonBird migration script for the
legacy Docker Compose stack:

```bash
anonbird migrate server --install-dir /opt/netbird --dry-run
sudo anonbird migrate server --install-dir /opt/netbird --apply --yes
```

Rollback for client filesystem migration:

```bash
sudo anonbird migrate rollback --backup-dir /var/backups/anonbird/migration-YYYYMMDD-HHMMSS --apply
```

### Release-readiness status

The current branch contains a working anonymous MVP plus post-MVP production
hardening tasks. Before a public production release, run the release-readiness
plan in `anonbird_netbird_fork_plan.md`: release artifact install, migration from
ordinary NetBird, rollback, Tor/I2P remote smoke, a real application test over
the overlay, and a final leak sweep.

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
 
