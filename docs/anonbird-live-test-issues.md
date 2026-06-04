# AnonBird live test issues

Live window: 2026-06-03 to 2026-06-04.

Scope: reinstall and retest `anonbird.minet.space`, build binaries locally,
deploy them to weak test hosts, enroll clients with a NetBird-like management
URL plus setup key flow, and verify Tor/I2P anonymous peer operation.

## Fixed in this pass

1. Combined server anonymous endpoint override
   - Symptom: the published server advertised clearnet Signal and Relay URLs to
     anonymous clients.
   - Risk: clients either refuse to start because of the anonymous kill-switch,
     or a future regression could expose real client addresses.
   - Fix: allow `management` and `relay` sections in combined YAML and preserve
     explicit relay `exposedAddress`/`authSecret` instead of always deriving
     them from `server.exposedAddress`.

2. Installer I2P endpoint readiness accepted an empty endpoint
   - Symptom: `anonymous-endpoints.env` could be written with an empty
     `ANONBIRD_I2P_MANAGEMENT_ENDPOINT`.
   - Cause: the optional endpoint validator returned success for an empty
     value, and the I2P wait loop used it as a readiness check.
   - Fix: add a required endpoint validator for auto-generated endpoints and use
     it in Tor/I2P wait/final validation paths.

3. Anonymous management host DNS cache pre-resolution
   - Symptom: clients logged normal DNS lookups for `.onion` management hosts
     against the system resolver.
   - Risk: the real IP is not sent to peers, but the anonymous hostname can leak
     to local DNS infrastructure.
   - Fix: skip management DNS cache pre-resolution for `.onion` and `.i2p`
     hosts in both direct management URL population and server config domain
     updates. Anonymous transports resolve/dial through Tor SOCKS5 or I2P SAM.

4. Legacy NetBird process conflicts on migrated/test hosts
   - Symptom: old `netbird` kept `wt0`, UDP `51820`, and local DNS bindings,
     making AnonBird fail with `error creating tun device: invalid argument`.
   - Fix status: operationally fixed on the live hosts by stopping/disabling
     the old service. This still needs migration/install coverage if this path
     is expected during production re-enrollment.

5. Server I2P tunnel used HTTP tunnel mode
   - Symptom: I2P clients reached the `.b32.i2p` management endpoint
     intermittently, but gRPC failed with `http2: frame too large`.
   - Cause: the generated server I2P tunnel used `type = http`, which is not a
     transparent TCP tunnel for the management gRPC/h2c service.
   - Fix: render the I2P management tunnel as `type = server`, matching the raw
     TCP behavior used by the Tor hidden service forwarder.

6. Tor relay throughput was limited by single-flow channel selection
   - Symptom: default `iperf3` TCP tests through Tor relay stayed in the
     hundreds of Kbit/s even though multiple anonymous relay channels were
     opened.
   - Cause: relay multipath selected channels by inner plaintext flow before
     WireGuard encryption. That preserves packet order, but one TCP flow maps to
     one Tor stream, so a single `iperf3` run cannot use the extra channels.
   - Fix: add `NB_ANON_RELAY_MULTIPATH_STRATEGY=packet-burst`, which sends
     WireGuard data packets in small bursts across healthy relay channels so a
     single bulk flow can use more than one Tor stream.
   - Tuning: add `NB_ANON_RELAY_MULTIPATH_CHANNELS` with a safe cap of 16
     channels. The live tests used 8 channels.

7. Closed Tor relay stream could close the whole peer dataplane
   - Symptom: a single failed secondary Tor relay channel could bubble a write
     error up to the WireGuard UDP proxy.
   - Risk: `WGUDPProxy.proxyToRemote` exits on any remote write error, so one
     dead Tor stream can close the entire relayed peer dataplane even when other
     multipath channels are still healthy.
   - Fix: `relayMultipathConn.Write` retries zero-byte failed writes through
     another healthy channel, marks the failed channel unhealthy, and starts a
     background channel reopen. Reopen does not block normal writes, so healthy
     channels continue carrying traffic.
   - Race fix: stale read loops can no longer mark a newly reopened channel
     unhealthy for an error that belonged to the old connection instance.

