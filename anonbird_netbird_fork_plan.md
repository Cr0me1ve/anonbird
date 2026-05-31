# План форка NetBird для anonymous mesh через Tor/I2P

## Журнал реализации

Дата: 2026-05-31

- Статус: в работе.
- Текущий фокус: Phase 1-4, Phase 5 Tor relay stream multipath/per-channel health и Phase 6 direct-I2P MVP закрыты на code/unit + стендовом уровне; leak-map audit закрывает найденные clearnet side channels, remaining follow-up — long-soak/benchmark variants и финальная сверка.
- Правило выполнения: каждая реализованная часть отмечается здесь или в соответствующем чеклисте ниже; если в ходе сверки с ТЗ появляются ограничения или риски, они фиксируются в заметках.
- Заметка: проектное имя клиента и пользовательских команд — AnonBird; CLI должен использовать `anonbird`.
- Реализовано: Phase 0 документы `docs/leak-map.md` и `docs/netbird-transport-analysis.md` добавлены с конкретными code paths, runtime запретами и тестовыми целями.
- Реализовано: клиентский `anonymous_mode` с transport config `tor-relay-only`, CLI flags, daemon proto/config persistence и runtime validation.
- Реализовано: SOCKS5 gRPC/WebSocket dial paths для management, signal и relay; в anonymous mode management/signal/relay проходят через Tor SOCKS5 без clearnet fallback.
- Реализовано: STUN/ICE/direct UDP kill-switch — anonymous mode принудительно отключает ICE worker/candidates, STUN/TURN из network map, NAT external IPs, lazy connection, client/server routes, LAN access и kernel WireGuard path.
- Реализовано: sanitization/redaction — hostname/system/network metadata sanitization на клиенте, anonymous flag в management metadata/API, redaction real source IP в login/sync/audit peer event path.
- Реализовано: CLI `anonbird debug anonymous-check`, который проверяет anonymous mode, transport, management/signal/relay endpoint class, STUN/ICE/direct UDP, published ICE endpoints и userspace/kernel WG mode.
- Реализовано: `anonbird debug anonymous-check` теперь работает и в single-config daemon mode без active profile state, fallback читает default profile через daemon API.
- Реализовано: CLI `anonbird join "anonbird://join?server=...&setup_key=...&transport=tor-relay-only"` с parser/validation, который включает anonymous mode и передаёт настройки в существующий `up` flow.
- Реализовано: серверная operator-команда `setup-key bootstrap` для `management` и `combined`, создающая account при необходимости и печатающая plain setup key один раз; это bootstrap path для anonymous/self-hosted без dashboard/SSO.
- Реализовано: исправлен `combined` server config для anonymous стенда — явное `stunPorts: []` теперь реально отключает embedded STUN и не публикует STUN URI клиентам; добавлены regression tests.
- Реализовано: исправлен `combined`/management listen path — `server.listenAddress` теперь передаётся в management listener, legacy gRPC может быть отключён через `disableLegacyManagementPort`, metrics bind наследует host из `listenAddress`; это закрывает clearnet exposure для onion-only стенда.
- Реализовано: исправлен `setup-key bootstrap` для encrypted sqlite store — management/combined CLI теперь подключает field encryption key перед созданием account/setup key, иначе owner user ломал последующий login decrypt.
- Реализовано: `setup-key bootstrap` теперь добавляет All group в auto-groups setup key, чтобы default All-to-All policy реально применялась к enrolled peers.
- Исправлено по итогам integration ping: anonymous mode больше не форсирует `BlockInbound=true`, потому что это блокирует входящий overlay traffic; direct inbound leak уже закрывается запретом ICE/STUN/candidate publication и relay-only transport.
- Реализовано: relay `basic rate limit` — per-peer token bucket для transport bytes на server-side forwarding path, standalone relay flags `--rate-limit-bytes-per-second/--rate-limit-burst-bytes`, combined YAML `server.relayRateLimit`, regression tests.
- Реализовано: Phase 4 service-name UX для anonymous invite — `anonbird join` теперь принимает `service`, `services`, `dns_label(s)` параметры, валидирует их как DNS labels и прокидывает в существующий `--extra-dns-labels` flow.
- Реализовано: Phase 5 foundation — `client/internal/anonymous/multipath` классифицирует plaintext IPv4/IPv6 overlay packets в direction-neutral flow keys и выбирает healthy channel через rendezvous hashing; это проверяемая часть userspace anonymous transport adapter, не заглушка поверх encrypted relay packets.
- Реализовано: Phase 5 runtime Tor relay multipath — relay protocol/client/server поддерживает логические transport channels (`channel 0` legacy-compatible), `Manager.OpenConnChannel` прокидывает channels через home/foreign relay, `WorkerRelay` в anonymous `tor-relay-only` режиме открывает 2 relay channels, `FilteredDevice` packet observers дают plaintext flow hints до WireGuard encryption, а объединённый relay `net.Conn` направляет только WireGuard data packets по flow-affinity; WireGuard handshake/control остаются на channel 0.
- Реализовано: Phase 5 real Tor stream multipath hardening — relay handshake получил `AuthChannel`, relay server store теперь держит несколько live sessions одного peer по `channel_id`, а anonymous `channel > 0` открывается отдельным relay client/WebSocket через SOCKS5/I2P dialer вместо мультиплексирования внутри одного relay socket. Server-side transport routing выбирает matching peer channel и fallback на channel 0, чтобы не терять early packets пока второй peer поднимает тот же stream.
- Реализовано: Phase 5 per-channel health/adaptive channel count — relay multipath `net.Conn` помечает failed channel unhealthy по read/write/healthcheck-close, продолжает работу через оставшиеся healthy channels и перестраивает rendezvous selection без packet-level round-robin; `WorkerRelay` пытается открыть до 4 anonymous channels, но включает фактически поднятое число и fallback на primary, если extra streams недоступны.
- Исправлено: anonymous Tor/I2P transports получили длинные relay healthcheck и gRPC keepalive тайминги: SOCKS5/I2P management/signal dialers используют anonymous keepalive, relay healthcheck sender/receiver переключаются на anonymous options для `.onion`/`.i2p`, а management/signal server-side keepalive ослаблен для high-latency hidden services. Это устранило ложные disconnect после 30-35 секунд на реальном Tor стенде.
- Реализовано: Phase 6 foundation — `shared/anonymous/i2psam` добавляет реальный I2P SAM STREAM dialer (`HELLO`, `SESSION CREATE`, `STREAM CONNECT`), SAM availability check, `SIGNATURE_TYPE=7`, явные `inbound/outbound.quantity=3`, quoted SAM reply parsing и naming lookup с fake-SAM regression tests.
- Реализовано: anonymous transport `i2p-datagram` теперь валидируется как I2P SAM transport для `.b32.i2p` management/signal/relay endpoints; management/signal gRPC и relay WebSocket могут dial через SAM, а transport mismatch `.onion` vs `.b32.i2p` отклоняется до подключения.
- Реализовано: Phase 6 datagram primitive — `shared/anonymous/i2psam` теперь создаёт настоящие SAM `STYLE=RAW`/`STYLE=DATAGRAM` sessions, отправляет datagrams через SAM UDP bridge с SAM v3 header и разбирает `RAW RECEIVED`/`DATAGRAM RECEIVED` payload с ports/protocol metadata.
- Реализовано: Phase 6 destination primitive — `shared/anonymous/i2psam` поддерживает `DEST GENERATE SIGNATURE_TYPE=7` и создание SAM datagram sessions на persistent private destination key вместо `TRANSIENT`.
- Реализовано: Phase 6 tunnel tuning config — CLI/daemon proto/profile config/join tokens/debug output принимают `i2p_tunnel_length`/`i2p_tunnel_quantity`, I2P SAM STREAM и RAW/DATAGRAM session create передают `inbound/outbound.length` и `inbound/outbound.quantity`, invalid values rejected до dial.
- Реализовано: Phase 6 I2P destination registration/exchange — клиент в `i2p-datagram` mode генерирует persistent SAM destination при первом подключении, сохраняет private key только в локальном profile JSON, отправляет в management только public destination через `PeerSystemMeta.anonymousTransport`, management хранит его в peer meta и возвращает другим пирам через `RemotePeerConfig.anonymousTransport`.
- Реализовано: `anonbird debug anonymous-check` и daemon `GetConfig` теперь показывают только статус/public I2P destination registration (`registered`/`missing`), не раскрывая private destination key.
- Реализовано: Phase 6 i2pd lifecycle — transport config/CLI/daemon/join tokens получили `i2p_daemon_mode` (`external`/`auto`/`managed`), `i2pd_path`, `i2p_data_dir`; `auto`/`managed` запускают реальный `i2pd` процесс, пишут per-profile `i2pd.conf`/`tunnels.conf`, ждут SAM readiness и останавливают управляемый процесс при завершении клиента.
- Реализовано: Phase 6 direct I2P datagram peer transport — `ConnConfig` получает local/remote anonymous transport, `Conn.Open` при `i2p-datagram` открывает direct peer path поверх общего SAM `STYLE=DATAGRAM` session, логический `net.Conn` демультиплексирует peer payloads по I2P destination, а WireGuard endpoint переключается на этот транспорт без ICE/STUN/direct UDP; relay-over-I2P остаётся анонимным fallback.
- Исправлено: i2pd managed/auto data dir больше не автопрописывается в `/etc/anonbird/i2pd` из profile path; Linux root default переведён в `/var/lib/i2pd/anonbird`, создаётся managed `tunnels.d`, а i2pd запускается с `--tunnelsdir`, что совместимо с Debian/Ubuntu AppArmor.
- Исправлено: anonymous LAN-block больше не добавляет drop-rule для собственного WireGuard интерфейса `wt0`, поэтому overlay `100.119.0.0/16` и `fd7b:.../64` не блокируются как "LAN".
- Исправлено: `anonbird join` в daemon mode теперь применяет новую anonymous config даже если daemon уже был connected; join делает controlled reconnect вместо `Already connected`.
- Исправлено: `anonbird debug anonymous-check` признаёт `i2p-datagram` peer endpoint/status anonymous-safe и больше не считает direct I2P endpoint leak.
- Реализовано: I2P management/signal gRPC dial timeout увеличен для SAM STREAM до 2 минут, потому что реальные `.b32.i2p` STREAM CONNECT на холодном i2pd часто превышали обычные 30 секунд.
- Реализовано: direct I2P datagram transport получил retry/upgrade loop после relay fallback; если одна сторона временно ушла в relay, она повторно поднимает direct I2P и избегает асимметрии `direct`/`relay`, которая давала WireGuard handshake без overlay payload.
- Исправлено: direct I2P теперь сбрасывает WireGuard watcher при повторной настройке endpoint и при fallback на relay; stale `Connected` без свежего handshake переводится в reconnect за короткий timeout, после чего direct retry снова поднимает `i2p-datagram`.
- Исправлено: I2P-only combined relay healthcheck получил localhost/self probe для multiplexed `/relay` handler и явный `ws` protocol override; `/health` больше не пытается резолвить публичный `.b32.i2p` обычным DNS на самом сервере.
- Реализовано: Linux service install/reconfigure теперь добавляет anonymous runtime dependencies: для `i2p-datagram` — `Wants/After=i2pd.service`, для `tor-relay-only` — `Wants/After=tor.service`; текущий I2P стенд приведён к этому unit layout.
- Реализовано: AnonBird daemon default socket переведён на `unix:///var/run/anonbird.sock` (CLI/UI/SSH helper/Docker env/daemon discovery), поэтому команды `anonbird status`/`debug` работают с установленным `anonbird.service` без ручного `--daemon-addr`.
- Реализовано в `Code/dashboard`: title AnonBird Dashboard, `anonymous_mode` peer flag, peer/table UI hides public IP/region/serial for anonymous peers, shows network mode/transport, hides network routes for anonymous peers, setup/SSH commands use `anonbird up --anonymous-mode --anonymous-transport tor-relay-only`.
- Реализовано в `Code/dashboard`: install modal получил transport selector Tor/I2P; I2P command generation добавляет `--i2p-sam`, `--i2p-tunnel-length`, `--i2p-tunnel-quantity`, Docker flow добавляет соответствующие `NB_I2P_*`/anonymous env vars.
- Реализовано в `Code/dashboard`: I2P install flow получил controls для i2pd lifecycle (`auto`/`managed`/`external`, path, data dir); CLI/Docker command generation добавляет `--i2p-daemon-mode`, `--i2pd-path`, optional `--i2p-data-dir` и соответствующие `NB_I2P_DAEMON_MODE`/`NB_I2PD_PATH`/`NB_I2P_DATA_DIR`.
- Реализовано в `Code/dashboard`: anonymous peer overview получил `Anonymous safety check` warning, если к peer/group назначены network/exit routes или включены несовместимые local flags (`disable_client_routes=false`, `disable_server_routes=false`, `block_lan_access=false`, `disable_firewall=true`, `lazy_connection_enabled=true`); warning не раскрывает real IP/endpoint metadata.
- Реализовано в `Code/dashboard`: install/setup-key flow теперь предпочитает короткий `anonbird join "anonbird://join?...` invite command с management URL, setup key, transport и I2P lifecycle/tunnel params; если setup key ещё placeholder или management URL не настроен, UI корректно остаётся на `anonbird up` fallback.
- Реализовано в `Code/dashboard`: audit activity rendering теперь redacts `location_*` meta для anonymous peer events; management `Peer.EventMeta` дополнительно помечает anonymous events через `anonymous_mode`/`anonymous_transport`, чтобы UI не показывал connection IP/location даже при fallback/dev tooltip.
- Реализовано: пользовательские CLI help/error/service strings переведены на AnonBird (`anonbird service ...`, `anonbird status/debug/capture/ssh`, service display name `AnonBird`, generated SSH config header/commands); совместимые import path/protocol/file identifiers (`github.com/netbirdio/netbird`, `netbird-ssh`, legacy config/package paths) намеренно не переименованы без отдельной миграции упаковки и API.
- Реализовано в `Code/dashboard`: видимые product strings в setup/install, onboarding, invite/error, activity, SSH, reverse proxy, access tokens, settings и peer/network empty states переведены на AnonBird; full logo component больше не рендерит старый wordmark, package/image/doc URLs и protocol/CSS identifiers оставлены как compatibility-layer до реального выпуска AnonBird packages/docs.
- Реализовано: management HTTP peer API теперь отдаёт `anonymous_transport` только для anonymous peers и продолжает скрывать `connection_ip`; dashboard peer details/tooltip показывают реальный anonymous transport (`tor-relay-only` или `i2p-datagram`) вместо общего `anonymous relay`.
- Реализовано: добавлен production operations doc `docs/anonbird-i2p-operations.md` с i2pd 2.60+ guidance, daemon modes, managed data layout, systemd dependencies, health/recovery commands и security checks для SAM/I2P.
- Исправлено: anonymous profile теперь останавливает/не создаёт client update manager, GUI event subscription не запускает `https://pkgs.netbird.io/releases/latest/version`, а daemon `GetFeatures` помечает update settings disabled для anonymous profile; это закрывает clearnet external-update check из leak-map.
- Исправлено: anonymous/onion/I2P combined management больше не запускает server-side `https://pkgs.netbird.io/releases/latest/version` check — добавлен `DisableVersionCheck` в management server config, standalone management flag `--disable-version-check`, combined YAML `server.disableVersionCheck` и автоотключение при `.onion`/`.i2p` `server.exposedAddress`.
- Исправлено: anonymous client больше не запускает NAT port mapper; `Engine.Start` пропускает PCP/NAT-PMP/UPnP discovery в anonymous mode и логирует `NAT port mapper is disabled in anonymous mode`, чтобы локальный gateway discovery не становился UDP leak.
- Исправлено: leak-map audit hardening — relay client больше не использует `serverIP` direct shortcut при SOCKS5/I2P anonymous relay dialers даже если caller ошибочно передал IP, а `UpdateOldManagementURL` пропускает cloud-management migration probe в anonymous mode, чтобы legacy URL helper не мог сделать clearnet healthcheck.
- Исправлено: management API/event redaction для anonymous peers теперь backend-side скрывает не только `connection_ip`, но и сохранённые `city/country/geoname` и hardware serial в peer single/list/accessible-peer responses и activity `EventMeta`, чтобы UI не был единственным барьером.
- Исправлено: self-hosted external server checks leak — combined автоматически отключает push на `https://metrics.netbird.io`, geolite download с `pkgs.netbird.io` и version update checks, когда `server.exposedAddress` использует `.onion`/`.i2p`; standalone management автоматически отключает anonymous metrics, geolite updates и version update checks, если auth/oidc/device-flow endpoints указывают на `.onion`/`.i2p`.
- Исправлено: client metrics push leak — anonymous profile теперь не запускает metrics remote-config/push на `https://ingest.netbird.io` даже если оператор выставил `NB_METRICS_PUSH_ENABLED=true`; metrics collection для local debug bundle остаётся локальной.
- Исправлено: debug bundle upload leak — общий `UploadDebugBundle` запрещает upload на clearnet URL для anonymous management (`.onion`/`.i2p`) и дополнительно проверяет presigned PUT URL; CLI/UI/Android/remote job больше не могут случайно отправить bundle на `https://upload.debug.netbird.io` из anonymous profile.
- Исправлено: debug bundle config coverage — `config.txt` теперь явно показывает `AnonymousMode` и безопасную сводку `AnonymousTransport` без Tor/SAM адресов, I2P public/private destination и локальных path; `ConfigPath` исключён как локальный filesystem identifier.
- Исправлено: OAuth/IdP leak — anonymous SSO теперь отклоняет clearnet или transport-mismatch provider endpoints (`token`, `device`, `authorization`), device/PKCE token exchange ходит через SOCKS5 Tor или I2P SAM HTTP transport, а device verification URL из ответа IdP валидируется перед показом пользователю.
- Исправлено: server-side external OIDC/JWKS leak — management OIDC discovery, JWKS refresh и issuer validation теперь для `.onion`/`.b32.i2p` endpoints используют общий anonymous HTTP client через локальный Tor SOCKS5 или I2P SAM вместо обычного `http.Get`; embedded IdP по-прежнему использует direct key fetcher без сетевого обхода наружу.
- Сверка с UI/management ТЗ: device approval уже есть в существующем dashboard/API (`approval_required`, peer approve action) и подходит для anonymous peers без раскрытия IP. Relay health/status достоверно доступен на client side (`anonbird status`, `anonbird debug anonymous-check`); management dashboard пока не получает per-client relay health, поэтому fake relay status в UI не добавляется.
- Проверки 2026-05-31: `go test ./client/cmd ./client/internal/anonymous ./client/internal/profilemanager ./shared/signal/client ./client/internal/peer ./shared/relay/client ./shared/relay/client/dialer/ws ./management/server/peer` — OK; после `join` добавки повторно `go test ./client/cmd` — OK; dashboard `npm run build` — OK.
- Проверки 2026-05-31: `go test ./management/cmd/setupkey ./combined/cmd ./management/cmd` — OK.
- Проверки 2026-05-31: direct `npx tsc --noEmit` в dashboard падает на существующих декларациях assets (`.svg/.png/.jpg`), но `next build` проходит; `npm run lint` сейчас сломан upstream-скриптом `next lint` для Next 16 / ESLint 9.
- Заметка: для локальной browser-проверки dashboard создан ignored `.local-config.json`; без backend/auth ожидаемо видна login error page, но build/runtime module error отсутствует, title = `AnonBird Dashboard`.
- Заметка: тестовые серверы для integration этапа: `93.177.116.58`, `213.108.3.228`, `185.246.220.249`, `45.138.103.224`.
- Проверки 2026-05-31: real integration на тестовых серверах — enrollment через `.onion` setup key OK, all clients anonymous-check OK, relayed peers `2/2 Connected`, ICE/STUN/direct UDP отсутствуют, WG userspace.
- Проверки 2026-05-31: end-to-end overlay ping через Tor onion relay подтверждён между всеми тремя клиентами, 0% loss: `100.79.239.83` ↔ `100.79.204.47`/`100.79.143.119`, `100.79.204.47` ↔ `100.79.143.119`; RTT примерно 0.8–1.2s.
- Проверки 2026-05-31: после включения `server.relayRateLimit` на onion combined (`1048576` B/s, burst `2097152`) сервис активен, слушает только `127.0.0.1:8080/19000/19090`, клиенты снова `Management/Signal Connected`, `Relays 1/1 Available`, `Peers 2/2 Connected`, overlay ping 0% loss.
- Проверки 2026-05-31: Phase 4 DNS/no-exit на стенде — `anonbird-*.anonbird.local` резолвится в overlay IPv6 и ping по имени работает через relay; route table содержит только overlay `100.79.0.0/16 dev wt0`, default route не переносится в AnonBird.
- Проверки 2026-05-31: `go test ./client/internal/anonymous/...` — OK, включая Phase 5 flow classifier/healthy-channel selector.
- Проверки 2026-05-31: `go test ./shared/anonymous/i2psam ./client/cmd ./client/internal/profilemanager ./shared/relay/client` — OK; compile-only `go test -run '^$' ./client/internal ./client/server ./shared/management/client ./shared/signal/client ./shared/relay/client/dialer/ws ./client/grpc` — OK.
- Проверки 2026-05-31: после SAM datagram/destination primitive — `go test ./shared/anonymous/i2psam ./client/internal/anonymous/... ./client/internal/profilemanager ./client/cmd` OK; compile-only `go test -run '^$' ./client/internal ./client/server ./shared/management/client ./shared/signal/client ./shared/relay/client/dialer/ws ./client/grpc ./shared/relay/client` OK.
- Проверки 2026-05-31: dashboard после I2P foundation copy update — `npm run build` в `/Users/kirill/Code/dashboard` OK.
- Проверки 2026-05-31: после I2P tunnel tuning — `go test ./shared/anonymous/i2psam ./client/internal/anonymous/... ./client/internal/profilemanager ./client/cmd` OK; compile-only `go test -run '^$' ./client/internal ./client/server ./shared/management/client ./shared/signal/client ./shared/relay/client/dialer/ws ./client/grpc ./shared/relay/client` OK; `git diff --check` OK.
- Проверки 2026-05-31: dashboard I2P transport selector — `npm run build` OK; local `/install` DOM verified через in-app Browser: switching Tor→I2P exposes SAM/length/quantity inputs and generated command contains `--i2p-sam 127.0.0.1:7656 --i2p-tunnel-length 1 --i2p-tunnel-quantity 3`.
- Проверки 2026-05-31: после I2P destination registration/exchange — `go test ./client/internal/anonymous ./client/internal/profilemanager ./shared/management/client ./management/server/peer` OK; `go test ./client/cmd` OK; compile-only `go test -run '^$' ./management/internals/shared/grpc ./client/internal ./client/server ./client/internal/auth ./shared/management/proto ./management/server/store ./client/proto` OK; `go test -run 'TestToSyncResponse|TestMigrate' ./management/server ./management/server/store` OK; `git diff --check` OK.
- Проверки 2026-05-31: после i2pd lifecycle — `go test ./client/internal/anonymous ./client/internal/profilemanager ./client/cmd` OK; compile-only `go test -run '^$' ./client/internal ./client/server ./client/internal/auth ./client/proto` OK; dashboard `npm run build` OK; local `/install` opened через in-app Browser, base install modal rendered, but switching Tor→I2P in Browser hit CDP click timeout, so final UI interaction needs a later browser retry.
- Проверки 2026-05-31: после Phase 5 runtime multipath — `go test ./client/internal/anonymous/... ./client/iface/device ./client/internal/peer ./shared/relay/messages ./shared/relay/client ./relay/server -count=1 -timeout 180s` OK; targeted pre-checks OK: `go test ./client/internal/anonymous/multipath ./client/iface/device ./client/internal/peer -run 'TestClassify|TestSelect|TestDeviceWrapperPacketObserver|TestRelayMultipathConn'`, `go test ./shared/relay/messages`, `go test ./shared/relay/client -run 'TestClientMultipleChannels|TestForeignConn|TestForeignAutoClose'`, `go test ./relay/server`.
- Проверки 2026-05-31: после Phase 5 real Tor stream/per-channel health hardening — targeted `go test ./shared/relay/messages ./relay/server/store ./relay/server ./shared/relay/client ./client/internal/peer -run 'TestMarshalAuth|TestUnmarshalAuth|TestStore|TestConfigValidate|TestPeerRateLimit|TestClientMultipleChannels|TestClientDedicatedRelayChannel|TestRelayMultipathConn' -count=1 -timeout 120s` OK; broader `go test ./shared/relay/messages ./relay/server/store ./relay/server ./shared/relay/client ./client/internal/peer ./client/internal/anonymous/... ./client/iface/device ./client/grpc ./shared/relay/healthcheck ./combined/cmd ./management/internals/server ./management/cmd -count=1 -timeout 180s` OK; `git diff --check` OK.
- Проверки 2026-05-31: Phase 5 real Tor stream/per-channel health hardening задеплоен на Tor стенд `93.177.116.58` + `185.246.220.249`/`45.138.103.224`; server logs подтверждают `relay_channel` 0..3 для обоих peers, клиенты открыли отдельные SOCKS5/WebSocket relay sessions и залогировали `anonymous relay multipath enabled with 4/4 channels`; после нагрузки оба клиента сохранили `Management/Signal Connected`, relay `Available`, peer pair `Relayed`, без новых `health check timeout`/`keepalive ping failed`/`PCP discovery`/`NAT-PMP`/`UPnP` и без server-side `pkgs.netbird.io`/version-check утечек.
- Проверки 2026-05-31: Phase 5 4-channel stream benchmark между `185.246.220.249` (`100.79.204.47`) и `45.138.103.224` (`100.79.143.119`) — ping 6/6 packets в обе стороны, avg RTT `822.687 ms` и `783.181 ms`; `iperf3 -P 4 -t 25` без client/server error, client sender `8.01 Mbits/s`, server receiver `5.827 Mbits/s`.
- Проверки 2026-05-31: после anonymous health/keepalive hardening — `go test ./client/grpc ./shared/relay/healthcheck ./client/internal/anonymous/... ./client/iface/device ./client/internal/peer ./shared/relay/messages ./shared/relay/client ./relay/server ./management/internals/server ./signal/cmd -count=1 -timeout 180s` OK.
- Проверки 2026-05-31: Phase 5 runtime multipath задеплоен на Tor стенд `93.177.116.58` + `185.246.220.249`/`45.138.103.224`; логи подтверждают открытие relay channels 0 и 1 и `anonymous relay multipath enabled with 2 channels`; после 150s hold оба клиента сохранили `Management/Signal Connected`, `Relays 1/1 Available`, peer connection `Relayed`, новых relay healthcheck/gRPC keepalive failures нет.
- Проверки 2026-05-31: Phase 5 runtime multipath ping между `185.246.220.249` (`100.79.204.47`) и `45.138.103.224` (`100.79.143.119`) — 6/6 packets в обе стороны, avg RTT `~817 ms` и `~811 ms`.
- Проверки 2026-05-31: Phase 5 runtime multipath throughput smoke через Tor relay: single-stream baseline `185→45` server receiver `~1.86 Mbits/s`; 4-stream runtime multipath run server receiver `~2.32 Mbits/s`. `iperf3 -P 4` завершился с control-message/BFD caveat на iperf, но receiver summary на server side получен; Tor TCP-over-relay остаётся вариативным.
- Проверки 2026-05-31: после direct I2P datagram peer transport — `go test ./client/internal/peer ./client/internal/anonymous ./client/internal/profilemanager ./client/cmd` OK; compile-only `go test -run '^$' ./client/internal ./client/server ./client/internal/auth ./client/proto ./shared/management/client ./management/internals/shared/grpc` OK; `GOOS=linux GOARCH=amd64 go test -run '^$' ./client/iface/wgproxy/udp` OK.
- Проверки 2026-05-31: final direct-I2P sweep — `go test ./shared/anonymous/i2psam ./client/internal/peer ./client/internal/anonymous ./client/internal/profilemanager ./client/cmd` OK; `git diff --check` в `/Users/kirill/Code/netbird` OK; dashboard `npm run build` OK; `git diff --check` в `/Users/kirill/Code/dashboard` OK.
- Проверки 2026-05-31: real I2P management/signal/relay стенд поднят на `213.108.3.228` через i2pd 2.60.0 PPA, service слушает только `127.0.0.1:8080/19000/19090`, I2P server tunnel доступен как `pulknohg3yhfor65tprlfujboc3c6ozj7qxll2c56oec6ywucrnq.b32.i2p:80`; клиенты `185.246.220.249` и `45.138.103.224` enrolled через `i2p-datagram`, `anonymous-check` OK, STUN/ICE/direct UDP/clearnet fallback отсутствуют.
- Проверки 2026-05-31: direct I2P datagram data-plane validated между `185.246.220.249` (`100.119.42.220`) и `45.138.103.224` (`100.119.148.129`): оба статуса `P2P`, endpoints `i2p-datagram/i2p-datagram`, `ping 185→45` 8/8 packets, avg ~496 ms; `ping 45→185` 7/8 packets, avg ~449 ms.
- Проверки 2026-05-31: direct I2P throughput smoke через `iperf3`: `185→45` 20s sender 2.12 MiB / 891 Kbits/s, receiver 1.75 MiB / 720 Kbits/s; `45→185` 20s sender 5.38 MiB / 2.25 Mbits/s, receiver 5.12 MiB / 2.10 Mbits/s.
- Проверки 2026-05-31: после LAN-block/socket/I2P retry fixes — `go test ./client/internal/peer ./client/grpc ./client/internal/daemonaddr ./client/cmd ./client/internal/anonymous ./client/internal/profilemanager` OK; `go test ./client/internal -run 'TestGetInterfacePrefixesExcludesNamedInterface|TestCompareNetIPLists'` OK; полный `go test ./client/internal` на macOS без TUN privileges ожидаемо падает на existing engine integration tests с `operation not permitted`.
- Проверки 2026-05-31: после healthcheck/WG-watcher/systemd dependency fixes — `go test ./relay/healthcheck ./combined/cmd ./client/internal/peer ./client/cmd ./client/internal/daemonaddr ./client/grpc ./client/internal/anonymous ./client/internal/profilemanager` OK; `git diff --check` OK.
- Проверки 2026-05-31: I2P-only combined на `213.108.3.228` пересобран с CGO/Dex SQLite на Go 1.26.2 и задеплоен; `curl http://127.0.0.1:19000/health` возвращает `200 OK` со статусом `healthy`, `listeners:["ws"]`.
- Проверки 2026-05-31: после restart обоих I2P clients direct retry подтверждён заново: старт через relay fallback, затем upgrade в `P2P` `i2p-datagram/i2p-datagram`; `185→45` ping 8/8, avg ~399 ms; `45→185` ping 8/8, avg ~364 ms; `anonymous-check` OK, STUN/ICE/direct UDP/clearnet fallback отсутствуют.
- Проверки 2026-05-31: dashboard anonymous safety warning — `npm run build` в `/Users/kirill/Code/dashboard` OK; `git diff --check` в `/Users/kirill/Code/dashboard` OK.
- Проверки 2026-05-31: dashboard anonymous join URL generation — `npm run build` в `/Users/kirill/Code/dashboard` OK; `git diff --check` в `/Users/kirill/Code/dashboard` OK.
- Проверки 2026-05-31: anonymous transport in peer API/dashboard — `go test ./management/server/http/handlers/peers ./management/server/peer ./management/internals/shared/grpc` OK; dashboard `npm run build` OK; `git diff --check` в `/Users/kirill/Code/netbird` и `/Users/kirill/Code/dashboard` OK.
- Проверки 2026-05-31: I2P operations docs сверены с текущим `client/internal/anonymous/i2pd.go` layout и тестовым стендом; `git diff --check` в `/Users/kirill/Code/netbird` OK.
- Проверки 2026-05-31: anonymous update-manager guard — `go test ./client/server -run 'TestAnonymousProfileDisablesUpdateManager|TestServer_SubcribeEvents|TestSetConfig_AllFieldsSaved|TestCLIFlags_MappedToSetConfig'` OK; `go test ./client/internal/updater ./client/internal/anonymous/...` OK.
- Проверки 2026-05-31: после фикса test profile isolation для `TestConnectWithRetryRuns` полный `go test ./client/server` OK; `git diff --check` OK.
- Проверки 2026-05-31: anonymous activity redaction — `go test ./management/server/peer` OK; dashboard `npm run build` OK; `git diff --check` в `/Users/kirill/Code/netbird` и `/Users/kirill/Code/dashboard` OK.
- Проверки 2026-05-31: CLI/dashboard visible rebrand — `go test ./client/cmd ./client/server ./client/ssh/config` OK; dashboard `npm run build` OK; `git diff --check` в `/Users/kirill/Code/netbird` и `/Users/kirill/Code/dashboard` OK.
- Проверки 2026-05-31: server-side version-check guard — `go test ./combined/cmd ./management/cmd ./management/internals/server -count=1 -timeout 180s` OK; broader affected sweep `go test ./combined/cmd ./management/cmd ./management/internals/server ./client/grpc ./shared/relay/healthcheck ./client/internal/anonymous/... ./client/iface/device ./client/internal/peer ./shared/relay/messages ./shared/relay/client ./relay/server ./signal/cmd -count=1 -timeout 180s` OK; `git diff --check` OK.
- Проверки 2026-05-31: combined на `93.177.116.58` пересобран и задеплоен; `/health` возвращает `200`, journal после restart содержит `management version update check disabled` и не содержит `fetching version info`/`pkgs.netbird.io`/`outdated`.
- Проверки 2026-05-31: NAT mapper anonymous guard — `go test ./client/internal -run 'TestShouldStartPortForwardManager|TestGetInterfacePrefixesExcludesNamedInterface|TestCompareNetIPLists' -count=1 -timeout 120s` OK; affected sweep `go test ./client/internal/peer ./client/grpc ./shared/relay/healthcheck ./combined/cmd ./management/internals/server ./management/cmd ./shared/relay/client ./relay/server -count=1 -timeout 180s` OK.
- Проверки 2026-05-31: новый client binary задеплоен на `185.246.220.249` и `45.138.103.224`; после restart оба клиента снова `Management/Signal Connected`, relay `Available`, peer pair `Relayed`, логи после marker содержат `NAT port mapper is disabled in anonymous mode` и не содержат новых `PCP discovery`/`NAT-PMP`/`UPnP`; `anonymous relay multipath enabled with 2 channels` подтверждён на обеих сторонах.
- Проверки 2026-05-31: leak-map audit hardening — `go test ./shared/relay/client -run 'TestClient_ServerIP|TestClient_ConnectedIP|TestSubstituteHost' -count=1 -timeout 120s` OK; `go test ./client/internal/profilemanager -run 'TestUpdateOldManagementURL|TestConfigAnonymous|TestEnsureAnonymous' -count=1 -timeout 120s` OK.
- Проверки 2026-05-31: management anonymous API/event redaction — `go test ./management/server/peer -run 'TestEventMetaOmitsConnectionIPForAnonymousPeer' -count=1 -timeout 120s` OK; `go test ./management/server/http/handlers/peers -run 'TestAnonymousPeerResponse|TestNonAnonymousPeerResponse' -count=1 -timeout 120s` OK.
- Проверки 2026-05-31: self-hosted metrics/geolite/version-check anonymous auto-disable — `go test ./combined/cmd -run 'TestLoadConfigAnonymousExposedAddressDisablesExternalChecks|TestLoadConfigExplicitDisableVersionCheckPropagates' -count=1 -timeout 120s` OK; `go test ./management/cmd -run 'TestManagementConfigUsesAnonymousEndpoint|Test_loadMgmtConfig' -count=1 -timeout 120s` OK.
- Проверки 2026-05-31: client metrics anonymous guard — `go test ./client/internal -run 'TestShouldStartMetricsPushDisabledForAnonymousMode|Test_freePort' -count=1 -timeout 120s` OK.
- Проверки 2026-05-31: debug bundle anonymous upload guard — `go test ./client/internal/debug -run 'TestUploadDebugBundleRejectsClearnetUploadForAnonymousManagement|TestUpload' -count=1 -timeout 120s` OK.
- Проверки 2026-05-31: debug bundle anonymous config coverage — `go test ./client/internal/debug -run 'TestAddConfig_AllFieldsCovered|TestAddCommonConfigFieldsRedactsAnonymousTransportSensitiveFields|TestUploadDebugBundleRejectsClearnetUploadForAnonymousManagement|TestUpload' -count=1 -timeout 120s` OK; полный `go test ./client/internal/debug -count=1 -timeout 180s` OK.
- Проверки 2026-05-31: anonymous OAuth/IdP guard — `go test ./client/internal/anonymous -run 'TestValidateEndpointForTransport|TestValidateNetbirdConfigForTransport|TestValidateTransport' -count=1 -timeout 120s` OK; `go test ./client/internal/auth -run 'TestConfigureAnonymous|TestDeviceAuthorizationFlowRejectsClearnetVerificationURI|TestPKCEWaitTokenUsesConfiguredHTTPClient|TestHosted_RequestDeviceCode|TestHosted_WaitToken|TestPromptLogin' -count=1 -timeout 120s` OK.
- Проверки 2026-05-31: server-side anonymous OIDC/JWKS guard — `go test ./shared/anonymous ./shared/auth/jwt -count=1 -timeout 120s` OK; `go test ./management/cmd -run 'TestManagementConfigUsesAnonymousEndpoint|Test_loadMgmtConfig' -count=1 -timeout 120s` OK; `go test ./management/server -run 'Test.*IdentityProvider|Test.*OIDC|Test.*Auth' -count=1 -timeout 180s` OK.
- Проверки 2026-05-31: leak-map affected sweep — `go test ./client/internal/auth ./client/internal/anonymous ./shared/anonymous ./shared/auth/jwt ./management/cmd ./management/server/peer ./management/server/http/handlers/peers ./combined/cmd ./client/internal/profilemanager ./shared/relay/client ./client/internal/debug -count=1 -timeout 240s` OK; compile-only `go test -run '^$' ./client/internal ./client/server ./shared/management/client ./management/internals/server ./management/internals/shared/grpc ./client/proto ./shared/management/proto -count=1 -timeout 180s` OK; `git diff --check` OK. Полный `go test ./management/server` не является локально runnable без rootless Docker/testcontainers (`rootless Docker not found`), targeted management server tests above passed.
- Проверки 2026-05-31: leak-map guard binaries задеплоены на стенд — client `/usr/local/bin/anonbird` обновлён на `185.246.220.249` и `45.138.103.224`, combined обновлён на `93.177.116.58` и `213.108.3.228`; все systemd services вернулись `active`, `/health` на `93` и `213` возвращает `healthy`, `anonymous-check` на клиентах OK. Tor рабочая пара после restart: `185→45` ping 6/6 avg `823.708 ms`, `45→185` ping 6/6 avg `807.676 ms`; journal grep после restart не показал новых `pkgs.netbird`/`ingest.netbird`/`metrics.netbird`/`upload.debug`/NAT discovery/STUN/ICE строк.
- Проверки 2026-05-31: dashboard repeat sanity — `npm run build` в `/Users/kirill/Code/dashboard` OK, `git diff --check` OK.
- Проверки 2026-05-31: Tor benchmark preflight — SSH к `93.177.116.58` восстановился, Tor onion hostname `o2n24n6pjl4dkz2i3tlyfov3ozpnwcu4bhy26rtd65stqctn6rg3vpad.onion` доступен в конфиге, remote combined всё ещё старой сборки с `/health` 503 из-за onion DNS self-probe, но management/setup-key path через onion работоспособен.
- Проверки 2026-05-31: Tor relay-only benchmark восстановлен на `93.177.116.58` + `185.246.220.249`/`45.138.103.224`; Tor management/signal/relay идут через `http://o2n24n6pjl4dkz2i3tlyfov3ozpnwcu4bhy26rtd65stqctn6rg3vpad.onion:80`, клиенты подключены `Relayed`, `anonymous-check` OK, STUN/ICE/direct UDP/clearnet fallback отсутствуют.
- Проверки 2026-05-31: Tor relay-only ping между `185.246.220.249` (`100.79.204.47`) и `45.138.103.224` (`100.79.143.119`) — 10/10 packets в обе стороны, avg RTT `~1047 ms` и `~1092 ms`.
- Проверки 2026-05-31: Tor relay-only `iperf3` 20s: `185→45` sender `5.75 MiB / 2.41 Mbits/s`, receiver `4.50 MiB / 1.45 Mbits/s`; `45→185` sender `5.38 MiB / 2.25 Mbits/s`, receiver `3.38 MiB / 1.18 Mbits/s`; retransmits `0`.
- Заметка: при восстановлении Tor benchmark существующие peer-записи на `93.177.116.58` были созданы старым setup key без `allow_extra_dns_labels`; так как этот флаг хранится в peer, а не обновляется новым setup key при login, для тестового account `anonbird` выставлен `allow_extra_dns_labels=1` существующим peers. Это изменение только стендовое, не кодовая заглушка.
- Ограничение: Ubuntu/Noble package `i2pd 2.49.0` падал на static server tunnels; на тестовом I2P management server использован PurpleI2P PPA `i2pd 2.60.0`, после чего tunnel стабилен. Production guidance зафиксирован в `docs/anonbird-i2p-operations.md`: использовать 2.60+ или предварительно проверять packaged i2pd.
- Заметка: на `45.138.103.224` i2pd после нескольких restart/нагрузок временно начал отдавать SAM DIAL timeout; `systemctl restart i2pd` восстановил STREAM/DATAGRAM. Это стендовый operational риск, отдельный от AnonBird data-plane; recovery guidance добавлен в `docs/anonbird-i2p-operations.md`.
- Сверка с ТЗ: like-for-like benchmark закрыт на одной клиентской паре `185.246.220.249` ↔ `45.138.103.224`. Для direct I2P ранее зафиксировано `ping avg ~399/364 ms`, throughput `185→45` receiver `720 Kbits/s`, `45→185` receiver `2.10 Mbits/s`; для Tor relay-only зафиксировано `ping avg ~1047/1092 ms`, throughput `185→45` receiver `1.45 Mbits/s`, `45→185` receiver `1.18 Mbits/s`. Tor latency ожидаемо выше, throughput сравним/вариативен из-за TCP-over-Tor relay и server relayRateLimit.
- Сверка с ТЗ: Phase 5 runtime multipath benchmark закрыт на той же Tor клиентской паре. Первый runtime pass с двумя логическими relay channels дал modest throughput uplift на multi-flow smoke (`~2.32 Mbits/s` receiver против single-stream baseline `~1.86 Mbits/s` и прежнего relay-only `~1.45 Mbits/s` в том же направлении). Follow-up hardening заменил logical-only extra channels на отдельные relay WebSocket/SOCKS streams через `AuthChannel`, добавил active relay-client health per stream и adaptive фактическое число channels; повторный 4-channel stream benchmark прошёл без health/keepalive regressions и дал server receiver `5.827 Mbits/s` при `iperf3 -P 4 -t 25`.
- Сверка с ТЗ: Phase 3 и Phase 4 MVP закрыты по чеклисту; повторный leak-map audit закрыл найденные side channels (`serverIP`, legacy cloud migration probe, API geo/serial redaction, self-hosted metrics/geolite/version checks, client metrics, debug upload, client OAuth/IdP provider HTTP, server-side OIDC/JWKS HTTP). Следующий фокус — long-soak checks после новых Tor stream/I2P data-plane изменений.
- Сверка с ТЗ: Phase 5 повторно проверен по дата-плейну. `FilteredDevice.Read` видит plaintext overlay packets до WG и отдаёт flow hints, а anonymous relay multipath `net.Conn` применяет эти hints уже на encrypted WireGuard data packets через rendezvous hashing. Это даёт flow affinity без packet-level round-robin и без попытки парсить inner flow из encrypted relay/I2P payload.

