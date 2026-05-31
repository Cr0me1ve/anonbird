# AnonBird anonymous mode leak map

Дата анализа: 2026-05-31

Этот документ фиксирует места NetBird, где в обычном режиме появляются реальные IP, endpoint candidates, STUN/ICE или clearnet fallback. Для AnonBird anonymous mode каждое место должно быть либо отключено, либо проходить через Tor/I2P transport с runtime validation.

## Security invariant

В anonymous mode клиент не должен:

- подключаться к management, signal или relay по clearnet hostname/IP;
- собирать или отправлять STUN/ICE candidates;
- публиковать WireGuard `ip:port`, public endpoint, LAN address или NAT mapping;
- сохранять real source IP в management peer metadata/location/events;
- показывать real IP, endpoint candidates или LAN addresses в dashboard.

## Client data plane

| Leak surface | Current code path | Risk | Required AnonBird behavior |
|---|---|---|---|
| ICE allocation | `client/internal/peer/conn.go`, `Conn.Open`, `NewWorkerICE(...)` | Creates ICE agent and can gather host/srflx candidates. | Do not create `WorkerICE` when `AnonymousMode=true`; relay-only must be unconditional. |
| ICE signaling | `client/internal/peer/signaler.go`, `SignalICECandidate` | Sends serialized ICE candidate to peer through signal. | Drop/refuse ICE candidate signaling in anonymous mode; tests must assert zero candidate messages. |
| Direct WG endpoint selection | `client/internal/peer/conn.go`, ICE ready path configures WireGuard endpoint | Remote peer learns direct UDP endpoint. | Direct endpoint updater must never be reached in anonymous mode. |
| STUN/TURN config | `client/internal/engine.go`, `handleSync`, `updateSTUNs`, `updateTURNs`, `createICEConfig` | Management-provided STUN/TURN enables endpoint discovery. | Ignore and reject STUN/TURN entries in anonymous mode; runtime validation fails closed. |
| NAT external IP mapping | `client/internal/profilemanager/config.go`, `NATExternalIPs`; `client/internal/engine_generic.go` | User can advertise explicit public IP or STUN mapping. | Config validation rejects non-empty NAT external IPs in anonymous mode. |
| UDP mux/srflx | `client/internal/engine_generic.go`, `UDPMux`, `UDPMuxSrflx` | Enables UDP candidate generation. | ICE disabled means UDP mux is unused for peer discovery. |
| Relay server IP fallback | `shared/signal/proto/signalexchange.proto` field `relayServerIP`; `shared/signal/client/client.go`; `shared/relay/client/client.go` `serverIP` shortcut | Peer receives or uses clearnet IP fallback for relay. | Suppress `relayServerIP` in anonymous mode; relay client ignores `serverIP` shortcut when SOCKS5/I2P anonymous dialers are enabled. |
| Relay QUIC/direct TCP | `shared/relay/client/dialers_generic.go` | QUIC and direct dialing bypass Tor. | Anonymous relay client uses WebSocket-over-SOCKS5 only. |
| Kernel WireGuard endpoint | `client/iface/iface_new_linux.go` | Kernel interface expects UDP endpoint semantics and can expose routing behavior. | Force userspace WireGuard in anonymous mode. |

## Client control plane

| Leak surface | Current code path | Risk | Required AnonBird behavior |
|---|---|---|---|
| Management gRPC dial | `client/internal/connect.go` -> `shared/management/client.NewClient` -> `client/grpc.CreateConnection` | Normal TCP dialer exposes source IP to management. | Anonymous mode requires `.onion` or `.b32.i2p` URL and SOCKS5/I2P dialer. |
| Signal gRPC dial | `client/internal/connect.go` -> `shared/signal/client.NewClient` | Normal TCP dialer exposes source IP to signal server. | Signal server must be anonymous endpoint and use SOCKS5/I2P dialer. |
| Management URL parser | `client/internal/profilemanager/config.go`, `parseURL` | Parser accepts normal HTTPS hosts. | Anonymous config validation rejects clearnet management hosts. |
| Client system metadata | `client/system/info.go`; `shared/management/client/grpc.go` `infoToMetaData` | Sends hostname, network addresses, serial/manufacturer data. | Sanitize hostname and remove local network/hardware identifiers in anonymous mode. |
| Auto update / external checks | `client/server/event.go`, `client/server/server.go`, `client/internal/updater`, `version/update.go` | GUI event subscription can start the update manager and fetch `https://pkgs.netbird.io/releases/latest/version` outside anonymous transport. | Anonymous profile stops/does not create the update manager, reports update settings disabled to UI, and must not start clearnet update/check flows unless routed through anonymous transport. |
| Cloud URL migration probe | `client/internal/profilemanager/config.go`, `UpdateOldManagementURL` | Legacy managed-cloud migration can probe `api.netbird.io:443` via normal management gRPC client. | Skip the migration probe entirely when `AnonymousMode=true`; anonymous config validation handles allowed management URLs. |
| OAuth/IdP provider endpoints | `client/internal/auth/auth.go`, `device_flow.go`, `pkce_flow.go` | Management can return clearnet token/device/authorization endpoints; device/PKCE flows otherwise use normal `net/http` clients and can reveal the client IP to the IdP. | Anonymous profile rejects provider endpoints that do not match the selected `.onion`/`.b32.i2p` transport, routes device/token requests through Tor SOCKS5 or I2P SAM, and validates device verification URLs before showing them. |
| Client metrics push | `client/internal/connect.go`; `client/internal/metrics` | Env-enabled metrics push can fetch `https://ingest.netbird.io/config` and push to `https://ingest.netbird.io`. | Anonymous profile never starts metrics push even if `NB_METRICS_PUSH_ENABLED=true`; local metrics collection remains available for debug bundles. |
| Debug bundle upload | `client/internal/debug/upload.go`; CLI/UI/Android/remote bundle jobs | User, UI or management job can upload bundles to `https://upload.debug.netbird.io` and receive clearnet presigned upload URLs. | For anonymous management URLs, reject non-`.onion`/`.i2p` upload endpoints and non-anonymous presigned URLs before uploading. |