8. WebSocket relay client `Write` returned the wrong byte count
   - Symptom: successful WebSocket writes returned the compressed frame length,
     not `len(b)`.
   - Risk: callers treating the object as `net.Conn` could mis-handle partial
     write semantics.
   - Fix: return `len(b)` after a successful full message write.

9. Relayed client connection deadline methods panicked
   - Symptom: `shared/relay/client.Conn` implemented `SetDeadline`,
     `SetReadDeadline`, and `SetWriteDeadline` with `panic`.
   - Risk: any caller treating the relay connection as a normal `net.Conn`
     could crash the process.
   - Fix: these methods now return a stable unsupported-deadline error.

10. Managed Tor runtime failed as root on Debian/Ubuntu hosts
    - Symptom: Tor failed to open its log/data files under
      `/var/lib/anonbird/tor` when started by the daemon.
    - Fix: create/chown the data directory and run Tor with a packaged Tor user
      when one exists (`debian-tor`/`tor`). The managed torrc logs to stdout and
      the daemon redirects stdout/stderr to `tor.log`.

11. Managed I2P runtime failed on packaged Linux installs
    - Symptom: packaged `i2pd` failed with permission errors under
      `/var/lib/i2pd/anonbird/destinations`.
    - Fix: create the expected managed I2P directory layout, chown it for the
      packaged `i2pd` user, and start managed `i2pd` under that uid/gid when the
      daemon runs as root.

12. Managed I2P pidfile was blocked by AppArmor
    - Symptom: Ubuntu AppArmor denied file locking for
      `/var/lib/i2pd/anonbird/i2pd.pid`.
    - Fix: use `/run/i2pd/i2pd.pid` for packaged root/Linux managed `i2pd`,
      create/chown `/run/i2pd`, and remove stale pidfiles before start.

13. Dependency installer got stuck after apt update failures
    - Symptom: a transient `apt-get update` failure prevented dependency
      install even when the package metadata was already usable.
    - Fix: retry package install without update when the update/install path
      fails.

## Live deployment state

- Local Linux amd64 client artifact:
  `/tmp/anonbird-build/anonbird-linux-amd64`.
- Deployed client version: `development-local-i2p-pidfile-reopen`.
- Deployed SHA256:
  `3762c1b4707b92a0f87a374ac6dc57aab8347ab43396368343464509e12dacbe`.
- Tor management URL:
  `http://4mvuprkev3xqtzeb2ha4gskcjut7maktl6dic6u22cqktzswah5ugrad.onion:80`.
- I2P management URL:
  `http://gbgahblqb6bzuh64fbeyopxudfkczbr6sta7ausufezjts74faka.b32.i2p`.
- Server config was restored to Tor after I2P smoke testing.
- Dashboard/server health after restore:
  - `https://anonbird.minet.space` returned HTTP 200.
  - Docker containers were running for `anonbird-server`, `anonbird-dashboard`,
    `anonbird-traefik`, `anonbird-tor`, and `anonbird-i2pd`.
  - `/opt/anonbird/config.yaml` advertised onion Signal and Relay endpoints for
    anonymous clients.

Current Tor peer map after re-enrollment:

| Host | AnonBird IP | FQDN |
| --- | --- | --- |
| `185.246.220.249` | `100.68.230.18` | `anonbird-dff311e007b1.anonbird.local` |
| `45.138.103.224` | `100.68.209.114` | `anonbird-66bf4c83b717.anonbird.local` |
| `213.108.2.95` | `100.68.140.77` | `anonbird-2ea94005d484.anonbird.local` |

`83.171.225.182` was not usable for this pass: SSH eventually answered once,
but upload of the compressed client artifact stalled for several minutes and
had to be killed. No client was installed there.

## Tor retest results on final build

All three installed clients reported:

- daemon and CLI version `development-local-i2p-pidfile-reopen`;
- matching binary SHA256;
- Management and Signal connected to the onion URL;
- relay available at the onion `rel://` URL;
- active peer connections as `Connection type: Relayed`;
- `ICE candidate endpoints (Local/Remote): -/-`.

Ping, `5 x ICMP`, final corrected peer map:

| Direction | Result |
| --- | --- |
| `185 -> 45` | 5/5, 0% loss, avg 1213 ms |
| `45 -> 185` | 5/5, 0% loss, avg 1181 ms |
| `45 -> 213` | 5/5, 0% loss, avg 1252 ms |
| `213 -> 45` | 5/5, 0% loss, avg 1226 ms |
| `185 -> 213` | first corrected run 0/5, repeat 4/5, avg 1281 ms |
| `213 -> 185` | first corrected run 2/5, repeat 5/5, avg 1332 ms |

`iperf3 -P 4 -t 8`, final build:

| Direction | Result |
| --- | --- |
| `185 -> 45` | sender 2.36 Mbit/s, receiver 1.36 Mbit/s, peak interval 6.28 Mbit/s |
| `45 -> 185` | sender 0.52 Mbit/s, receiver 0.11 Mbit/s |
| `185 -> 213` | sender 0.92 Mbit/s, receiver 0.17 Mbit/s |
| `213 -> 185` | first run `Broken pipe`; retry completed but effectively stalled, receiver 17.4 Kbit/s |
| `45 -> 213` | first run control socket closed; after receiver restart: sender 1.44 Mbit/s, receiver 0.63 Mbit/s |
| `213 -> 45` | first run control socket closed; after receiver restart: sender 2.10 Mbit/s, receiver 0.86 Mbit/s |

Conclusion: the stream-reopen fix prevents a closed Tor relay stream from
permanently killing the peer connection, and control/status stays alive under
load. It does not yet make Tor throughput reliably hit 5-10 Mbit/s. The next
speed bottleneck is path quality and TCP behavior over high-latency Tor relay
multipath: there are still long zero-throughput intervals, retransmits, and
occasional `iperf3` control-channel closes.

## I2P retest results

What worked:

- The server has a generated I2P endpoint and an `i2pd` 2.60 sidecar.
- A client on `185.246.220.249` could connect to I2P management/signal/relay
  after switching the server config to the I2P URL.
- The client status showed the `.b32.i2p` management URL and I2P relay URL.

What did not work:

- Full I2P mesh throughput was not measured successfully.
- On `213.108.2.95`, the distro package `i2pd v2.49.0` started after the data
  directory and pidfile fixes, but later crashed with a kernel general
  protection fault in the `i2pd` process.
- In I2P mode, `anonbird up` can return `DeadlineExceeded` while the daemon
  finishes registration/connect later. That is not NetBird-like UX yet.

Conclusion: Tor is currently the only tested out-of-box URL+setup-key path.
I2P has a working server side and partial client smoke success, but it is not
reliable out of the box on hosts where apt installs old `i2pd` 2.49. A proper
fix should provide or require a modern `i2pd` runtime, preferably 2.60 or newer,
instead of depending blindly on the distro package.

## Privacy checks

- Client `status --detail` for active peers showed `Connection type: Relayed`,
  no ICE local/remote endpoints, and only the onion relay address.
- `ss -Htnp` on `185`, `45`, and `213` showed no AnonBird TCP connections to
  the real test host IPv4 addresses.
- Server logs from the test window did not contain the real test host IPv4
  addresses.

This verifies that active Tor peers did not learn each other's real IPv4/IPv6
addresses during the retest. The check does not claim global anonymity against
external observers outside the tested software path.

## Remaining issues

1. Tor throughput target not met reliably
   - Goal: sustained 5-10 Mbit/s on ordinary Tor paths.
   - Current: short sender-side peaks can exceed 5 Mbit/s, but receiver-side
     sustained throughput is usually below 1.5 Mbit/s and some directions
     degrade to Kbit/s.
   - Next code direction: avoid using poor Tor channels for bulk writes by
     adding per-channel health scoring/latency/loss EWMA, cooldowns, and a
     bulk strategy that stripes only over channels that recently delivered ACKed
     traffic. Packet-burst alone is too optimistic.

## 2026-06-04 Tor speed follow-up

Research notes from primary project docs:

- Tor SOCKS supports application-provided stream isolation tokens through
  SOCKS5 username/password values and the newer `<torS0X>` extension format.
  The Tor path spec treats these as isolation properties that can prevent
  streams from sharing circuits.
- `KeepAliveSOCKSAuth`/`KeepAliveIsolateSOCKSAuth` can keep circuits with
  isolated SOCKS auth alive longer, but using isolation more aggressively also
  consumes more Tor circuit resources.