---

## 0. Краткая идея

Цель проекта — сделать open-source форк NetBird, в котором private mesh-сеть работает поверх Tor и/или I2P так, чтобы:

1. Пиры не знали реальные IPv4/IPv6 адреса друг друга.
2. Пиры работали только с внутренними overlay IP и DNS-именами.
3. Управляющий сервер не видел реальные IP подключающихся клиентов.
4. В anonymous mode не было fallback на обычный clearnet/STUN/direct UDP.
5. Подключение оставалось простым: URL управляющего сервера + setup key, как в NetBird.

Проект не предназначен для выхода трафика в интернет. Только private mesh.

---

## 1. Основные security goals

### 1.1. Что должно скрываться

| От кого | Что скрываем |
|---|---|
| От других пиров | Реальный IPv4/IPv6 адрес, порт, NAT-информацию, физическую сеть |
| От управляющего сервера | Реальный IPv4/IPv6 адрес клиента |
| От провайдера/хостера клиента | Содержимое VPN-трафика, реальные IP других пиров |
| От relay | Содержимое VPN-трафика |

### 1.2. Что не скрывается полностью

| Сторона | Что может быть видно |
|---|---|
| Провайдер клиента | Факт использования Tor/I2P, объём трафика, тайминги |
| Управляющий сервер | Identity устройства, public key, overlay IP, ACL, группы, членство в сети |
| Relay | Факт пересылки зашифрованного трафика между anonymous endpoints |
| Сильный глобальный наблюдатель | Может пытаться делать traffic correlation |

