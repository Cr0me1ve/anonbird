# AnonBird v0.72.2 Release Report

Date: 2026-06-01

Verdict: production-ready for the tested open-source self-hosted anonymous mesh
scope. A test project can replace ordinary self-hosted NetBird with AnonBird
using the documented server and client migration commands, then run clients in
anonymous mode by default. This is not a promise of drop-in replacement for every
private NetBird deployment customization without reading the migration notes.

## Release Identity

- Main repo: `Cr0me1ve/anonbird`
- Tag: `v0.72.2`
- Commit: `d06b590cc9d0a08db724e2cdf967a735b8c467cd`
- Release URL: `https://github.com/Cr0me1ve/anonbird/releases/tag/v0.72.2`
- Dashboard repo: `Cr0me1ve/anonbird-dashboard`
- Dashboard tag: `v0.72.2`
- Dashboard commit: `4f85d96d43b43f40e0bcdb69af165814590665c4`

`v0.72.2` supersedes `v0.72.0` and `v0.72.1`. `v0.72.0` exposed an
RC-to-stable installer SemVer comparison bug. `v0.72.1` fixed that bug, but its
tag release gate was not fully green because the Windows installer amd64 job
depended on an unavailable external Mesa3D host. `v0.72.2` pins the Windows
Mesa3D source to a checksum-verified GitHub release and passed the full tag
release gate.

## Key Artifacts

| Artifact | SHA256 |
| --- | --- |
| `install.sh` | `8388aeac08121d3644306561072e4a58910cd8b118bf654da45bc1aa54b28e2d` |
| `getting-started.sh` | `b62f3f9227213016ba6640b02a945973a8db8dfd8dcf3d2f0910ef170559b15f` |
| `anonbird_0.72.2_checksums.txt` | `16d1adb9856bc6ca354f99f532083bc618daecbdbee23d225bea59efa29c15bd` |
| `anonbird_0.72.2_linux_amd64.tar.gz` | `ea8be967b734926cdfc8b3a1acfa2fdf53c303a3e781d563659f8ed7f518fca9` |
| `anonbird_0.72.2_linux_amd64.deb` | `b8e4930e9f958337cd252aee7c7ae5e752676d17bf021e8b6858d45a846fccef` |
| `anonbird_0.72.2_linux_amd64.rpm` | `48fbcd48ea2a8001f5a1abfef7fadb6575728cc1b24b8627080541434b2173a0` |
| `anonbird_0.72.2_windows_amd64.tar.gz` | `edec0e25e127bb5c5859dc5db941d211c24cdc1d79e794efc05510b167d1346b` |
| `anonbird-ui-windows_0.72.2_windows_amd64.tar.gz` | `682b2ecab3b4093cbd2d0aedce7903f91dbc12aec82bbdf1b4be1d315b90ddbd` |

`releases/latest/download/install.sh` and
`releases/latest/download/getting-started.sh` both redirect to `v0.72.2`.

## CI Matrix

Main branch commit `d06b590cc`:

- `Release`: success, run `26739034197`, `25m42s`
- `Linux`: success, run `26739034219`, `17m7s`
- `Windows`: success, run `26739034172`, `12m57s`
- `Darwin`: success, run `26739034170`, `9m42s`
- `FreeBSD`: success, run `26739034214`, `13m36s`
- `Mobile`: success, run `26739034208`, `5m28s`
- `Wasm`: success, run `26739034174`, `1m17s`
- `Test installation`: success, run `26739034210`, `44s`
- `Test Infrastructure files`: success, run `26739034195`, `4m6s`

Tag `v0.72.2` release gate:

- Run: `https://github.com/Cr0me1ve/anonbird/actions/runs/26740109076`
- `release`: success
- `release_ui`: success
- `release_ui_darwin`: success
- `Windows Installer / Build Test (arm64)`: success
- `Windows Installer / Build Test (amd64)`: success

Dashboard tag `v0.72.2`:

- Run: `https://github.com/Cr0me1ve/anonbird-dashboard/actions/runs/26740115841`
- `build_n_push`: success

## Container Images

Verified manifests:

- `ghcr.io/cr0me1ve/anonbird-server:0.72.2`: `linux/amd64`, `linux/arm64`
- `ghcr.io/cr0me1ve/anonbird-reverse-proxy:0.72.2`: `linux/amd64`, `linux/arm64`
- `ghcr.io/cr0me1ve/anonbird-dashboard:0.72.2`: `linux/amd64`, `linux/arm64`, `linux/arm/v7`
- `ghcr.io/cr0me1ve/anonbird-server:latest`: `linux/amd64`, `linux/arm64`
- `ghcr.io/cr0me1ve/anonbird-dashboard:latest`: `linux/amd64`, `linux/arm64`, `linux/arm/v7`

## Manual Release Tests

Testbed:

- Server/self-host smoke: `213.108.3.228`
- Client pair: `185.246.220.249`, `45.138.103.224`
- Additional release testbed servers tracked in the fork plan:
  `93.177.116.58`, `213.108.3.228`, `185.246.220.249`,
  `45.138.103.224`