- Hysteria2 is a QUIC-based TCP/UDP proxy designed for speed on lossy networks.
  It is a useful candidate for a separate fast transport mode, but it is not a
  Tor transport replacement: a Hysteria server sees the client's real source IP
  unless another anonymity layer is used.

Code changes made in this follow-up:

- Relay WebSocket SOCKS5 dialer can now send SOCKS5 username/password auth.
- Added opt-in Tor SOCKS isolation for relay channels:
  `NB_ANON_RELAY_TOR_SOCKS_ISOLATION=true`.
- Managed Tor now writes `SocksPort ... IsolateSOCKSAuth
  KeepAliveIsolateSOCKSAuth`.
- Packet-burst size is configurable:
  `NB_ANON_RELAY_MULTIPATH_PACKET_BURST_SIZE`, default 64, cap 4096.

Live experiments:

| Config | Result |
| --- | --- |
| isolation on, `packet-burst`, 4 channels, burst 8 | ping improved on `185/45` to roughly 0.57-0.64 s with 0% loss in one run, but `iperf` was still poor and unstable |
| isolation on, `flow-affine`, 4 channels | peer status stayed connected, but ICMP to `185/45` failed in the retest |
| isolation on, `packet-burst`, 4 channels, burst 64 | ICMP stayed alive, but `iperf3 -P 4` `185 -> 45` effectively stalled at 0 useful throughput |
| isolation off, `packet-burst`, 8 channels, burst 8 | restored the previous behavior class but `185/45` still showed high loss after repeated Tor restarts |
| isolation off, `packet-burst`, 4 channels, burst 8 | live was left here to reduce pressure on Tor; `185/45` was still lossy, but less aggressive than 8 channels |

Conclusion: the simple internet-derived Tor optimization, per-channel SOCKS
isolation, is not a safe default for this relay dataplane. It can reduce ICMP
latency in short windows, but combining independent Tor circuits with packet
striping makes TCP reordering/tails worse. The feature is kept as an opt-in
knob for future tests, not enabled by default.

Next viable speed direction:

1. Stop trying to make one TCP stream fast by striping packets over independent
   Tor circuits. That produces reordering and stalls.
2. Add relay-channel telemetry: write duration, read activity, error rate,
   cooldowns, and per-channel score.
3. Use channel scoring to select only the best live Tor stream and fail over
   quickly, rather than striping blindly.
4. For genuinely high throughput, add a separate fast transport such as
   Hysteria2/QUIC with an explicit privacy label. It can hide peer IPs from
   each other if all peers connect only to the relay, but the relay/Hysteria
   server will know client source IPs unless combined with Tor/I2P.

2. I2P out-of-box runtime is not reliable
   - Goal: user provides URL plus setup key and I2P works like NetBird.
   - Current: server/one-client smoke works, but apt `i2pd 2.49` crashed on one
     host and no I2P speed number can be honestly reported.
   - Next code direction: ship/manage a known-good `i2pd` runtime or enforce a
     minimum supported version with a clear installer path.

3. I2P CLI readiness is not NetBird-like yet
   - Goal: `anonbird up --management-url <i2p-url> --setup-key <key>` returns
     success only when the daemon is actually usable, or gives a clear pending
     state.
   - Current: command can return `DeadlineExceeded` even when the daemon later
     connects.

4. Fourth test host unavailable
   - `83.171.225.182` needs separate infrastructure repair before it can be
     part of the live matrix.

## Local verification

Run after the code changes:

```sh
go test ./client/internal/anonymous ./client/internal/peer ./shared/relay/client ./shared/relay/client/dialer/ws ./client/internal/dns/config ./client/internal/dns/mgmt ./combined/cmd ./relay/server ./shared/relay/messages ./shared/anonymous/i2psam -count=1
```

Expected status: pass.

Actual status on 2026-06-04: pass.

## 2026-06-05 Tor packet packing follow-up

User correction: the desired direction is not simply "use Hysteria2 instead of
Tor", but to replace the current WireGuard-over-WebSocket-over-Tor shape with a
packet-packing transport in the spirit of Hysteria2-over-WebSocket-over-Tor.