### 1.3. Формальная security promise

Проект скрывает реальные IPv4/IPv6 адреса mesh-пиров от других пиров и от управляющего сервера при включённом anonymous mode. Проект не обещает абсолютную анонимность против глобального сетевого наблюдателя и не скрывает сам факт использования Tor/I2P от локального провайдера.

---

## 2. Режимы работы

### 2.1. `tor-relay-only`

Первый и самый реалистичный MVP-режим.

```text
Peer A
  WireGuard userspace
    ↓
  Tor SOCKS / embedded Tor
    ↓
  Onion relay
    ↓
  Tor
    ↓
Peer B
```

Особенности:

- Нет прямого UDP между пирами.
- Нет STUN.
- Нет ICE.
- Нет exchange обычных `ip:port`.
- Все transport endpoints являются onion/relay handles.
- WireGuard-трафик остаётся end-to-end encrypted.
- Relay не видит содержимое inner traffic.

Плюсы:

- Самый простой путь к сокрытию IP.
- Работает за NAT/firewall.
- Control plane тоже можно сделать `.onion`.

Минусы:

- Низкая скорость.
- Высокая задержка.
- Tor плохо подходит для VPN-over-TCP.
- Relay может стать bottleneck.

---

### 2.2. `i2p-datagram`

Более перспективный режим для настоящего anonymous mesh.