- Manual Windows fallback build/test host: `gay--@192.168.0.100`

Latest self-host smoke from published assets:

- Command source: `https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh`
- Domain: `anonbird-v0722.213.108.3.228.sslip.io`
- `--preflight-only`: `AnonBird Docker image preflight passed`
- Full `--yes` install: stack started from published `latest` images
- Dashboard `/`: HTTP `200`
- OIDC discovery: HTTP `200`
- Unauthenticated `/api/users`: HTTP `401`
- `setup-key bootstrap`: succeeded; setup key redacted from this report
- Config confirmed: `stunPorts: []`, `disableAnonymousMetrics: true`,
  `disableGeoliteUpdate: true`, `disableVersionCheck: true`
- Compose/config grep confirmed no `3478/udp`
- Server log grep for upstream/version/geolocation leak patterns: empty
- Test stack and volumes removed after evidence capture

Published client update smoke:

- `185.246.220.249`: updated from `0.72.0-rc.7` to `0.72.2` using
  `releases/latest/download/install.sh --update`
- `45.138.103.224`: updated from `0.72.0-rc.7` to `0.72.2` using
  `releases/latest/download/install.sh --update`
- Both clients: `command -v anonbird` -> `/usr/bin/anonbird`
- Both clients: `anonbird.service` active, legacy `netbird.service` inactive
- Both clients: `/etc/anonbird/install.conf` records `package_manager=apt`
- Both clients: `anonbird debug anonymous-check` result `OK`
- Both clients: management, signal and relay over Tor; STUN, ICE, direct UDP
  and clearnet fallback disabled; published endpoints none; userspace
  WireGuard mode

Overlay connectivity after update:

- `185.246.220.249` (`100.79.32.188`) -> `45.138.103.224`
  (`100.79.20.213`): ping `4/4`, average `961.040 ms`
- `45.138.103.224` (`100.79.20.213`) -> `185.246.220.249`
  (`100.79.32.188`): ping `4/4`, average `1040.185 ms`
- Both sides report the tested peer as `Relayed`
- Relay server: `.onion`
- ICE candidate endpoints: `-/-`
- Post-update journal grep over the fresh window for upstream, version-check,
  STUN/ICE/NAT discovery, public testbed IP and shortcut leak patterns: empty

Windows Mesa3D release hardening:

- New source:
  `https://github.com/pal1000/mesa-dist-win/releases/download/26.1.1/mesa3d-26.1.1-release-msvc.7z`
- SHA256:
  `d5e90e9ae4d620313b61fbbf8e9a55761454e38b6501c39be6d93449c88780e1`
- Verified locally and on `gay--@192.168.0.100`
- Required files extracted: `x64/opengl32.dll`, `x64/libgallium_wgl.dll`
- Local PE import check confirmed `opengl32.dll` imports
  `libgallium_wgl.dll`

Migration and application-layer gates:

- Server-side migration from upstream NetBird `v0.64.6`, rollback and restore
  were completed on `93.177.116.58`; details are recorded in
  `anonbird_netbird_fork_plan.md`
- Two-client NetBird-to-AnonBird migration, rollback/reapply and anonymous
  rejoin were completed; details are recorded in `anonbird_netbird_fork_plan.md`
- Marton master/edge subscription flow was tested over AnonBird Tor relay-only
  overlay from published RC7 client artifacts; the stable `v0.72.2` update
  preserved the same tested overlay pair and anonymous transport properties

## Known Limitations

- Anonymous mode protects AnonBird peer connectivity from publishing real peer
  IPs when clients use the onion/I2P management, signal and relay endpoints. It
  does not claim anonymity against a global passive observer, compromised
  endpoints or host-level malware.
- Tor/I2P usage can still be visible to a local network provider unless the
  operator adds a separate pluggable-transport or censorship-circumvention
  layer.
- A clearnet admin dashboard is supported for operator convenience, but peer
  onboarding should use the onion/I2P service URL to preserve the "dashboard and
  management do not learn client public IPs through the AnonBird client path"
  property.
- Non-anonymous/clearnet modes remain possible for compatibility, but they are
  intentionally not the default and require explicit unsafe confirmation.
- Windows and macOS artifacts are built and packaged, but production code
  signing/notarization is opt-in and depends on operator-provided signing
  credentials. The release gate verified unsigned installer/package builds.
- Some stale peers from earlier test iterations remain in the test account as
  `Connecting`. The release verdict is based on the fresh tested peer pair and
  migration paths.

## Replacement Verdict

For a test project using ordinary self-hosted NetBird, replacing it with
AnonBird is realistic and supported through the documented migration flow:

1. Run the one-command self-host install or server migration command.
2. Generate or migrate setup keys.
3. Run client migration with anonymous rejoin.
4. Verify `anonbird debug anonymous-check`.
5. Verify application traffic over overlay IP/DNS.

The tested server, clients and Marton application flow did not require manual
code patches after the documented migration/update steps.