Protocol constraint:

- Stock Hysteria2 is QUIC/UDP based and its speed advantage depends on QUIC
  datagrams, QUIC congestion control, and UDP behavior. WebSocket-over-Tor is
  a TCP stream inside Tor, so embedding the stock Hysteria2 protocol there would
  remove the transport property that makes Hysteria2 fast. This needs either a
  custom packet-packing layer over the existing relay streams, or a separate
  Hysteria2/QUIC transport with an explicit different privacy model.

Code change made in this follow-up:

- Added opt-in relay packet batching:
  `NB_ANON_RELAY_MULTIPATH_BATCH=true`.
- WireGuard datagrams can now be grouped into a compact batch frame before
  writing to the Tor/WebSocket relay channel. Receivers decode batch frames and
  still accept old raw single-packet frames, so rollout is backwards compatible
  at the relay payload layer.
- The batcher flushes on a small packet count, byte limit, or short timer. This
  reduces write syscall/WebSocket frame churn without blocking reads on a slow
  or reopened Tor channel.
- Existing unhealthy-channel behavior still applies: if a write fails, the
  channel is marked unhealthy and reopened while other channels remain usable.

Local verification after this change:

```sh
go test ./client/internal/peer -run 'TestRelayMultipathConn(Batches|ReadsBatched|PacketBurst)' -count=1
go test ./client/internal/peer ./shared/relay/client ./shared/relay/client/dialer/ws -count=1
go test ./client/internal/anonymous ./client/internal/peer ./shared/relay/client ./shared/relay/client/dialer/ws ./client/internal/dns/config ./client/internal/dns/mgmt ./combined/cmd ./relay/server ./shared/relay/messages ./shared/anonymous/i2psam -count=1
```

Actual status on 2026-06-05: pass.

Live deployment:

- Locally built Linux amd64 binary:
  `/tmp/anonbird-build/anonbird-linux-amd64`.
- Binary SHA256:
  `4af04ed76687b65aeadaa7c18fd9973f2aa5509476d5914588e1ac2a7feff560`.
- Gzip SHA256:
  `4898a617c990bb21f6863de3012a4adb3d3ccb683015e0b3552cded9fd7fe5b5`.
- Deployed version on `185.246.220.249`, `45.138.103.224`,
  `213.108.2.95`: `development-local-tor-batch`.
- Runtime env on all three peers:
  `NB_ANON_RELAY_MULTIPATH_STRATEGY=packet-burst`,
  `NB_ANON_RELAY_MULTIPATH_CHANNELS=4`,
  `NB_ANON_RELAY_MULTIPATH_PACKET_BURST_SIZE=8`,
  `NB_ANON_RELAY_TOR_SOCKS_ISOLATION=false`,
  `NB_ANON_RELAY_MULTIPATH_BATCH=true`.

Live Tor ping results with batching enabled:

| Direction | Result |
| --- | --- |
| `185 -> 45` | 10 sent, 9 received, 10% loss, avg 1516.547 ms, max 3140.668 ms |
| `45 -> 185` | 10/10 received, 0% loss, avg 1449.480 ms, max 3015.175 ms |
| `185 -> 213` | 10 sent, 7 received, 30% loss, avg 1056.398 ms |
| `213 -> 185` | 10 sent, 6 received, 40% loss, avg 1344.163 ms, max 3095.636 ms |

Live Tor iperf3 results with batching enabled:

| Direction | Result |
| --- | --- |
| `185 -> 45`, `-P 4 -t 10` | sender 2.00 MBytes / 1.68 Mbit/s, receiver 1.38 MBytes / 796 Kbit/s, 19 retransmits, peak sender interval 6.29 Mbit/s |
| `45 -> 185`, `-P 4 -t 10` | sender 6.75 MBytes / 5.66 Mbit/s, receiver 5.00 MBytes / 3.28 Mbit/s, 8 retransmits, peak sender intervals 12.6 and 23.1 Mbit/s |

Privacy check:

- `ss -Htnp` on `185`, `45`, and `213` produced no AnonBird TCP connections
  to the real test host IPv4 addresses during the final check.
- `anonbird status --detail` still showed management/signal/relay over the
  onion URL, version `development-local-tor-batch`, and no direct ICE endpoint
  pairs for the checked peer detail tail.