```text
Peer A
  WireGuard userspace
    ↓
  I2P SAM / embedded i2pd
    ↓
  I2P tunnels/datagrams
    ↓
Peer B
```

Особенности:

- Endpoint пира — I2P destination, а не IPv4/IPv6.
- Лучше подходит для peer-to-peer модели, чем Tor.
- Можно использовать tunnel quantity/length для баланса скорости и приватности.

Плюсы:

- Потенциально лучше для mesh.
- Более естественная datagram-модель.
- Можно уменьшать tunnel length для private mesh.

Минусы:

- Сложнее UX.
- Меньше аудитория и инфраструктура.
- Нужно аккуратно проектировать transport layer.

---

### 2.3. `hybrid`

Поздний режим, не для MVP.

```text
transport preference:
  1. i2p-datagram
  2. tor-relay-only
  3. clearnet — forbidden in anonymous mode
```

В anonymous mode clearnet fallback должен быть технически запрещён.

---

## 3. UX подключения

Цель — сохранить UX NetBird:

```bash
anonbird up \
  --management-url http://examplexxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.onion \
  --setup-key NB-SETUP-xxxx
```

Или через join token:

```bash
anonbird join "anonbird://join?server=http://example.onion&setup_key=NB-SETUP-xxxx&transport=tor-relay-only"
```