## Management server

| Leak surface | Current code path | Risk | Required AnonBird behavior |
|---|---|---|---|
| Real IP extraction | `management/internals/shared/grpc/server.go`, `getRealIP` | Stores reverse-proxy/source IP. | If peer metadata says anonymous mode, treat source IP as nil and log `anonymous`. |
| Login storage | `server.go`, `Login`, `PeerLogin.ConnectionIP` | Real IP can be saved in peer location. | Anonymous login passes nil `ConnectionIP`. |
| Sync storage | `server.go`, `Sync`, `SyncAndMarkPeer(... realIP ...)` | Real IP can update peer location on every sync. | Anonymous sync passes nil realIP and excludes it from meta hash. |
| Events | `management/server/peer/peer.go`, `Peer.EventMeta` | Activity stream may include `location_connection_ip` or stale geo metadata. | Omit connection IP and location fields for anonymous peers. |
| Peer HTTP API | `management/server/http/handlers/peers/peers_handler.go` | Dashboard receives `connection_ip`, stale geo metadata or hardware serial. | Return empty connection IP, location fields and serial plus anonymous flag for anonymous peers. |
| X-Forwarded-For trust | gRPC realip middleware | Reverse proxy headers can become stored identity. | Anonymous mode must ignore this path completely. |
| Self-hosted anonymous metrics | `management/server/metrics/selfhosted.go`; `management/internals/server/server.go`; `combined/cmd/config.go`; `management/cmd/management.go` | Management server can push usage metrics to `https://metrics.netbird.io` outside anonymous transport. | Disable metrics automatically for `.onion`/`.i2p` combined exposed address or standalone management anonymous auth/OIDC endpoints. |
| Geolocation database update | `management/server/geolocation/database.go`; `management/internals/server/modules.go`; `combined/cmd/config.go`; `management/cmd/management.go` | Management server can download GeoLite databases from `https://pkgs.netbird.io` outside anonymous transport. | Disable geolite updates automatically for `.onion`/`.i2p` combined exposed address or standalone management anonymous auth/OIDC endpoints. |
| External OIDC/JWKS fetch | `management/cmd/management.go`; `shared/auth/jwt/validator.go`; `management/server/identity_provider.go` | Anonymous self-hosted setup with external `.onion`/`.b32.i2p` IdP can otherwise use normal `http.Get`, leaking server IP or failing through clearnet DNS. | Route anonymous OIDC discovery, JWKS refresh and issuer validation via local Tor SOCKS5 or I2P SAM; clearnet IdP remains direct and embedded IdP uses direct key fetcher. |

## Dashboard/UI

| Leak surface | Current code path | Risk | Required AnonBird behavior |
|---|---|---|---|
| Peer details page | `/Users/kirill/Code/dashboard/src/app/(dashboard)/peer/page.tsx` | Shows `peer.ip`, `peer.ipv6`, `peer.connection_ip`; exposes IP editing affordances. | Show overlay IP only; hide connection IP and endpoint/LAN fields for anonymous peers. |
| Peer table/address tooltip | `/Users/kirill/Code/dashboard/src/modules/peers/PeerAddressCell.tsx`, `PeerAddressTooltipContent.tsx` | Tooltip can reveal connection IP. | Anonymous tooltip shows transport/status, not real source IP. |
| Remote access pages | `/Users/kirill/Code/dashboard/src/app/(remote-access)/peer/ssh/page.tsx`, `rdp/page.tsx` | Labels overlay IP as direct IP and can imply real endpoint. | Use overlay address wording and anonymous badge. |
| Activity descriptions | `/Users/kirill/Code/dashboard/src/modules/activity/ActivityDescription.tsx`; `management/server/peer/peer.go` `EventMeta` | Events can render connection IP metadata, including in dev fallback tooltips. | Anonymous peer events carry `anonymous_mode`/`anonymous_transport`; dashboard suppresses `location_*` meta for anonymous events. |

## Runtime validation checklist

Anonymous mode startup must fail when any of these are true:

- management URL host is not `.onion` or `.b32.i2p`;
- signal URL host is not `.onion` or `.b32.i2p`;
- relay URL host is not `.onion` or `.b32.i2p`;
- STUN/TURN list is non-empty;
- `NATExternalIPs` is non-empty or contains `stun`;
- OAuth token/device/authorization endpoint returned by management is not `.onion`/`.b32.i2p` for the selected transport;
- ICE/direct UDP is enabled;
- relay client can use QUIC or direct IP fallback;
- peer metadata contains local network addresses or real host identifiers.

## Test targets

- Config tests: anonymous mode accepts `.onion` management and rejects clearnet management/NAT external IPs.
- Auth tests: anonymous SSO rejects clearnet/mismatched IdP endpoints and uses the configured anonymous HTTP client for token exchange.
- Signal tests: anonymous credential marshal omits `relayServerIP`.
- Peer connection tests: anonymous `Conn.Open` never initializes `WorkerICE`.
- Engine tests: anonymous sync rejects STUN/TURN and clears ICE config inputs.
- Management tests: anonymous Login/Sync do not store `ConnectionIP`.
- API/dashboard tests: anonymous peers do not render `connection_ip`, endpoint candidates or LAN addresses.