Conclusion:

- Packet batching is compatible and improves the best observed Tor bulk
  direction: `45 -> 185` reached 5.66 Mbit/s sender-side and 3.28 Mbit/s
  receiver-side over 10 seconds, with short intervals above 10 Mbit/s.
- The target of stable sustained 5-10 Mbit/s is still not met. `185 -> 45`
  remains below 1 Mbit/s receiver-side, and `213` paths still have high loss.
- The next correct step is not a bigger blind batch. It is per-channel
  telemetry and congestion control: read/write latency, delivered bytes,
  write-error rate, recent activity, cooldown, and channel scoring so bulk
  traffic uses healthy Tor streams while reopen happens in the background.
- A true Hysteria2/QUIC path should be added as a separate fast transport mode,
  not hidden inside the Tor/WebSocket mode. It can be useful when peer IPs must
  be hidden from each other by using a relay, but the relay transport endpoint
  will still see client source IPs unless another anonymity layer is used.

## 2026-06-05 Tor channel scoring follow-up

Problem found after packet batching:

- Packet batching reduced frame/write overhead, but packet-burst still treated
  all open Tor streams as equally useful until they hard-failed. A Tor stream
  can be technically open while writes are slow or its batch queue is backed up.
  Blindly continuing to feed that stream causes TCP-over-WireGuard stalls and
  large tail latency.

Code change made in this follow-up:

- Added per-channel telemetry for anonymous relay multipath:
  last read time, last write time, write latency EWMA, write failure count, and
  temporary cooldown deadline.
- Added channel scoring/cooldown controls:
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_SCORING`,
  `NB_ANON_RELAY_MULTIPATH_SLOW_WRITE_MS`,
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_COOLDOWN_MS`,
  `NB_ANON_RELAY_MULTIPATH_READ_IDLE_MS`.
- Slow `conn.Write` calls now temporarily cool down the channel.
- Batch queue overflow is treated as congestion, not as a hard stream failure:
  the write attempts another channel instead of blocking behind the full queue
  or forcing a Tor stream reopen.
- Reopened channels reset their telemetry so a new Tor stream is not penalized
  by the previous stream's history.
- Packet-burst keeps a stable channel order inside a burst and uses cooldown
  only to remove bad streams from the candidate list. This avoids reordering
  every packet when scores change after individual writes.

Local verification after this change:

```sh
go test ./client/internal/peer -count=1 -timeout=90s
go test ./client/internal/anonymous ./client/internal/peer ./shared/relay/client ./shared/relay/client/dialer/ws ./client/internal/dns/config ./client/internal/dns/mgmt ./combined/cmd ./relay/server ./shared/relay/messages ./shared/anonymous/i2psam -count=1 -timeout=180s
```

Actual status on 2026-06-05: pass.

Live deployment:

- Locally built Linux amd64 binary:
  `/tmp/anonbird-build/anonbird-linux-amd64`.
- Binary SHA256:
  `bf40abf4a05d53f9d56e78904c078b26a7dd24abe58a7d4542f887054ca578fa`.
- Gzip SHA256:
  `204af922d95670a94afc28ce526f12b026b6a34560fb9b96c0d23586dcc17e1e`.
- Deployed version on `185.246.220.249`, `45.138.103.224`,
  `213.108.2.95`: `development-local-tor-score`.
- Runtime env on all three peers:
  `NB_ANON_RELAY_MULTIPATH_STRATEGY=packet-burst`,
  `NB_ANON_RELAY_MULTIPATH_CHANNELS=4`,
  `NB_ANON_RELAY_MULTIPATH_PACKET_BURST_SIZE=8`,
  `NB_ANON_RELAY_TOR_SOCKS_ISOLATION=false`,
  `NB_ANON_RELAY_MULTIPATH_BATCH=true`,
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_SCORING=true`,
  `NB_ANON_RELAY_MULTIPATH_SLOW_WRITE_MS=250`,
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_COOLDOWN_MS=3000`,
  `NB_ANON_RELAY_MULTIPATH_READ_IDLE_MS=20000`.

Live Tor ping results with channel scoring enabled:

| Direction | Result |
| --- | --- |
| `185 -> 45` | 10 sent, 6 received, 40% loss, avg 934.753 ms, max 1017.802 ms |
| `45 -> 185` | 10 sent, 7 received, 30% loss, avg 1033.324 ms, max 1382.655 ms |
| `185 -> 213` | 10/10 received, 0% loss, avg 1202.502 ms, max 1467.454 ms |
| `213 -> 185` | 10 sent, 9 received, 10% loss, avg 1218.754 ms, max 1346.691 ms |

Live Tor iperf3 results with channel scoring enabled:

| Direction | Result |
| --- | --- |
| `185 -> 45`, `-P 4 -t 10` | sender 5.25 MBytes / 4.40 Mbit/s, receiver 3.65 MBytes / 2.48 Mbit/s, 7 retransmits, peak interval 10.5 Mbit/s |
| `45 -> 185`, `-P 4 -t 10` | TCP connected, but bulk stalled twice; 0 bytes transferred before `timeout 35s`, 9-14 retransmits in interrupted summaries |
| `185 -> 213`, `-P 4 -t 10` | sender 4.50 MBytes / 3.77 Mbit/s, receiver 3.50 MBytes / 2.54 Mbit/s, 7 retransmits, peak intervals 7.33-7.35 Mbit/s |
| `213 -> 185`, `-P 4 -t 10` | sender 2.62 MBytes / 2.20 Mbit/s, receiver 1.75 MBytes / 1.33 Mbit/s, 25 retransmits |

Privacy check:

- `ss -Htnp` on `185`, `45`, and `213` produced no AnonBird TCP connections
  to the real test host IPv4 addresses during the final check.
- Status still reported management, signal, and relay over the onion URL and
  version `development-local-tor-score`.

Conclusion:

- Channel scoring/cooldown is a real improvement for some paths. It roughly
  tripled `185 -> 45` receiver-side throughput versus the previous batching-only
  run and made `185 <-> 213` usable for bulk where ping was previously very
  lossy.
- It is not enough for stable 5-10 Mbit/s. `45 -> 185` exposed a stricter
  failure mode: ICMP works and TCP connects, but TCP bulk can stall at 0 bytes.
- The next Tor speed step should be pacing plus in-flight accounting at the
  relay multipath layer. The current writer can still enqueue too much data
  into a Tor/WebSocket stream before the code knows whether that stream is
  delivering useful bytes. The fix should limit per-channel in-flight bytes,
  prefer channels that have recent read activity while bulk is active, and make
  batch flush size adaptive instead of static.

## 2026-06-05 Tor pacing and in-flight follow-up

Problem found after channel scoring:

- Channel scoring only reacts after a stream has already become slow or failed.
  With batching enabled, the writer could still enqueue too much data into one
  Tor/WebSocket stream before the code had a useful signal that the stream was
  not draining.
- A stale read error from an old stream could also mark the replacement stream
  as cooling after a successful reopen. That made the hinted reopened channel
  lose to an unrelated channel in the next flow-affine write.

Code change made in this follow-up:

- Added per-channel pending/active byte accounting.
- Added per-channel max in-flight knob:
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_MAX_INFLIGHT_BYTES`.
- Added per-channel pacing knob:
  `NB_ANON_RELAY_MULTIPATH_PACING_MS`.
- Batch enqueue now reserves pending bytes and returns congestion before the
  queue can grow without bounds.
- Batch flush releases pending bytes, accounts active write bytes while
  `conn.Write` is in progress, and uses a per-channel pacing delay between
  flushes.
- Batch limits are adaptive: high pending bytes, slow write EWMA, or stale read
  activity reduce batch packet/byte limits.
- `PACING_MS=0` and `CHANNEL_MAX_INFLIGHT_BYTES=0` now disable those two
  mechanisms explicitly for A/B testing.
- Fixed stale failure accounting: `markChannelUnhealthy` now records a failure
  only after confirming that the failed connection is still the current channel
  connection.

Local verification after this change:

```sh
go test ./client/internal/peer -count=1 -timeout=90s
go test ./client/internal/anonymous ./client/internal/peer ./shared/relay/client ./shared/relay/client/dialer/ws ./client/internal/dns/config ./client/internal/dns/mgmt ./combined/cmd ./relay/server ./shared/relay/messages ./shared/anonymous/i2psam -count=1 -timeout=180s
git diff --check
```

Actual status on 2026-06-05: pass.

Live deployment:

- Locally built Linux amd64 binary:
  `/tmp/anonbird-build/anonbird-linux-amd64`.
- Binary SHA256:
  `02cb74ca4ad3c8d762f786f16159ab7dc37e179d71757086093863d62529f57b`.
- Gzip SHA256:
  `79a89277f7ae1efbffbc6e7b28cffa567453b167fb0fab36e6b2838ff463b73d`.
- Deployed version on `185.246.220.249`, `45.138.103.224`,
  `213.108.2.95`: `development-local-tor-pacing2`.
- Runtime env on all three peers:
  `NB_ANON_RELAY_MULTIPATH_STRATEGY=packet-burst`,
  `NB_ANON_RELAY_MULTIPATH_CHANNELS=4`,
  `NB_ANON_RELAY_MULTIPATH_PACKET_BURST_SIZE=8`,
  `NB_ANON_RELAY_TOR_SOCKS_ISOLATION=false`,
  `NB_ANON_RELAY_MULTIPATH_BATCH=true`,
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_SCORING=true`,
  `NB_ANON_RELAY_MULTIPATH_SLOW_WRITE_MS=250`,
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_COOLDOWN_MS=3000`,
  `NB_ANON_RELAY_MULTIPATH_READ_IDLE_MS=20000`,
  `NB_ANON_RELAY_MULTIPATH_PACING_MS=2`,
  `NB_ANON_RELAY_MULTIPATH_CHANNEL_MAX_INFLIGHT_BYTES=131072`.

Live Tor results with pacing profile `2 ms / 128 KiB`:

| Direction | Result |
| --- | --- |
| `185 -> 45` ping | 8 sent, 7 received, 12.5% loss, avg 1638.918 ms, max 3405.137 ms |
| `45 -> 185` ping | 8/8 received, 0% loss, avg 1467.893 ms, max 3040.947 ms |
| `185 -> 213` ping after warmup | 8 sent, 5 received, 37.5% loss, avg 1257.685 ms |
| `213 -> 185` ping after warmup | 8 sent, 5 received, 37.5% loss, avg 1408.105 ms |
| `213 -> 45` ping after warmup | 8 sent, 6 received, 25% loss, avg 3402.750 ms |
| `185 -> 45`, `iperf3 -P 4 -t 10` | sender 3.12 MBytes / 2.62 Mbit/s, receiver 2.00 MBytes / 1.52 Mbit/s, 12 retransmits, peak interval 9.44 Mbit/s |
| `45 -> 185`, `iperf3 -P 4 -t 10` | sender 5.00 MBytes / 4.19 Mbit/s, receiver 2.75 MBytes / 2.00 Mbit/s, 93 retransmits, peak interval 12.6 Mbit/s |

Rejected tuning attempt:

- Tried a softer profile, `PACING_MS=1` and
  `CHANNEL_MAX_INFLIGHT_BYTES=262144`.
- `45 -> 185` ICMP looked good in one short run: 6/6 received, avg 872.457 ms.
- But `185 -> 45` ICMP had 100% loss in the same window, and `45 -> 185`
  `iperf3` timed out with 0 bytes. The live env was reverted to `2 ms /
  128 KiB`.

Privacy check:

- Final `ss -Htnp` checks on `185`, `45`, and `213` produced no AnonBird TCP
  connections to the real test host IPv4 addresses.
- Final status checks reported management, signal, and relay over the onion URL
  and version `development-local-tor-pacing2`.

Conclusion:

- Pacing/in-flight accounting is useful because it eliminated the repeated
  `45 -> 185` 0-byte bulk stall under the `2 ms / 128 KiB` profile.
- It did not reach stable 5-10 Mbit/s. The best sustained receiver-side result
  in this step was 2.00 Mbit/s, and the opposite direction regressed versus the
  scoring-only build.
- The next code direction should expose per-channel telemetry in logs/status
  before adding more control logic. Without seeing pending bytes, active write
  bytes, write EWMA, cooldown, pacing wait, and selected channel IDs from live
  nodes, further tuning is mostly guesswork.