Статус: реализовано 2026-05-31. Parser принимает `server`/`management_url`, `setup_key`/`setup-key`, `transport`, `tor_socks5`, `i2p_sam`, `i2p_tunnel_length`, `i2p_tunnel_quantity`, `hostname`, `profile`; clearnet management и transport mismatch (`tor-relay-only` с `.b32.i2p`, `i2p-datagram` с `.onion`) отклоняются validation.

### 3.1. Что делает клиент после команды

1. Проверяет режим anonymous transport.
2. Проверяет доступность Tor/I2P.
3. При необходимости запускает embedded Tor/i2pd.
4. Генерирует WireGuard keypair локально.
5. Генерирует anonymous transport identity:
   - Tor: onion identity или relay session identity.
   - I2P: destination.
6. Подключается к management server через Tor/I2P.
7. Регистрирует устройство через setup key.
8. Получает overlay IP, DNS имя, ACL, список peers.
9. Поднимает userspace WireGuard.
10. Подключается к relay или I2P peers.
11. Никогда не отправляет control server или другим пирам свой реальный `ip:port`.

---

## 4. Обязательные запреты в anonymous mode

В anonymous mode должно быть невозможно случайно раскрыть IP.

```yaml
anonymous_mode: true

forbidden:
  stun: true
  ice: true
  direct_udp: true
  public_endpoint_advertisement: true
  local_lan_discovery: true
  clearnet_signal: true
  clearnet_management: true
  clearnet_relay: true
```

### 4.1. Что нужно удалить/отключить из обычной модели NetBird

- STUN discovery.
- ICE candidate gathering.
- Direct WireGuard endpoint exchange.
- Использование публичного source IP в peer metadata.
- Clearnet signal server.
- Clearnet relay fallback.
- Логи с real source IP.
- Any `X-Forwarded-For` trust path.
- Внешний OIDC/SSO через clearnet в privacy-first режиме.

---

## 5. Control plane

### 5.1. Management server

Должен быть доступен как:

```text
http://management-name.onion
```

и/или:

```text
http://management-name.b32.i2p
```

В anonymous mode клиент не должен подключаться к management server через обычный HTTPS endpoint.

### 5.2. Что management server знает

Management всё равно будет знать:

- device identity;
- WireGuard public key;
- overlay IP;
- DNS name;
- groups;
- ACL;
- setup key или enrollment policy;
- список устройств в account/network;
- разрешённые связи между peers.

Management не должен знать:

- real IPv4/IPv6 клиента;
- NAT mapping;
- local endpoint candidates;
- public UDP endpoint;
- LAN IP, если это не разрешено отдельной настройкой.

### 5.3. Enrollment

MVP:

```text
management-url + setup-key
```

Позже:

- single-use invite links;
- expiring join tokens;
- device approval;
- QR-code enrollment;
- offline bootstrap bundle.

---

## 6. Signal plane

Обычный NetBird signal server используется для координации соединений. В anonymous fork signal plane должен быть изменён.

### 6.1. Tor relay-only mode

Signal server не должен передавать `ip:port`.

Вместо этого он передаёт:

```json
{
  "peer_id": "peer-a",
  "overlay_ip": "100.80.0.12",
  "wg_public_key": "...",
  "transport": {
    "type": "tor-relay-only",
    "relay_handle": "relay-session-id",
    "relay_onion": "relayxxxxxxxxxxxxxxxx.onion"
  }
}
```

### 6.2. I2P mode

Signal server может передавать I2P destination:

```json
{
  "peer_id": "peer-a",
  "overlay_ip": "100.80.0.12",
  "wg_public_key": "...",
  "transport": {
    "type": "i2p",
    "i2p_destination": "base64-or-b32-destination"
  }
}
```

---

## 7. Data plane

### 7.1. WireGuard userspace

Anonymous mode должен использовать userspace WireGuard, потому что kernel WireGuard ожидает UDP endpoint.

Варианты:

- `wireguard-go`;
- custom userspace WG integration;
- netstack mode;
- TUN device + custom transport adapter.

### 7.2. Transport adapter

Нужен слой между WireGuard и Tor/I2P:

```text
TUN
 ↓
WireGuard userspace
 ↓
Anonymous transport adapter
 ├── Tor stream/channel
 ├── Tor stream/channel
 ├── I2P datagram/session
 └── Relay session
```

Задачи adapter:

- читать encrypted WG packets;
- доставлять их через Tor/I2P;
- принимать packets обратно;
- делать peer/session mapping;
- поддерживать keepalive;
- контролировать backpressure;
- не раскрывать IP endpoints.

---

## 8. Relay design

### 8.1. Зачем relay нужен в Tor MVP

Tor onion services хорошо подходят для inbound TCP, но VPN-трафик поверх Tor напрямую между onion services будет сложным и медленным. Для MVP проще сделать relay-only:

```text
Peer A → Tor → Relay Onion → Tor → Peer B
```

Relay должен:

- принимать anonymous sessions;
- связывать peer sessions по relay handle;
- пересылать encrypted WG packets;
- не расшифровывать payload;
- не знать real IP;
- не требовать clearnet connectivity.

### 8.2. Relay не должен быть trust anchor

Relay видит:

- session IDs;
- объём трафика;
- тайминги;
- fact of communication, если один relay обслуживает обе стороны.

Relay не видит:

- inner IP payload;
- TCP/UDP содержимое внутри VPN;
- WireGuard plaintext.

### 8.3. Relay protocol sketch

```text
Client → Relay:
  AUTH peer_id, session_token, wg_pubkey

Relay → Client:
  OK relay_session_id

Client → Relay:
  SEND dst_peer_id, packet_len, encrypted_wg_packet

Relay → Destination Client:
  RECV src_peer_id, packet_len, encrypted_wg_packet
```

Позже можно добавить:

- padding;
- batching;
- rate limits;
- per-network relay isolation;
- relay rotation;
- multiple relays;
- multipath relay sessions.

---

## 9. Multipath / несколько потоков

### 9.1. Зачем

Tor/I2P будут медленнее обычного WireGuard. Несколько потоков могут помочь при нескольких одновременных внутренних соединениях.

### 9.2. Не делать round-robin по каждому пакету

Плохо:

```text
WG packet #1 → stream 1
WG packet #2 → stream 2
WG packet #3 → stream 3
```

Это вызовет packet reordering и ухудшит TCP внутри VPN.

### 9.3. Правильный MVP-подход: flow affinity

```text
flow A → channel 1
flow B → channel 2
flow C → channel 3
```

Пример:

```text
10.0.0.2:50001 → 10.0.0.5:22   → stream 1
10.0.0.2:50002 → 10.0.0.5:443  → stream 2
10.0.0.2:50003 → 10.0.0.5:8080 → stream 3
```

### 9.4. Конфиг

```yaml
anonymous_transport:
  multipath:
    enabled: true
    strategy: flow_affinity
    min_channels: 1
    max_channels: 4
    close_idle_channel_after: 60s
```

Для Tor:

```yaml
tor:
  streams_per_peer: 2
  max_streams_per_peer: 4
  circuit_isolation: true
```

Для I2P:

```yaml
i2p:
  tunnel_length: 1
  tunnel_quantity: 3
  inbound_quantity: 3
  outbound_quantity: 3
```

---

## 10. DNS и внутренние имена

Пиры должны видеть только внутренние имена:

```text
laptop.alice.mesh
server.prod.mesh
db.internal.mesh
```

DNS должен резолвить только overlay IP:

```text
server.prod.mesh → 100.80.0.20
```

Нельзя раскрывать:

- real hostname;
- LAN hostname;
- real IP;
- public endpoint;
- local network ranges без явного разрешения.

### 10.1. MagicDNS-like режим

Нужно сохранить аналог NetBird DNS:

```yaml
dns:
  enabled: true
  zone: mesh.internal
  records:
    server-1.mesh.internal: 100.80.0.10
    laptop-1.mesh.internal: 100.80.0.11
```

---

## 11. ACL и routing

Так как нет выхода наружу, MVP должен поддерживать только:

- peer-to-peer overlay traffic;
- private service access;
- ACL by group/user/device;
- DNS внутри mesh.

Не нужно в MVP:

- exit nodes;
- internet gateway;
- subnet routing;
- site-to-site routing через LAN;
- NAT traversal;
- split tunnel to public internet.

### 11.1. Разрешённый MVP scope

```yaml
features:
  peer_to_peer_overlay: true
  internal_dns: true
  acl: true
  device_groups: true
  setup_keys: true
  exit_nodes: false
  subnet_routes: false
  lan_discovery: false
```

---

## 12. Изменения в кодовой базе NetBird

### 12.1. Client

Добавить:

- `anonymous_mode`;
- transport adapter interface;
- Tor transport backend;
- I2P transport backend;
- userspace WireGuard transport binding;
- leak prevention checks;
- bootstrap through onion/I2P;
- join token parser.

Изменить:

- peer connection manager;
- ICE/STUN logic;
- signal client;
- relay client;
- DNS metadata sanitization;
- logging.

Отключить в anonymous mode:

- direct UDP;
- STUN;
- ICE;
- local discovery;
- endpoint candidate publication.

---

### 12.2. Management server

Добавить:

- anonymous transport policy;
- validation that peers do not publish clearnet endpoints;
- transport descriptors instead of IP endpoints;
- setup key policy for anonymous networks;
- optional onion/I2P service deployment config.

Изменить:

- peer metadata model;
- network map generation;
- audit logs;
- device registration;
- policy validation.

---

### 12.3. Signal server

Добавить:

- relay handle exchange;
- I2P destination exchange;
- anonymous session negotiation;
- no-IP candidate schema.

Убрать/запретить:

- passing public UDP endpoints in anonymous networks;
- STUN-derived candidates;
- clearnet relay fallback.

---

### 12.4. Relay server

Добавить новый relay type:

```text
anonymous relay
```

Функции:

- onion/I2P listener;
- authenticated peer sessions;
- packet forwarding;
- per-network isolation;
- rate limiting;
- metrics without IP logging;
- optional padding/batching later.

---

## 13. Конфигурация

### 13.1. Клиентский конфиг

```yaml
mode: anonymous

management:
  url: "http://managementxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.onion"

enrollment:
  setup_key: "NB-SETUP-xxxx"

transport:
  type: "tor-relay-only"
  require_anonymous: true

tor:
  mode: "system"
  socks5: "127.0.0.1:9050"
  control: "127.0.0.1:9051"
  circuit_isolation: true

wireguard:
  mode: "userspace"

leak_prevention:
  disable_stun: true
  disable_ice: true
  disable_direct_udp: true
  disable_lan_discovery: true
```

### 13.2. Management config

```yaml
anonymous_networks:
  enabled: true
  require_anonymous_transport: true
  reject_clearnet_endpoints: true

server:
  public_url: "http://managementxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.onion"

logging:
  log_source_ip: false
  log_forwarded_for: false
  anonymize_audit_events: true
```

### 13.3. Relay config

```yaml
relay:
  mode: anonymous
  listen:
    tor_onion: true
    i2p: false

privacy:
  log_source_ip: false
  log_session_payload: false
  enable_padding: false

limits:
  max_sessions_per_peer: 8
  max_channels_per_peer_pair: 4
```

---

## 14. Leak prevention checklist

Клиент в anonymous mode должен падать с ошибкой, если:

- management URL не `.onion`/I2P;
- signal URL не `.onion`/I2P;
- relay URL не `.onion`/I2P;
- включён STUN;
- включён ICE;
- включён direct UDP;
- включён local peer discovery;
- включён clearnet fallback;
- внешний OIDC провайдер используется через clearnet;
- peer metadata содержит public/private real IP endpoint.

Пример ошибки:

```text
Anonymous mode violation:
direct UDP endpoint discovery is enabled.
Refusing to start because this may leak the real IP address.
```

---

## 15. MVP roadmap

### Phase 0 — исследование

- [x] Изучить NetBird client connection manager.
- [x] Изучить signal protocol.
- [x] Изучить relay implementation.
- [x] Найти все места, где появляются endpoint candidates.
- [x] Найти все места логирования source IP.
- [x] Составить leak map.

Deliverable:

```text
docs/leak-map.md
docs/netbird-transport-analysis.md
```

Статус: выполнено 2026-05-31.

---

### Phase 1 — Tor-only control plane

Цель: клиент подключается к management через `.onion`.

Задачи:

- [x] поддержка management URL `.onion`;
- [x] SOCKS5 transport для HTTP/gRPC;
- [x] отключение IP logging;
- [x] setup key enrollment через Tor;
- [x] запрет clearnet management в anonymous mode.

Deliverable:

```text
anonbird up --management-url http://management.onion --setup-key xxx
```

Статус: реализовано и подтверждено 2026-05-31 на выделенных серверах: management `.onion`, setup-key enrollment через Tor SOCKS5, без clearnet management fallback.

---

### Phase 2 — STUN/ICE/direct UDP kill switch

Цель: гарантировать отсутствие IP leak.

Задачи:

- [x] `anonymous_mode=true`;
- [x] отключить STUN;
- [x] отключить ICE;
- [x] отключить direct UDP;
- [x] запретить endpoint candidate publication;
- [x] добавить runtime validation;
- [x] добавить тесты на отсутствие IP в network map.

Deliverable:

```text
anonymous client starts and publishes zero clearnet endpoints
```

Статус: реализовано и покрыто unit/compile checks 2026-05-31; `anonbird debug anonymous-check` добавлен как runtime audit.

---

### Phase 3 — Anonymous relay MVP

Цель: data plane через Tor relay.

Задачи:

- [x] onion relay;
- [x] relay sessions;
- [x] packet forwarding;
- [x] userspace WG adapter;
- [x] peer mapping by overlay IP/public key;
- [x] keepalive;
- [x] reconnect;
- [x] basic rate limit.

Deliverable:

```text
Peer A can ping Peer B overlay IP through Tor relay
```

Статус: выполнено 2026-05-31. Existing NetBird relay client/server path теперь в anonymous mode использует SOCKS5 WebSocket-only dialer и userspace WG; `combined` поддерживает anonymous-конфиг без embedded STUN через явное `stunPorts: []`; onion management/relay listener подтверждён через Tor hidden service; реальный ping overlay IP через Tor relay подтверждён между тремя клиентами; добавлен server-side per-peer transport byte rate limit.

---

### Phase 4 — DNS + ACL

Цель: usable private mesh.

Задачи:

- [x] internal DNS;
- [x] ACL enforcement;
- [x] device groups;
- [x] service names;
- [x] management UI 표시 anonymous status;
- [x] no exit traffic.

Deliverable:

```text
ssh server-1.mesh.internal works through anonymous mesh
```

Статус: выполнено 2026-05-31 для MVP scope. Internal DNS/FQDN verified на тестовом стенде (`anonbird-*.anonbird.local` → overlay IPv6, ping by name OK), ACL/device groups используются через bootstrap All group + default All-to-All policy, dashboard показывает anonymous status и скрывает route/public metadata, anonymous client config запрещает subnet/exit routes, route table verified без default/subnet routes через `wt0`. Service names поддержаны через existing `--extra-dns-labels` и anonymous join-token параметры `service(s)`/`dns_label(s)`.

---

### Phase 5 — Multipath

Цель: улучшить скорость и устойчивость.

Задачи:

- multiple Tor streams;
- flow affinity;
- channel health checks;
- adaptive channel count;
- no packet-level round-robin.

Deliverable:

```text
multiple internal flows are distributed across channels
```

Статус: закрыто для MVP на code/unit + реальном Tor стенде 2026-05-31, включая дополнительный stream/health hardening. Сверка показала, что простой packet-level round-robin в `client/iface/wgproxy` был бы неправильным, потому что relay path видит encrypted WG packets и не может классифицировать внутренние TCP/UDP flows. Поэтому добавлен pre-WireGuard hint path: `FilteredDevice.Read` наблюдает plaintext overlay packets, `client/internal/anonymous/multipath` строит direction-neutral flow key для TCP/UDP/ICMP, а relay multipath `net.Conn` потребляет эти hints только для WireGuard data packets и выбирает channel через rendezvous hashing. Relay protocol/client/server теперь поддерживает `transport channel` frames с legacy channel 0. Follow-up hardening добавил `AuthChannel` handshake: relay server store держит несколько live sessions одного peer по channel ID, а anonymous `channel > 0` открывается отдельным relay client/WebSocket через SOCKS5/I2P dialer, то есть channel может стать отдельным Tor stream. Server route для channel-aware transport выбирает matching peer channel и fallback на channel 0 для early packets. Packet-level round-robin не реализуется намеренно. Anonymous relay healthcheck и management/signal gRPC keepalive тайминги hardened для hidden-service latency; per-channel health теперь опирается на отдельные relay-client healthchecks/read/write errors, marks failed channel unhealthy и продолжает через оставшиеся healthy channels. `WorkerRelay` пытается открыть до 4 anonymous channels, включает фактически поднятое число и fallback на primary, если extra streams недоступны. На стенде `93.177.116.58` + `185.246.220.249`/`45.138.103.224` соединение держалось стабильно, ping прошёл 0% loss, two-channel smoke дал `~2.32 Mbits/s` receiver против single-stream baseline `~1.86 Mbits/s` и прежнего relay-only `~1.45 Mbits/s`; после stream hardening server logs подтвердили `relay_channel` 0..3 для обоих peers, клиенты подняли `4/4` channels, `iperf3 -P 4 -t 25` прошёл без ошибок с server receiver `5.827 Mbits/s`.

---

### Phase 6 — I2P backend

Цель: datagram-oriented anonymous mesh backend.

Задачи:

- [x] I2P SAM integration for control/relay streams;
- [x] I2P SAM RAW/DATAGRAM session primitive;
- [x] I2P destination key generation primitive;
- [x] i2pd embedded/system lifecycle management;
- [x] I2P destination exchange/registration in management/signal;
- [x] I2P datagram peer transport binding to WireGuard/netstack;
- [x] tunnel quantity/length config;
- [x] like-for-like benchmark against Tor relay-only.

Deliverable:

```text
Peer A can ping Peer B through I2P transport
```

Статус: закрыто для direct I2P MVP 2026-05-31. Добавлен SAM client/dialer с настоящим протоколом и unit tests; `i2p-datagram` config теперь принимает `.b32.i2p` management/signal/relay endpoints и подключает management/signal gRPC + relay WebSocket через system SAM bridge. Добавлен SAM RAW/DATAGRAM primitive: session create, UDP send header, receive parser, size/protocol validation. Добавлен destination primitive: `DEST GENERATE SIGNATURE_TYPE=7`, persistent private destination key в local profile config и management exchange public destinations через `PeerSystemMeta`/`RemotePeerConfig`. Добавлен managed/system i2pd lifecycle (`external`/`auto`/`managed`) с настоящим запуском `i2pd`, per-profile config files, SAM readiness wait, graceful stop и systemd dependency на `i2pd.service` для Linux service install. Добавлен tunnel tuning config (`length`/`quantity`) через CLI/proto/profile/join/debug/dashboard и SAM session options. Direct peer data-plane подключён и validated на двух реальных клиентах: один локальный SAM `STYLE=DATAGRAM` session используется всеми peer connections, incoming datagrams демультиплексируются по source I2P destination, каждый peer получает `net.Conn` для существующего WireGuard proxy, retry loop + WG watcher reset предотвращают асимметрию direct/relay и stale Connected без handshake, а anonymous mode по-прежнему не создаёт ICE/STUN/direct UDP path. I2P-only combined healthcheck закрыт через localhost/self probe multiplexed relay handler. Like-for-like benchmark against Tor relay-only выполнен на той же клиентской паре: direct I2P дал меньшую latency (~399/364 ms против Tor ~1047/1092 ms), throughput оказался сравнимым и направленно вариативным (`720 Kbits/s`/`2.10 Mbits/s` receiver для I2P против `1.45 Mbits/s`/`1.18 Mbits/s` receiver для Tor).

---

## 16. Тестирование

### 16.1. Leak tests

Автоматические тесты должны проверять, что:

- network map не содержит real IP;
- signal messages не содержат real IP;
- relay messages не содержат real IP;
- logs не содержат source IP;
- клиент не делает UDP/STUN запросы;
- клиент не подключается к clearnet endpoints в anonymous mode.

### 16.2. Integration tests

Сценарии:

1. Management через onion.
2. Enrollment через setup key.
3. Два peer подключаются через Tor.
4. Ping overlay IP.
5. DNS resolve внутреннего имени.
6. SSH внутри mesh.
7. Отключение relay и reconnect.
8. Проверка, что direct UDP не появляется.
9. Проверка, что peer не узнаёт IP другого peer.
10. Проверка, что management не получает IP, кроме Tor/I2P source.

### 16.3. Adversarial tests

- Malicious peer пытается запросить endpoint — закрыто runtime validation/debug anonymous-check: ICE endpoint publication отсутствует, remote peer видит только overlay IP/anonymous relay/I2P destination.
- Malicious relay пытается прочитать payload — закрыто в MVP scope: relay пересылает WireGuard encrypted packets, flow hints строятся pre-WG локально и не отправляют plaintext inner flow relay server.
- Malicious management пытается включить clearnet fallback — закрыто anonymous validation: management/signal/relay/STUN/TURN/provider endpoints fail-closed при clearnet или transport mismatch.
- Client config accidentally enables STUN — закрыто config/runtime validation: anonymous mode отклоняет STUN/TURN/NAT external IP/direct UDP/ICE.
- External OIDC leaks IP — закрыто client-side provider guard и server-side OIDC/JWKS anonymous HTTP transport для `.onion`/`.b32.i2p`.
- Logs accidentally contain real IP — частично закрыто sanitization/redaction и стендовыми grep-проверками; remaining release-candidate task: longer log soak после разных reconnect/failure сценариев.

---

## 17. CLI

### 17.1. MVP команды

```bash
anonbird up \
  --management-url http://management.onion \
  --setup-key NB-SETUP-xxxx
```

```bash
anonbird status
```

```bash
anonbird down
```

```bash
anonbird debug anonymous-check
```

### 17.2. Проверка режима

```bash
anonbird debug anonymous-check
```

Пример вывода:

```text
Anonymous mode: enabled
Management transport: tor
Signal transport: tor
Relay transport: tor
STUN: disabled
ICE: disabled
Direct UDP: disabled
Clearnet fallback: disabled
Published endpoints: none
Result: OK
```

---

## 18. UI/Management dashboard

Добавить в UI:

- network mode: `anonymous`;
- transport: `tor-relay-only` / `i2p`; реализовано через `anonymous_transport` HTTP API field и dashboard peer details/tooltip;
- peer status without real IP;
- relay status; заметка: достоверный relay health остаётся client-side (`anonbird status`/`debug anonymous-check`), management dashboard не получает per-client relay probes;
- setup/install command selector for Tor/I2P transport and I2P SAM tunnel tuning;
- warning if non-anonymous feature enabled; реализовано в `Code/dashboard` на anonymous peer overview для routes и unsafe local flags;
- setup key generation for anonymous network; реализовано через in-modal generator и `anonbird join` invite command для готового ключа;
- device approval; existing peer approval UI/API используется без AnonBird-specific изменений.

Не показывать:

- real client IP;
- last seen from IP;
- endpoint candidates;
- LAN addresses.

---

## 19. Open questions

1. Использовать системный Tor/I2P или embedded daemon?
2. Делать ли persistent onion identity для каждого peer?
3. Нужны ли private onion services per peer или только relay sessions?
4. Какой формат relay session token?
5. Как избежать лишней корреляции трафика на relay?
6. Нужно ли padding/batching в MVP?
7. Какой минимальный performance target?
8. Как лучше интегрировать userspace WireGuard в существующий NetBird client?
9. Можно ли сохранить совместимость с upstream NetBird network map?
10. Как лицензировать форк и upstream patches?

---

## 20. Рекомендуемый MVP scope

Для первого рабочего релиза:

```text
Transport:
  Tor relay-only

Control plane:
  Management over onion
  Signal over onion

Data plane:
  WireGuard userspace over anonymous relay

Features:
  Setup keys
  Device identity
  Overlay IP
  Internal DNS
  ACL
  Groups
  No exit traffic
  No subnet routes
  No direct UDP
  No STUN
  No ICE
```

MVP считается успешным, если:

```text
1. Два клиента подключаются через .onion management.
2. Management не видит real IP клиентов.
3. Клиенты не видят real IP друг друга.
4. Peer A может ping/ssh Peer B по overlay IP/DNS.
5. В network map, signal messages и logs нет real IP.
6. tcpdump показывает отсутствие direct UDP/STUN/ICE.
```

---

## 21. Названия проекта

Возможные рабочие названия:

- AnonBird
- ShadowBird
- OnionBird
- VeilMesh
- HiddenMesh
- NebulaBird

---

## 22. Короткий итог

Форк NetBird под anonymous private mesh реалистичен, если не пытаться сохранить обычную direct peer-to-peer модель. Главный принцип MVP:

```text
no clearnet endpoints, no STUN, no ICE, relay required, management over onion
```

Самый быстрый путь:

```text
Tor relay-only → userspace WireGuard → internal DNS/ACL → multipath → I2P backend
```

Такой проект сможет дать простой NetBird-like UX:

```bash
anonbird up --management-url http://server.onion --setup-key xxx
```

но с другим security contract: реальные IPv4/IPv6 адреса не раскрываются пирам и control server, пока anonymous mode строго запрещает любые clearnet fallback-механизмы.
