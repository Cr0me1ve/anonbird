# План форка NetBird для anonymous mesh через Tor/I2P

## Журнал реализации

Дата: 2026-05-31

- Статус: MVP anonymous mesh реализован и прошёл requirement-by-requirement audit 2026-05-31; post-MVP production/open-source release gate остаётся открытым до полного release-candidate прогона.
- Текущий фокус: Phase 1-4, Phase 5 Tor relay stream multipath/per-channel health и Phase 6 direct-I2P MVP закрыты на code/unit + стендовом уровне; leak-map audit закрывает найденные clearnet side channels, reconnect/failure soak прошёл, dashboard/runtime/release packaging/proxy web/docs/infrastructure surface hardened. Сейчас идёт production-readiness слой: published artifacts/images, full release test suite, Marton Tor repeat/release-artifact repeat, NetBird->AnonBird migration server+clients, rollback/uninstall/reinstall и итоговый open-source release report.
- Правило выполнения: каждая реализованная часть отмечается здесь или в соответствующем чеклисте ниже; если в ходе сверки с ТЗ появляются ограничения или риски, они фиксируются в заметках.
- Сверка с новым ТЗ: четыре сервера пользователя для финального testbed зафиксированы в разделе 22.0; release/open-source readiness нельзя закрывать без полного remote прогона, Marton через виртуальную сеть, server/client migration с обычного NetBird и финального verdict, можно ли заменить NetBird на AnonBird без ручных исправлений.
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
- Реализовано: Linux service install/reconfigure теперь добавляет anonymous runtime dependencies по реальному transport/lifecycle profile: для `tor-relay-only` — `Wants/After=tor.service`, для `i2p-datagram` + `external` — `Wants/After=i2pd.service`, для `i2p-datagram` + `auto`/`managed` внешняя `i2pd.service` dependency не добавляется, чтобы AnonBird владел managed i2pd process.
- Реализовано: AnonBird daemon default socket переведён на `unix:///var/run/anonbird.sock` (CLI/UI/SSH helper/Docker env/daemon discovery), поэтому команды `anonbird status`/`debug` работают с установленным `anonbird.service` без ручного `--daemon-addr`.
- Реализовано в `Code/dashboard`: title AnonBird Dashboard, `anonymous_mode` peer flag, peer/table UI hides public IP/region/serial for anonymous peers, shows network mode/transport, hides network routes for anonymous peers, setup/SSH commands use `anonbird up --anonymous-mode --anonymous-transport tor-relay-only`.
- Реализовано в `Code/dashboard`: install modal получил transport selector Tor/I2P; I2P command generation добавляет `--i2p-sam`, `--i2p-tunnel-length`, `--i2p-tunnel-quantity`, Docker flow добавляет соответствующие `NB_I2P_*`/anonymous env vars.
- Реализовано в `Code/dashboard`: I2P install flow получил controls для i2pd lifecycle (`auto`/`managed`/`external`, path, data dir); CLI/Docker command generation добавляет `--i2p-daemon-mode`, `--i2pd-path`, optional `--i2p-data-dir` и соответствующие `NB_I2P_DAEMON_MODE`/`NB_I2PD_PATH`/`NB_I2P_DATA_DIR`.
- Реализовано в `Code/dashboard`: anonymous peer overview получил `Anonymous safety check` warning, если к peer/group назначены network/exit routes или включены несовместимые local flags (`disable_client_routes=false`, `disable_server_routes=false`, `block_lan_access=false`, `disable_firewall=true`, `lazy_connection_enabled=true`); warning не раскрывает real IP/endpoint metadata.
- Реализовано в `Code/dashboard`: install/setup-key flow теперь предпочитает короткий `anonbird join "anonbird://join?...` invite command с management URL, setup key, transport и I2P lifecycle/tunnel params; если setup key ещё placeholder или management URL не настроен, UI корректно остаётся на `anonbird up` fallback.
- Реализовано в `Code/dashboard`: audit activity rendering теперь redacts `location_*` meta для anonymous peer events; management `Peer.EventMeta` дополнительно помечает anonymous events через `anonymous_mode`/`anonymous_transport`, чтобы UI не показывал connection IP/location даже при fallback/dev tooltip.
- Реализовано: пользовательские CLI help/error/service strings переведены на AnonBird (`anonbird service ...`, `anonbird status/debug/capture/ssh`, service display name `AnonBird`, generated SSH config header/commands); desktop UI/PKCE login page/window titles/tooltips/debug/profile strings и базовые Windows/macOS/Linux UI package descriptors переведены на AnonBird; совместимые import path/protocol/file identifiers (`github.com/netbirdio/netbird`, `netbird-ssh`, legacy config/package paths) намеренно не переименованы без отдельной миграции упаковки и API.
- Реализовано в `Code/dashboard`: видимые product strings в setup/install, onboarding, invite/error, activity, SSH, reverse proxy, access tokens, settings и peer/network empty states переведены на AnonBird; full logo component больше не рендерит старый wordmark, protocol/CSS identifiers и внешние docs links оставлены как compatibility-layer до отдельной миграции docs/API.
- Реализовано в `Code/dashboard`: release/runtime external fetch hardening — dashboard больше не делает default GitHub latest-release check и не использует `https://pkgs.netbird.io/wasm/...` как fallback; добавлен локальный `public/wasm/anonbird-client.wasm`, собранный из текущего AnonBird WASM client, а release check включается только явно через `ANONBIRD_RELEASE_CHECK_ENABLED=true` + `ANONBIRD_RELEASE_CHECK_URL`. Reverse-proxy offline warnings больше не ведут на `status.netbird.io`/`support@netbird.io`.
- Реализовано в `Code/dashboard`: install/package surface hardening — Linux/macOS/Windows setup tabs больше не ведут на `pkgs.netbird.io`, `netbirdio/tap` или `netbird-ui`; команды строят AnonBird из `ANONBIRD_SOURCE_URL` и устанавливают `anonbird`, Docker tab собирает локальный `ANONBIRD_DOCKER_IMAGE` из исходников вместо `netbirdio/netbird:latest`.
- Реализовано в `Code/dashboard`: release image publishing path hardened — package metadata renamed to `anonbird-dashboard`, GHCR workflow has explicit `packages: write`, does not push images on PRs, publishes semver/ref tags and `latest` on release tags for `ghcr.io/cr0me1ve/anonbird-dashboard`, and Docker docs use `:latest` to match `getting-started.sh` defaults. Проверено: `actionlint .github/workflows/build_and_push.yml`, YAML parse, `npm run build`, `git diff --check`.
- Реализовано в `Code/dashboard`: setup/install modal теперь показывает явное blocking unsafe warning для clearnet management config и прямо предупреждает про раскрытие real IP, NAT endpoint и local network metadata; команды при unsafe config остаются заблокированы placeholder path, без silent non-anonymous setup.
- Реализовано: management HTTP peer API теперь отдаёт `anonymous_transport` только для anonymous peers и продолжает скрывать `connection_ip`; dashboard peer details/tooltip показывают реальный anonymous transport (`tor-relay-only` или `i2p-datagram`) вместо общего `anonymous relay`.
- Реализовано: добавлен production operations doc `docs/anonbird-i2p-operations.md` с i2pd 2.60+ guidance, daemon modes, managed data layout, systemd dependencies, health/recovery commands и security checks для SAM/I2P.
- Исправлено: anonymous profile теперь останавливает/не создаёт client update manager, GUI event subscription не запускает `https://pkgs.netbird.io/releases/latest/version`, а daemon `GetFeatures` помечает update settings disabled для anonymous profile; это закрывает clearnet external-update check из leak-map.
- Исправлено: anonymous/onion/I2P combined management больше не запускает server-side `https://pkgs.netbird.io/releases/latest/version` check — добавлен `DisableVersionCheck` в management server config, standalone management flag `--disable-version-check`, combined YAML `server.disableVersionCheck` и автоотключение при `.onion`/`.i2p` `server.exposedAddress`.
- Исправлено: anonymous client больше не запускает NAT port mapper; `Engine.Start` пропускает PCP/NAT-PMP/UPnP discovery в anonymous mode и логирует `NAT port mapper is disabled in anonymous mode`, чтобы локальный gateway discovery не становился UDP leak.
- Исправлено: leak-map audit hardening — relay client больше не использует `serverIP` direct shortcut при SOCKS5/I2P anonymous relay dialers даже если caller ошибочно передал IP, а `UpdateOldManagementURL` пропускает cloud-management migration probe в anonymous mode, чтобы legacy URL helper не мог сделать clearnet healthcheck.
- Исправлено: management API/event redaction для anonymous peers теперь backend-side скрывает не только `connection_ip`, но и сохранённые `city/country/geoname` и hardware serial в peer single/list/accessible-peer responses и activity `EventMeta`, чтобы UI не был единственным барьером.
- Исправлено: self-hosted external server checks leak — combined автоматически отключает push на `https://metrics.netbird.io`, geolite download с `pkgs.netbird.io` и version update checks, когда `server.exposedAddress` использует `.onion`/`.i2p`; standalone management автоматически отключает anonymous metrics, geolite updates и version update checks, если auth/oidc/device-flow endpoints указывают на `.onion`/`.i2p`.
- Исправлено: release/update runtime defaults больше не указывают на upstream NetBird hosts — client/management version checks opt-in через `ANONBIRD_VERSION_URL`, `ANONBIRD_MANAGEMENT_VERSION_URL`, `ANONBIRD_DASHBOARD_RELEASE_URL`; desktop project/download links используют AnonBird fork URLs или `ANONBIRD_PROJECT_URL`/`ANONBIRD_DOWNLOAD_URL`; installer auto-update берёт artifacts/signing keys только из `ANONBIRD_RELEASE_BASE_URL`/`ANONBIRD_SIGNING_KEYS_BASE_URL`, а production signing key host пустой и fails closed.
- Исправлено: debug upload/geolocation/proxy runtime defaults hardened — `ANONBIRD_DEBUG_UPLOAD_URL` обязателен для default debug uploads, management/proxy GeoLite downloads требуют явные `ANONBIRD_GEOLITE_*`/`ANONBIRD_PROXY_GEOLITE_*` URLs или pre-seeded DB files, standalone proxy больше не defaults to `api.netbird.io` и требует `--mgmt`/`NB_PROXY_MANAGEMENT_ADDRESS`.
- Реализовано: добавлен release operations doc `docs/anonbird-release-hardening.md` с env matrix, fail-closed behavior и targeted release-candidate audit command для runtime/update endpoints.
- Исправлено: release packaging/install surface больше не defaults to upstream NetBird package/CDN hosts — `release_files/install.sh` использует fork GitHub release API/base URL или явные `ANONBIRD_*` override, Linux package managers ставят fork release binaries без подключения `pkgs.netbird.io` repos, Homebrew/macOS package paths требуют явные AnonBird URLs/formula, DNS fallback на `8.8.8.8` удалён.
- Реализовано: GoReleaser/Linux/macOS/systemd packaging переведён на AnonBird artifacts и commands — binaries/packages называются `anonbird`, `anonbird-ui`, `anonbird-*` server tools; Docker images публикуются как `cr0me1ve/anonbird*`/`ghcr.io/cr0me1ve/anonbird*`; service units, release hooks, desktop entry и installed paths используют `/etc`, `/var/log`, `/var/lib`, `/var/run` AnonBird namespaces.
- Реализовано: proxy web embedded UI rebrand/hardening — assets/components переименованы на AnonBird, visible title/powered-by/docs links ведут на AnonBird fork, embedded asset prefix изменён на `/__anonbird__/`, `dist` пересобран.
- Исправлено: server/package runtime defaults согласованы с AnonBird paths — management defaults используют `/etc/anonbird`, `/var/lib/anonbird`, `/var/log/anonbird`, combined server и upload server defaults используют `/var/lib/anonbird`; legacy `/etc|/var/lib|/var/log/netbird` и Wiretrustee paths сохранены только как migration probes, чтобы существующие установки можно было перенести.
- Исправлено: docs/infrastructure visible surfaces больше не ведут на upstream cloud/package/docs hosts — root README, management/signal/proxy READMEs, metrics notes, quickstart/configure/migration scripts, compose templates и TURN template переведены на AnonBird wording, fork release URLs/images, `/etc|/var/lib/anonbird`, `anonbird.selfhosted`, `anonbird-*` generated container names и AnonBird proxy docs/defaults. `NETBIRD_*`/`NB_*` env names оставлены как compatibility config contract.
- Исправлено: proxy observability/k8s naming surface — runtime metric descriptions говорят `AnonBird proxy`, а Kubernetes Lease test fixture больше не использует старый `netbird.io/domain` label prefix.
- Исправлено: final completion-audit sweep по visible/runtime old-upstream surfaces — `SECURITY.md`/`CODE_OF_CONDUCT.md`/`CONTRIBUTING.md`, combined CLI help, client default management/admin URLs, updater temp-dir/log strings, DNS warning docs link, runtime Kubernetes ACME lease label, dashboard docs/help links, local announcements, reverse-proxy install command, dashboard Docker/CI image names и setup Docker build/volume переведены на AnonBird/fork/self-hosted defaults. Старые upstream host strings в focused runtime/dashboard sweep больше не находятся; оставшиеся `github.com/netbirdio/netbird` import/module paths, `NB_*`/`NETBIRD_*` env names и legacy migration probes считаются compatibility contract.
- Исправлено: `.github` workflow/template surface больше не ведёт на upstream docs/forum/package CDN/shared-actions — issue/contact/PR/docs ack links переведены на fork, docs update workflow валидирует bundled OpenAPI локально, release workflow использует official Wintun/Mesa/NSIS artifact URLs с SHA256 вместо `pkgs.netbird.io`, RPM verification импортирует public key из AnonBird signing key, Discourse release posting стал opt-in через `ANONBIRD_DISCOURSE_*`.
- Исправлено: legacy `AdminURL` migration — старые клиентские config-файлы с hosted admin URL автоматически переводятся на self-hosted `http://localhost:33071` при следующем config apply/write; это закрывает leftover old-upstream URL в существующих anonymous client configs без ручной правки профиля.
- Исправлено: anonymous LAN/firewall log redaction — LAN-block и nftables route filtering больше не пишут реальные локальные prefixes/destinations в логи anonymous profile; вместо этого логируются count/redacted/source_count. Legacy AdminURL migration log также не печатает старый hosted URL.
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
- Проверки 2026-05-31: Tor short soak-smoke после leak-map guard deploy — 5 циклов `status` + `ping -c 3` в обе стороны на `185.246.220.249` и `45.138.103.224`; оба клиента все циклы держали `Management/Signal Connected`, relay `Available`, peer pair `Relayed`; ping avg по циклам: `185→45` `967/940/912/841/846 ms`, `45→185` `918/892/923/880/832 ms`; post-soak journal grep не нашёл новых `pkgs.netbird`/`ingest.netbird`/`metrics.netbird`/`upload.debug`/NAT discovery/STUN/ICE строк.
- Проверки 2026-05-31: Tor reconnect/failure soak на свежем client binary `c34d341e2` — `/usr/local/bin/anonbird` обновлён на `185.246.220.249` и `45.138.103.224`; прогон включал restart `anonbird-combined.service` на `93.177.116.58`, restart `tor.service` на `185.246.220.249`, restart `anonbird.service` на `45.138.103.224`, затем 8 циклов `status` + `ping -c 3` в обе стороны. Во всех 10 проверочных точках на направление target pair держала `Management/Signal Connected`, relay `1/1 Available`, ping `3/3` без loss; avg RTT диапазон `185→45` `961..1144 ms`, `45→185` `940..1302 ms`. Финальный `anonymous-check` на обоих клиентах OK, `/health` на `93` после soak `healthy`; journal grep с маркеров на `185`, `45`, `93` пустой для `pkgs.netbird`/`ingest.netbird`/`metrics.netbird`/`upload.debug`/version fetch/NAT discovery/STUN/ICE/relayServerIP/serverIP shortcut. В status остаётся старый inactive peer record, поэтому общий счётчик `Peers count: 1/2 Connected`, но проверяемая пара `185↔45` стабильно connected.
- Проверки 2026-05-31: desktop UI/PKCE rebrand pass — `go test ./client/internal/templates ./client/cmd ./client/ui/... -count=1 -timeout 180s` OK; `git diff --check` в `/Users/kirill/Code/netbird` OK. Остаточные `NetBird`/`netbird` совпадения в этом scope относятся к compatibility identifiers: legacy Windows config path migration, event names, executable/asset filenames, user-agent/version function names.
- Проверки 2026-05-31: dashboard external runtime fetch hardening — `GOOS=js GOARCH=wasm go build -trimpath -o public/wasm/anonbird-client.wasm ./client/wasm/cmd` из `/Users/kirill/Code/netbird` OK, SHA256 `0f268da8983d486ebdc350fd868364a397de359234682899b250296453434d73`; `npm run build` в `/Users/kirill/Code/dashboard` OK; `git diff --check` OK; targeted rg в `src/config.json/out` не нашёл `api.github.com`, `repos/netbirdio/netbird`, `pkgs.netbird.io/wasm`, `support@netbird`, `status.netbird`, old Netbird update/delete copy.
- Проверки 2026-05-31: dashboard repeat sanity — `npm run build` в `/Users/kirill/Code/dashboard` OK, `git diff --check` OK.
- Проверки 2026-05-31: dashboard install/package hardening — `npm run build` в `/Users/kirill/Code/dashboard` OK; `git diff --check` OK; targeted rg в tracked `src`/`config.json` не нашёл `netbirdio/tap`, `netbirdio/netbird`, `netbird-ui`, `install.sh`, `api.github.com`, `repos/netbirdio/netbird`, `support@netbird`, `status.netbird`; единственный `pkgs.netbird.io` в этом scope — deliberate sanitizer в `src/utils/config.ts`.
- Проверки 2026-05-31: release/update runtime hardening — targeted rg в `version`, client updater/debug/jobexec/android/server/cmd, `management/server/instance`, `management/server/geolocation`, `management/internals/server`, `proxy/cmd`, `proxy/internal/geolocation`, `upload-server` не нашёл runtime `pkgs.netbird.io`, `api.netbird.io`, `app.netbird.io`, `upload.debug.netbird.io`, `publickeys.netbird.io`, `github.com/netbirdio/netbird/releases`, `api.github.com/repos/netbirdio` или `netbirdio/tap`; `go test ./version ./client/internal/updater ./client/server ./client/cmd ./client/internal/debug ./client/jobexec ./management/server/instance ./management/server/geolocation ./management/internals/server ./proxy/cmd/proxy/cmd ./proxy/internal/geolocation ./upload-server/types -count=1 -timeout 240s` OK; `git diff --check` OK.
- Проверки 2026-05-31: release packaging/proxy web hardening — `npm run build` в `proxy/web` OK; `sh -n` для install/release/freebsd/macOS hook scripts OK; YAML parse `.goreleaser*.yaml` OK; compile-only `go test -run '^$' ./client ./combined ./management ./signal ./relay ./upload-server ./proxy/cmd/proxy ./proxy/web -count=1 -timeout 240s` OK; focused rg в release/goreleaser/proxy/Docker packaging scope не нашёл upstream runtime/download/package hosts, оставшиеся `github.com/netbirdio/netbird/version.version` — compatibility ldflags из текущего Go module path, не сетевые endpoints.
- Проверки 2026-05-31: docs/infrastructure hardening — `bash -n infrastructure_files/configure.sh infrastructure_files/getting-started.sh infrastructure_files/getting-started-with-dex.sh infrastructure_files/getting-started-with-zitadel.sh infrastructure_files/migrate.sh` OK; `go test ./management/cmd ./combined/cmd ./upload-server/server ./proxy/cmd/proxy/cmd ./proxy/internal/k8s ./proxy/internal/metrics -count=1 -timeout 240s` OK; `envsubst` + `docker compose config` для `infrastructure_files/docker-compose.yml.tmpl` и `.traefik` OK; focused rg в README/management/signal/proxy/metrics/infrastructure scope не нашёл upstream cloud/package/docs hosts или old runtime paths вне legacy migration probes для existing NetBird installs.
- Проверки 2026-05-31: final completion-audit surface sweep — `go test ./combined/cmd ./client/internal/updater/installer ./proxy/internal/acme ./proxy/internal/auth ./client/internal/profilemanager ./client/internal/metrics ./shared/auth/jwt ./client/server ./shared/management/client/rest -count=1 -timeout 240s` OK; compile-only `go test -run '^$' ./client/internal/dns -count=1 -timeout 120s` OK; dashboard `npm run build` OK; `git diff --check` OK в `/Users/kirill/Code/netbird` и `/Users/kirill/Code/dashboard`; focused rg больше не находит upstream docs/cloud/package/support/runtime hosts в changed runtime/dashboard/docs surface. Полный `go test ./client/internal/dns` на macOS без TUN/root privileges ожидаемо не runnable (`operation not permitted`, mDNSResponder/scutil assertions), поэтому для этого пакета использован compile-only sweep.
- Проверки 2026-05-31: legacy AdminURL migration — `go test ./client/internal/profilemanager -run 'TestLegacyAdminURLMigratesToAnonBirdDefault|TestNewProfileDefaults|TestUpdateOldManagementURL|TestAnonymousMode' -count=1 -timeout 120s` OK; новый linux/amd64 client binary задеплоен на `185.246.220.249` и `45.138.103.224`, после restart оба `anonymous-check` OK, `AdminURL.Host=localhost:33071`, Tor management/signal/relay connected, STUN/ICE/direct UDP/fallback disabled, published endpoints none.
- Проверки 2026-05-31: финальный Tor remote smoke после свежего client deploy — `93.177.116.58` и `213.108.3.228` health `healthy`; ping `185.246.220.249` (`100.79.204.47`) ↔ `45.138.103.224` (`100.79.143.119`) прошёл 6/6 в обе стороны после restart, avg RTT `~962 ms` и `~898 ms`; refined journal grep за последние 5 минут не нашёл `app.netbird`, package/metrics/debug hosts, STUN/ICE/NAT discovery/candidate/relayServerIP/serverIP shortcut patterns.
- Проверки 2026-05-31: anonymous log redaction fix — `go test ./client/internal/profilemanager ./client/internal -run 'TestLegacyAdminURLMigratesToAnonBirdDefault|TestShouldStartPortForwardManager|TestGetInterfacePrefixesExcludesNamedInterface|TestCompareNetIPLists' -count=1 -timeout 120s` OK; `GOOS=linux GOARCH=amd64 go test -c -o /tmp/nftables.test ./client/firewall/nftables` OK; `git diff --check` OK. Новый linux/amd64 client binary с redaction fix задеплоен на `185.246.220.249` и `45.138.103.224`.
- Проверки 2026-05-31: `.github` workflow/template hardening — YAML parse для release/windows/forum/docs workflows и issue templates OK; `git diff --check` OK; focused rg по `.github`/runtime surface больше не находит `docs.netbird.io`, `pkgs.netbird.io`, `forum.netbird.io`, `netbirdio/docs`, `netbirdio/dashboard`, `netbirdio/shared-actions`, old NetBird team/copyright strings или old package/docs/CDN hosts.
- Проверки 2026-05-31: финальный I2P remote smoke после redaction deploy — `213.108.3.228` `anonbird-i2p-combined.service` active и `/health` `healthy`; `anonbird debug anonymous-check` на `185.246.220.249` и `45.138.103.224` OK (`i2p-datagram`, management/signal/relay `i2p`, STUN/ICE/direct UDP/fallback disabled, published endpoints none, I2P destination registered, SAM reachable). Status показывает peer pair `P2P`, ICE candidate `i2p-datagram/i2p-datagram`, relay `.b32.i2p` available; ping `185→45` 8/8 avg `471.286 ms`, `45→185` 8/8 avg `489.559 ms`.
- Проверки 2026-05-31: финальный I2P adversarial leak sweep — client log grep после fresh restart на `185.246.220.249` и `45.138.103.224` не нашёл `app.netbird`, package/metrics/debug hosts, STUN/ICE/NAT discovery/candidate/relayServerIP/serverIP shortcut или реальные серверные/client IP prefixes; redaction evidence в логах: `blocking route LAN access for N local networks (redacted in anonymous mode)` и nftables `source_count=...`. Server journal grep на `213.108.3.228` чистый по тем же patterns; sqlite `peers.location_connection_ip` пустой, `meta_network_addresses=[]`, `meta_anonymous_transport=i2p-datagram` для обоих peers.
- Проверки 2026-05-31: final requirement-by-requirement completion audit — в плане не осталось незакрытых чекбоксов или рабочих пунктов по текущему MVP scope; Phase 0-6 имеют `Статус: выполнено/закрыто`; устаревших маркеров незавершённого remote smoke/goal-blocker больше нет; focused rg в netbird и dashboard не находит old upstream docs/cloud/package/forum/shared-actions hosts вне явно исключённых исторических audit docs; `git status --short` чистый в `/Users/kirill/Code/netbird` и `/Users/kirill/Code/dashboard` после push.
- Проверки 2026-05-31: post-MVP production readiness start — `infrastructure_files/getting-started.sh` получил non-interactive one-command режим (`--domain`, `--email`, `--yes`, `--render-only`, proxy options), README обновлён open-source quickstart инструкцией. Проверено: `bash -n`, `--help`, `--render-only`, `docker compose config` для сгенерированного self-host stack, fail-fast без `--email` в non-interactive Traefik mode.
- Проверки 2026-05-31: self-host quickstart hardening — `getting-started.sh` получил `--preflight-only`, `--skip-image-preflight`, `ANONBIRD_DASHBOARD_IMAGE`/`ANONBIRD_SERVER_IMAGE`/`ANONBIRD_PROXY_IMAGE` overrides и fail-fast Docker image preflight перед `docker compose up`; README/usage больше не показывают запрещённый placeholder `anonbird.example.com` и документируют image override для RC/private registry тестов. Проверено: `bash -n`, `shellcheck -S error`, `--help`, `--render-only` + `docker compose config`, preflight failure для missing images и skip path.
- Проверки 2026-05-31: self-host server/dashboard RC smoke на `93.177.116.58` через real one-command path — локально на сервере собраны RC images `anonbird-server:rc-local` из combined binary и `anonbird-dashboard:rc-local` из текущего dashboard `out/`; `getting-started.sh --domain anonbird.93.177.116.58.sslip.io --email admin@example.com --yes` с image overrides поднял Traefik/dashboard/combined server без ручной правки generated files. Dashboard `/` вернул `200`, OIDC discovery `/oauth2/.well-known/openid-configuration` вернул `200`, unauthenticated `/api/users` вернул expected `401`, `setup-key bootstrap --config /etc/anonbird/config.yaml` создал setup key с redacted output. Первый прогон выявил, что generated config оставлял management version check/geolocation downloads включёнными; исправлено в quickstart/migration config generation и dashboard init log. Повторный прогон подтвердил `disableVersionCheck: true`, `disableGeoliteUpdate: true`, `disableAnonymousMetrics: true`, `NB_DISABLE_GEOLOCATION=true`, логи `management version update check disabled`, `geolocation service is disabled`, `AnonBird latest version`, а grep по `outdated|github.com/netbirdio|pkgs.netbird|GeoLite|file will be downloaded|NetBird latest` пустой. После smoke контейнеры и volumes удалены через `docker compose down --volumes`.
- Проверки 2026-05-31: migration script release-image hardening — `infrastructure_files/migrate.sh` получил `ANONBIRD_DASHBOARD_IMAGE`/`ANONBIRD_SERVER_IMAGE`, `--dashboard-image`, `--server-image`, `ANONBIRD_SKIP_IMAGE_PREFLIGHT`/`--skip-image-preflight` и fail-fast image preflight перед apply; dry-run summary теперь показывает target images. Дополнительно исправлен portable domain parsing для `https://...` (`sed -E '^https?://'` вместо BSD-incompatible `https\?://`). Проверено: `bash -n`, `shellcheck -S error`, `--help`, fake embedded-IdP Caddy dry-run с custom RC images и корректным domain `anonbird.test.example`.
- Проверки 2026-05-31: real server migration E2E на `93.177.116.58` — поднят обычный upstream NetBird `v0.64.6` 5-container stack (`netbirdio/management:0.64.6`, `signal:0.64.6`, `relay:0.64.6`, `dashboard:latest`, Caddy) на `https://netbird.93.177.116.58.nip.io`; baseline dashboard `/` `200`, OIDC `200`, unauth API `/api/users` `401`. `migrate.sh --dry-run` корректно определил embedded Caddy, embedded IdP, sqlite, domain и management volume; `--apply` с RC-local AnonBird images сделал backup, остановил старые контейнеры, поднял Traefik + `anonbird-dashboard` + `anonbird-server`, verification `3/3`, dashboard/OIDC/API снова `200/200/401`, generated config содержит `disableAnonymousMetrics`, `disableGeoliteUpdate`, `disableVersionCheck`, `NB_DISABLE_GEOLOCATION=true`, setup-key bootstrap работает с redacted output, focused leak grep по upstream/version/geolite patterns clean. Найден и исправлен production blocker: rollback backup теперь сохраняет management Docker volume snapshot, а rollback script восстанавливает volume перед запуском старого compose. Повторный E2E подтвердил backup tar `33M`; после AnonBird DB write rollback вернул старые `netbird-*` контейнеры, удалил `config.yaml`, восстановил old volume, и после warmup старый NetBird снова отвечал `/` `200`, OIDC `200`, `/api/users` `401`. Тестовый стек после проверки остановлен через `docker compose down --volumes`. Открыто: baseline clients + `anonbird migrate client` E2E и replace-in-place test с реальным проектом.
- Проверки 2026-05-31: production path cleanup — default client/service paths переключены на `/etc/anonbird`, `/var/lib/anonbird`, `/var/log/anonbird`, `ProgramData\AnonBird`, `/var/db/anonbird`; legacy NetBird paths оставлены как source для миграции. Проверено: `gofmt`, `go test ./client/configs ./client/internal/profilemanager ./client/cmd -count=1 -timeout 240s`, `git diff --check`.
- Проверки 2026-05-31: migration CLI first pass — добавлен `anonbird migrate client|server|rollback`: client path делает dry-run/apply/backup/rollback для `/etc/netbird`, `/var/lib/netbird`, `/var/log/netbird` и systemd unit rewrite; server path запускает packaged `infrastructure_files/migrate.sh`, которому добавлен `--dry-run`; README получил migration раздел. Проверено: `go test ./client/cmd -run 'Test.*Migration|TestInitCommands' -count=1`, `go test ./client/configs ./client/internal/profilemanager ./client/cmd -count=1 -timeout 240s`, `go run ./client migrate --help`, `go run ./client migrate client --root /tmp/anonbird-missing-root --dry-run`, fake-root `migrate client --apply` + `migrate rollback --apply`, `bash -n infrastructure_files/migrate.sh`, `bash infrastructure_files/migrate.sh --help`.
- Проверки 2026-05-31: client migration anonymous safety hardening — `anonbird migrate client --apply` теперь отказывается переносить legacy NetBird config с non-anonymous `ManagementURL`, если нет `--rejoin "anonbird://join?..."` или явного unsafe подтверждения `--allow-unsafe-clearnet --yes-i-understand-this-may-leak-my-ip`; при `--rejoin` migrated config переписывается на anonymous management URL/transport и `DisableAutoConnect=true` до запуска сервиса, чтобы старый clearnet profile не успел подключиться. README migration section обновлён. Проверено: targeted `go test ./client/cmd -run 'Test.*Migration|TestInitCommands|TestApplyClientMigration' -count=1` и CLI fake-root dry-run/apply без rejoin/apply с rejoin.
- Проверки 2026-05-31: live-root client migration E2E на `93.177.116.58` — найден настоящий старый `netbird.service` (`/usr/bin/netbird`, `/etc/netbird`, `/var/lib/netbird/default.json`) с legacy `api.netbird.io:443`/`app.netbird.io:443` non-anonymous profile. Перед apply добавлены production hardening правки: systemd `EnvironmentFile` и `/etc/sysconfig|/etc/default/netbird` переписываются на AnonBird, migrated URL пишется в Go-compatible `url.URL` JSON format, helper files `/etc/anonbird/management-url` и `/etc/anonbird/setup-key` обновляются из `--rejoin`, setup-key получает mode `0600`, rollback manifest сохраняет исходные active/enabled state systemd service. Live apply с onion `--rejoin` остановил `netbird.service`, создал `anonbird.service`, переписал config/helper files и подключил daemon: management/signal/relay через Tor `.onion`, `anonymous-check` OK, STUN/ICE/direct UDP/fallback disabled, post-marker grep не нашёл `api.netbird.io`, `app.netbird.io`, `pkgs.netbird`, STUN/ICE/NAT discovery patterns. Rollback dry-run/apply затем удалил `/etc/anonbird`, `/var/lib/anonbird`, `/var/log/anonbird`, `anonbird.service`, восстановил `netbird.service` active/enabled и старый legacy profile. Открыто: двухклиентный upstream NetBird baseline connectivity + migration на client pair и application/DNS/ACL verification.
- Проверки 2026-05-31: dashboard anonymous setup hardening — `/Users/kirill/Code/dashboard` commit `4a59b20` больше не подставляет clearnet `NETBIRD_MGMT_GRPC_API_ENDPOINT` в peer setup команды; только `.onion`/`.i2p` URL попадают в `anonbird join`, `anonbird up`, Docker env и mobile/manual management steps. При clearnet endpoint dashboard показывает blocking warning и команды получают placeholder `ANONYMOUS_MANAGEMENT_URL_REQUIRED`. Проверено: `npx prettier --write ...`, `npx tsc --noEmit`, `npm run build`, Browser `/install` smoke с onion local config.
- Проверки 2026-05-31: release testbed preflight после post-MVP изменений — SSH доступен ко всем четырём серверам `93.177.116.58`, `213.108.3.228`, `185.246.220.249`, `45.138.103.224`; server services `anonbird-combined.service` и `anonbird-i2p-combined.service` active; client services на `185` и `45` active, `Management/Signal Connected`, relays available. `anonbird debug anonymous-check` на обоих клиентах OK (`i2p-datagram`, management/signal/relay `i2p`, STUN/ICE/direct UDP/fallback disabled, published endpoints none). Overlay ping `185→45` `6/6` avg `522.747 ms`, `45→185` `6/6` avg `514.316 ms`.
- Проверки 2026-05-31: CLI anonymous-by-default hardening — `--anonymous-mode` теперь default `true`; `up/login/set-config` применяют anonymous mode и Tor relay-only transport даже без явного флага; добавлены `--no-anonymous-mode`, `--allow-unsafe-clearnet`, `--yes-i-understand-this-may-leak-my-ip`. Unsafe clearnet режим печатает warning про real IP/NAT/local metadata и без подтверждения завершается ошибкой. Проверено: `go test ./client/configs ./client/internal/profilemanager ./client/cmd -count=1 -timeout 240s`, `go run ./client up --help` показывает новые flags/default, `go run ./client --no-anonymous-mode up` fail-fast с warning и required confirmation flags, `git diff --check`.
- Проверки 2026-05-31: Tor benchmark preflight — SSH к `93.177.116.58` восстановился, Tor onion hostname `o2n24n6pjl4dkz2i3tlyfov3ozpnwcu4bhy26rtd65stqctn6rg3vpad.onion` доступен в конфиге, remote combined всё ещё старой сборки с `/health` 503 из-за onion DNS self-probe, но management/setup-key path через onion работоспособен.
- Проверки 2026-05-31: Tor relay-only benchmark восстановлен на `93.177.116.58` + `185.246.220.249`/`45.138.103.224`; Tor management/signal/relay идут через `http://o2n24n6pjl4dkz2i3tlyfov3ozpnwcu4bhy26rtd65stqctn6rg3vpad.onion:80`, клиенты подключены `Relayed`, `anonymous-check` OK, STUN/ICE/direct UDP/clearnet fallback отсутствуют.
- Проверки 2026-05-31: Tor relay-only ping между `185.246.220.249` (`100.79.204.47`) и `45.138.103.224` (`100.79.143.119`) — 10/10 packets в обе стороны, avg RTT `~1047 ms` и `~1092 ms`.
- Проверки 2026-05-31: Tor relay-only `iperf3` 20s: `185→45` sender `5.75 MiB / 2.41 Mbits/s`, receiver `4.50 MiB / 1.45 Mbits/s`; `45→185` sender `5.38 MiB / 2.25 Mbits/s`, receiver `3.38 MiB / 1.18 Mbits/s`; retransmits `0`.
- Заметка: при восстановлении Tor benchmark существующие peer-записи на `93.177.116.58` были созданы старым setup key без `allow_extra_dns_labels`; так как этот флаг хранится в peer, а не обновляется новым setup key при login, для тестового account `anonbird` выставлен `allow_extra_dns_labels=1` существующим peers. Это изменение только стендовое, не кодовая заглушка.
- Ограничение: Ubuntu/Noble package `i2pd 2.49.0` падал на static server tunnels; на тестовом I2P management server использован PurpleI2P PPA `i2pd 2.60.0`, после чего tunnel стабилен. Production guidance зафиксирован в `docs/anonbird-i2p-operations.md`: использовать 2.60+ или предварительно проверять packaged i2pd.
- Заметка: на `45.138.103.224` i2pd после нескольких restart/нагрузок временно начал отдавать SAM DIAL timeout; `systemctl restart i2pd` восстановил STREAM/DATAGRAM. Это стендовый operational риск, отдельный от AnonBird data-plane; recovery guidance добавлен в `docs/anonbird-i2p-operations.md`.
- Сверка с ТЗ: like-for-like benchmark закрыт на одной клиентской паре `185.246.220.249` ↔ `45.138.103.224`. Для direct I2P ранее зафиксировано `ping avg ~399/364 ms`, throughput `185→45` receiver `720 Kbits/s`, `45→185` receiver `2.10 Mbits/s`; для Tor relay-only зафиксировано `ping avg ~1047/1092 ms`, throughput `185→45` receiver `1.45 Mbits/s`, `45→185` receiver `1.18 Mbits/s`. Tor latency ожидаемо выше, throughput сравним/вариативен из-за TCP-over-Tor relay и server relayRateLimit.
- Сверка с ТЗ: Phase 5 runtime multipath benchmark закрыт на той же Tor клиентской паре. Первый runtime pass с двумя логическими relay channels дал modest throughput uplift на multi-flow smoke (`~2.32 Mbits/s` receiver против single-stream baseline `~1.86 Mbits/s` и прежнего relay-only `~1.45 Mbits/s` в том же направлении). Follow-up hardening заменил logical-only extra channels на отдельные relay WebSocket/SOCKS streams через `AuthChannel`, добавил active relay-client health per stream и adaptive фактическое число channels; повторный 4-channel stream benchmark прошёл без health/keepalive regressions и дал server receiver `5.827 Mbits/s` при `iperf3 -P 4 -t 25`.
- Сверка с ТЗ: Phase 3 и Phase 4 MVP закрыты по чеклисту; повторный leak-map audit закрыл найденные side channels (`serverIP`, legacy cloud migration probe, API geo/serial redaction, self-hosted metrics/geolite/version checks, client metrics, debug upload, client OAuth/IdP provider HTTP, server-side OIDC/JWKS HTTP, runtime release/update endpoints, anonymous LAN/firewall log redaction). Long-soak/reconnect после новых Tor stream изменений закрыт отдельным стендовым прогоном; финальный docs/release/proxy/dashboard/updater/defaults sweep закрыл найденные old-upstream visible/runtime хвосты. Финальный Tor/I2P remote smoke, adversarial leak sweep и requirement-by-requirement completion audit на тестовых серверах закрыты.
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

Статус: закрыто для direct I2P MVP 2026-05-31. Добавлен SAM client/dialer с настоящим протоколом и unit tests; `i2p-datagram` config теперь принимает `.b32.i2p` management/signal/relay endpoints и подключает management/signal gRPC + relay WebSocket через system SAM bridge. Добавлен SAM RAW/DATAGRAM primitive: session create, UDP send header, receive parser, size/protocol validation. Добавлен destination primitive: `DEST GENERATE SIGNATURE_TYPE=7`, persistent private destination key в local profile config и management exchange public destinations через `PeerSystemMeta`/`RemotePeerConfig`. Добавлен managed/system i2pd lifecycle (`external`/`auto`/`managed`) с настоящим запуском `i2pd`, per-profile config files, SAM readiness wait, graceful stop и Linux service dependencies только для transport/lifecycle режимов, где внешний daemon действительно нужен. Добавлен tunnel tuning config (`length`/`quantity`) через CLI/proto/profile/join/debug/dashboard и SAM session options. Direct peer data-plane подключён и validated на двух реальных клиентах: один локальный SAM `STYLE=DATAGRAM` session используется всеми peer connections, incoming datagrams демультиплексируются по source I2P destination, каждый peer получает `net.Conn` для существующего WireGuard proxy, retry loop + WG watcher reset предотвращают асимметрию direct/relay и stale Connected без handshake, а anonymous mode по-прежнему не создаёт ICE/STUN/direct UDP path. I2P-only combined healthcheck закрыт через localhost/self probe multiplexed relay handler. Like-for-like benchmark against Tor relay-only выполнен на той же клиентской паре: direct I2P дал меньшую latency (~399/364 ms против Tor ~1047/1092 ms), throughput оказался сравнимым и направленно вариативным (`720 Kbits/s`/`2.10 Mbits/s` receiver для I2P против `1.45 Mbits/s`/`1.18 Mbits/s` receiver для Tor).

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
- Logs accidentally contain real IP — закрыто sanitization/redaction, targeted leak-map fixes, short soak, Tor reconnect/failure soak и финальный I2P/Tor adversarial grep после restart combined/Tor/I2P/client; дополнительно закрыты old-upstream visible/runtime surfaces и anonymous LAN/firewall log redaction.

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
- setup/install package commands must not depend on upstream NetBird package/CDN hosts; реализовано через AnonBird source-build commands и configurable local Docker image in `Code/dashboard`;
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

## 22. Post-MVP план доработок

Этот раздел не блокирует уже закрытый MVP scope, но нужен перед удобным тестовым/production rollout.

### 22.0. Release testbed

Для release-readiness и migration testing использовать четыре disposable сервера пользователя, на которых уже выполнялись Tor/I2P smoke tests:

- `93.177.116.58`
- `213.108.3.228`
- `185.246.220.249`
- `45.138.103.224`

Сверка 2026-05-31: это именно четыре сервера пользователя для финального release/open-source прогона; их нельзя заменять или исключать из отчёта без явной blocker-записи.

Это фиксированный release testbed для финального прогона. Если сервер временно недоступен, это записывается как blocker/замена с причиной, а не молча исключается из матрицы.

Роли по умолчанию для следующих прогонов:

- `93.177.116.58`: baseline/self-host management server для Tor/onion и NetBird->AnonBird server migration;
- `213.108.3.228`: baseline/self-host management server для I2P или отдельного clean-install/upgrade прогона;
- `185.246.220.249`: client peer, Marton-server host или NetBird client migration source;
- `45.138.103.224`: client peer, Marton client или NetBird client migration source.

Примечание: на этих серверах нет важных данных; перед destructive migration/rollback тестами всё равно делать backup/snapshot, чтобы проверять rollback path честно и не маскировать ошибки миграции.

Release-readiness gate нельзя закрывать только по локальным unit tests: финальная отметка требует реального release-candidate install/upgrade, NetBird->AnonBird migration server+clients, Marton через overlay и leak/log sweep на этих четырёх серверах.

Обязательный финальный прогон перед отметкой `production-ready`/публичным open-source релизом:

- подтвердить, что все четыре тестовых сервера из списка выше доступны по SSH и что их текущие роли/состояния записаны в release report;
- поднять release-candidate управляющий сервер/dashboard так же просто, как self-hosted NetBird, без ручных патчей после установки;
- выполнить полный test suite: targeted Go tests, dashboard/proxy build, package/install script lint, compose validation, focused leak/secrets sweep, artifact checksum verification и remote smoke уже из release artifacts;
- поднять реальный прикладной проект через AnonBird overlay: первым кандидатом использовать Marton-server, а если artifact/команды запуска недоступны, записать blocker и не закрывать Marton-specific gate smoke-сервисом;
- прогнать миграцию с обычного self-hosted NetBird для server и clients: baseline install обычного NetBird, baseline client connectivity, `anonbird migrate server`, `anonbird migrate client`, post-migration connectivity/dashboard checks и rollback;
- отдельно проверить replace-in-place сценарий для тестового проекта: удалить/остановить старый NetBird, поставить AnonBird documented commands, подтвердить, что dashboard/API/relay/client paths работают без ручных патчей;
- финальный verdict должен прямо ответить, можно ли на тестовом проекте удалить обычный NetBird, поставить AnonBird/`anonbird`, и получить рабочий результат без ручных исправлений.

### 22.1. Linux command/package parity

- [x] Добавить one-command self-host режим для управляющего сервера:
  - `getting-started.sh --domain <domain> --email <email> --yes`;
  - default stack: dashboard + embedded IdP + combined management/signal/relay server + Traefik TLS;
  - `--render-only` для dry-run генерации `docker-compose.yml`, `dashboard.env`, `config.yaml`;
  - README quickstart показывает одну команду для open-source установки.
- [x] Подготовить полноценную Linux install surface по аналогии с обычным `netbird`, но под AnonBird:
  - binary в `PATH`: `/usr/bin/anonbird` или `/usr/local/bin/anonbird`;
  - systemd unit: `anonbird.service`;
  - daemon socket: `/var/run/anonbird.sock`;
  - config/data/log paths: `/etc/anonbird`, `/var/lib/anonbird`, `/var/log/anonbird`;
  - package postinstall/postremove/upgrade scripts;
  - shell completion/man/help docs, если нужны для релиза.
- [x] Добавить временный compatibility option для тестовой миграции старых скриптов:
  - optional symlink `netbird -> anonbird`;
  - optional service alias или явное предупреждение, что canonical service name теперь `anonbird.service`.
- [x] Проверить clean install на Debian/Ubuntu-like Linux:
  - `anonbird service install --service anonbird`;
  - `systemctl enable --now anonbird`;
  - `anonbird status`;
  - `anonbird debug anonymous-check`.
- [x] Проверить clean install на RHEL-like Linux с теми же критериями.

Статус 2026-05-31: Linux install/package surface приведён к canonical AnonBird naming. `release_files/install.sh` ставит release binaries из `Cr0me1ve/anonbird` по умолчанию, поддерживает `--no-service`, `--no-start`, `--compat-symlink`, `--force-compat-symlink` и env overrides для RC до фактического repo rename. DEB/RPM postinstall/preremove умеют безопасно создавать/удалять временный `/usr/bin/netbird -> /usr/bin/anonbird` только по явному `ANONBIRD_COMPAT_SYMLINK=true`. Release metadata, updater artifact URLs, Windows/macOS installer naming, README install commands и GitHub release workflow artifact names переключены на `anonbird*`. Проверено локально: `sh -n`, `shellcheck -S error`, YAML parse, WiX XML parse, targeted `go test` по updater/cmd. Открыто: реальный clean install/upgrade из RC artifacts на testbed.

Статус 2026-05-31: one-command self-host script дополнительно hardened для release artifacts: добавлены image overrides (`ANONBIRD_DASHBOARD_IMAGE`, `ANONBIRD_SERVER_IMAGE`, `ANONBIRD_PROXY_IMAGE`), `--preflight-only`, `--skip-image-preflight` и Docker image preflight перед запуском контейнеров. Если AnonBird release images не опубликованы/приватны/недоступны, installer теперь падает до `docker compose up` с точным списком образов и командами исправления. Generated config теперь отключает anonymous metrics, geolocation downloads и management version checks, а server containers получают `NB_DISABLE_GEOLOCATION=true`. Проверено локально: `bash -n`, `shellcheck -S error`, `--help`, `--render-only` + `docker compose config`, preflight failure для missing images и skip path. Проверено на `93.177.116.58`: one-command RC stack с local RC images поднял dashboard/management/signal/relay через Traefik на `anonbird.93.177.116.58.sslip.io`, dashboard/OIDC отвечают `200`, API без auth `401`, setup-key bootstrap работает, old-upstream/version/geolocation leak grep пустой после fix. Открыто: опубликовать/подтвердить реальные `ghcr.io/cr0me1ve/anonbird-*` release images и прогнать полный server/dashboard clean install именно из published release artifacts.

Статус 2026-05-31: published-image gate preflight показал, что `ghcr.io/cr0me1ve/anonbird-dashboard:latest`, `ghcr.io/cr0me1ve/anonbird-server:latest` и `ghcr.io/cr0me1ve/anonbird-reverse-proxy:latest` пока недоступны для manifest inspect (`manifest unknown`; без auth earlier response was `denied`). Для server/proxy release path исправлен GitHub Actions blocker: `release` job теперь явно запрашивает `contents: write` и `packages: write`, а GHCR login использует `secrets.CI_DOCKER_PUSH_GITHUB_TOKEN || github.token`, чтобы tag release мог публиковать GHCR images без отдельного PAT. Дополнительно release workflow очищен под open-source lint: стандартный `ubuntu-latest-8-cores` runner label, shell-safe FreeBSD diff detection, quoted `$GITHUB_ENV`/`$GITHUB_PATH`, artifacts JSON через env вместо embedded expression. Проверено: YAML parse OK, `actionlint .github/workflows/release.yml` OK, `git diff --check` OK. Открыто: сделать tag/release run, подтвердить public GHCR manifests и повторить one-command self-host без image overrides.

Статус 2026-05-31: Debian-like remote clean install закрыт на `45.138.103.224` из локального RC tarball artifact через `release_files/install.sh` и `file://` release base. Прогон:

- artifact `v0.0.0/anonbird_0.0.0_linux_amd64.tar.gz`, SHA256 `ebd4def1bcb44dd7e0b8c1f50d197e11e23fda6ef4f451fa4667259f98047ae`;
- backup перед destructive clean install: `/root/anonbird-rc-cleaninstall-backup-20260531-144614`;
- installer command использовал `ANONBIRD_RELEASE=v0.0.0`, `ANONBIRD_RELEASE_BASE_URL=file:///tmp/anonbird-rc`, `SKIP_UI_APP=true`, `ANONBIRD_COMPAT_SYMLINK=true`, `--no-start`;
- проверено: `/usr/bin/anonbird`, version `0.0.0-rc-local`, owner/mode `root:root 755`, `/usr/bin/netbird -> /usr/bin/anonbird`, `systemctl enable --now anonbird.service`, service `active`;
- enrollment через I2P management URL с redacted setup key OK, `anonbird debug anonymous-check` OK: management/signal/relay `i2p`, STUN/ICE/direct UDP/fallback disabled, published endpoints none.

Заметка из ручного теста: первый RC tarball с macOS owner сохранил `501:staff` при распаковке; `install.sh` исправлен так, чтобы после установки бинарника нормализовать `0755` и `root:root` на Linux (`root:wheel` на macOS). Также `--update` теперь уважает `ANONBIRD_RELEASE`, а version compare больше не шумит `sort -V` для `*-rc-local` strings.

Статус 2026-05-31: RHEL-like clean install закрыт на `213.108.3.228` через одноразовый AlmaLinux 9.8 systemd container (`ID_LIKE="rhel centos fedora"`, `dnf`, `systemctl`, `x86_64`) с mounted release-style artifacts. Artifact `v0.0.3/anonbird_0.0.3_linux_amd64.tar.gz`, SHA256 `f62c30712a701ab0f473361787c16a8f9d83a7c506c97006a332da3690076026`; installer `release_files/install.sh`, SHA256 `f4c4b209bca6ba92998e048b3a313093aec478216409220a9af1e94fb9bd07e3`. Команда установки: `SKIP_UI_APP=true ANONBIRD_RELEASE=v0.0.3 ANONBIRD_RELEASE_BASE_URL=file:///release ANONBIRD_COMPAT_SYMLINK=true /release/install.sh --no-start`. Проверено: script detected `dnf` and used release binaries without configuring upstream RPM repos; `/usr/bin/anonbird` version `0.0.3-rc-rhel`, owner/mode `root:root 755`; `/usr/bin/netbird -> /usr/bin/anonbird`; `systemctl enable --now anonbird.service`; service `active`/`enabled`; `anonbird status` returns `NeedsLogin`; `anonbird debug anonymous-check` returns `Result: OK` in pre-enrollment mode with `Anonymous mode: pending enrollment`, `Default connection policy: anonymous tor-relay-only`, STUN/ICE/direct UDP disabled, no published endpoints. Найден и исправлен диагностический rough edge: pre-enrollment clean install previously reported a false `anonymous_mode disabled` failure even though no clearnet connection was configured or active. Проверено локально: `go test ./client/cmd -run 'TestBuildAnonymousCheckReport|TestAnonymousRootFlagsDefaultToTorRelayOnly|TestDefaultAnonymousModeAppliedToConfigInput|TestUnsafeClearnet' -count=1`.

### 22.2. Anonymous-by-default UX

- [x] Сделать anonymous mode режимом по умолчанию для новых подключений:
  - `anonbird up` и `anonbird join` без явного override должны включать `--anonymous-mode`;
  - default transport: `tor-relay-only`;
  - Tor relay multipath должен быть включён по умолчанию на несколько streams/channels;
  - generated dashboard/setup commands должны всегда генерировать anonymous command по умолчанию.
- [x] Неанонимное подключение оставить только как явный unsafe override:
  - например `--no-anonymous-mode --allow-unsafe-clearnet --yes-i-understand-this-may-leak-my-ip`;
  - запрещать silent fallback из anonymous mode в clearnet;
  - не принимать clearnet management/signal/relay URLs без явного unsafe confirmation.
- [x] Для CLI interactive mode добавить жёсткое предупреждение перед non-anonymous connect:

```text
WARNING: You are trying to connect without AnonBird anonymous mode.
Your real IP address, NAT endpoint, and local network metadata may be visible
to the management server, relay, and/or other peers.

Type "I understand this may leak my real IP" to continue:
```

- [x] Для non-interactive/scripts требовать отдельный флаг подтверждения и писать warning в stderr/log:
  - `--allow-unsafe-clearnet`;
  - `--yes-i-understand-this-may-leak-my-ip`;
  - exit code != 0, если подтверждение отсутствует.
- [x] В dashboard добавить такой же unsafe warning для любых UI flows, которые создают non-anonymous setup/install command.
- [x] Dashboard generated peer setup commands больше не используют clearnet management URL:
  - `anonbird join` строится только для `.onion`/`.i2p`;
  - `anonbird up`/Docker env не получают реальный clearnet management URL;
  - при unsafe dashboard config показывается blocking warning и placeholder вместо рабочей clearnet команды.

Статус 2026-05-31: dashboard warning расширен в `Code/dashboard/src/modules/setup-netbird-modal/SetupModal.tsx`: при clearnet management URL UI показывает blocking `Callout` с явным предупреждением про real IP/NAT/local metadata leak и требует onion/I2P endpoint до копирования install commands. Проверено: `npx prettier --write src/modules/setup-netbird-modal/SetupModal.tsx`, `npx tsc --noEmit`, `npm run build`, bundle/source grep по warning text и `git diff --check`.
- [x] Добавить/расширить тесты:
  - default `up/join` включает anonymous Tor relay-only;
  - clearnet URL без unsafe confirmation rejected;
  - unsafe confirmation required в non-interactive mode;
  - warning text не содержит secrets/setup keys.

Статус 2026-05-31: anonymous-by-default/unsafe UX tests расширены в `client/cmd`: root flags default to `anonymous-mode=true`, `anonymous-transport=tor-relay-only`, default Tor SOCKS5; `anonbird://join` без transport defaults to Tor relay-only; clearnet join rejection не раскрывает setup key; unsafe clearnet warning не раскрывает setup key или management URL. Проверено: `go test ./client/cmd -run 'Test(AnonymousRootFlagsDefaultToTorRelayOnly|DefaultAnonymousModeAppliedToConfigInput|DefaultAnonymousModeAppliedToLoginRequest|UnsafeClearnet|ParseJoinToken)' -count=1` и migration/init regression `go test ./client/cmd -run 'Test.*Migration|TestInitCommands' -count=1`.

### 22.3. Rename repository and local folders

- [ ] Переименовать GitHub repository/project surface из NetBird fork naming в AnonBird:
  - основной репозиторий: `netbird` -> `anonbird`;
  - dashboard repository: `dashboard` -> `anonbird-dashboard` или другой выбранный canonical name;
  - container/package/image/docs/release URLs должны использовать новое имя.
- [ ] Переименовать локальные рабочие папки на компьютере:
  - `/Users/kirill/Code/netbird` -> `/Users/kirill/Code/anonbird`;
  - `/Users/kirill/Code/dashboard` -> `/Users/kirill/Code/anonbird-dashboard`.
- [ ] Обновить git remotes, CI paths, docs, install commands, dashboard links и release scripts после rename.
- [ ] Отдельно принять решение по Go module/import path:
  - либо оставить `github.com/netbirdio/netbird` как compatibility module path;
  - либо мигрировать на `github.com/Cr0me1ve/anonbird` с полным import rewrite, `go.mod`, ldflags, CI и downstream compatibility notes.

### 22.4. Logo and visual identity

- [x] Нарисовать новый минималистичный логотип AnonBird:
  - красная птица;
  - без глаз по уточнению ТЗ;
  - небольшие рога как у дьявола;
  - трезубец;
  - flat/minimal design, хорошо читаемый в маленьком размере.
- [x] Подготовить asset set:
  - source PNG;
  - favicon/app icon sizes;
  - dashboard logo;
  - desktop tray/app icons;
  - release/social preview, если нужен.
- [x] Заменить старые logo/icon assets в `netbird` и `dashboard`, пересобрать UI/proxy/dashboard bundles и проверить light/dark backgrounds.

Статус 2026-05-31: SVG-вариант отброшен после визуальной сверки как слишком шумный; logo pipeline переведён на raster PNG. После повторной визуальной сверки знак перерисован заново как PNG: красная птица без глаз, с рогами и трезубцем, chroma-key фон вырезан в alpha, зелёный fringe на краях очищен. Source asset: `docs/media/anonbird-logo-source.png`, generated README PNGs `docs/media/logo.png`/`logo-full.png`, release/social preview `docs/media/anonbird-social-preview.png`, proxy web raster assets `proxy/web/src/assets/anonbird.png`/`anonbird-full.png`, dashboard raster assets `src/assets/anonbird-logo.png`/`anonbird-logo-full.png`, dashboard `apple-icon.png`/`favicon.ico`, desktop base/tray/app icons `client/ui/assets/*.png`, `*.ico`, `client/ui/Netbird.icns`; README фиксирует PNG source как canonical brand asset. Визуально проверено: знак красный, без глаз, с рогами и трезубцем; читается в small icon/tray preview. Проверено: `file`, alpha-channel check, proxy web `npm run build`, dashboard `npm run build`, targeted `go test ./client/ui -run '^$'`, `git diff --check` в `netbird` и `dashboard`.

### 22.5. Migration command

- [x] Добавить простой Linux migration helper как first-class CLI command:

```bash
anonbird migrate server --dry-run
anonbird migrate server --apply
anonbird migrate client --dry-run
anonbird migrate client --apply
anonbird migrate client --apply --rejoin "anonbird://join?..."
anonbird migrate rollback
```

Примечание: command name в Linux должен быть lowercase `anonbird`; бренд в тексте может оставаться `AnonBird`/`anonBird`.

Статус 2026-05-31: CLI surface реализован без заглушек. `client` path выполняет dry-run/apply/backup/rollback нативно; `server` path запускает существующий полноценный self-host migration script и поддерживает dry-run/apply. Открыты packaging/e2e пункты ниже.

Статус 2026-05-31: client migration теперь безопаснее для anonymous-by-default: apply отказывается копировать non-anonymous NetBird config без `--rejoin` или явного unsafe confirmation; `--rejoin` переписывает migrated config/helper files в anonymous mode до service start. Live-root migration + rollback закрыт на `93.177.116.58` с настоящим legacy NetBird client profile и installed AnonBird binary. Открыто: двухклиентный upstream NetBird baseline на testbed, apply через release package artifact и post-migration peer/DNS/ACL connectivity.

Статус 2026-05-31: server migration script hardened для release/RC tests: target images можно переопределять env/flags, apply делает image preflight до stop/backup/start, dry-run показывает target images, `--skip-image-preflight` оставлен только для controlled local tests. Portable domain parsing исправлен и проверен fake embedded-IdP dry-run. Реальный server-side NetBird baseline migration E2E закрыт на `93.177.116.58`: upstream NetBird `v0.64.6` 5-container stack поднят, dry-run/apply миграции на RC-local AnonBird images прошли, post-migration dashboard/OIDC/API/setup-key/leak-grep проверены, rollback после AnonBird DB write восстановил старый NetBird stack и management volume snapshot. Открыто: baseline clients + `anonbird migrate client` E2E, DNS/ACL/routes с реальными peers и replace-in-place тест на прикладном проекте.

- [ ] `anonbird migrate server` должен покрывать happy-path self-host Linux install:
  - detect existing NetBird services/processes;
  - stop old services;
  - backup `/etc/netbird`, `/var/lib/netbird`, `/var/log/netbird`, systemd units и DB;
  - copy/move config/data to AnonBird paths;
  - rewrite service/socket/path references;
  - install/start `anonbird.service`;
  - run management/dashboard/setup-key/peer-list sanity checks.
- [ ] `anonbird migrate client` должен покрывать Linux client migration:
  - backup old client config/profile/logs;
  - migrate compatible profile fields;
  - switch service/socket/path to AnonBird;
  - optionally create temporary `netbird` compatibility symlink;
  - support `--rejoin` for clean anonymous Tor/I2P enrollment.
- [ ] `anonbird migrate rollback` должен восстанавливать backup:
  - stop AnonBird services;
  - restore previous NetBird config/data/unit files;
  - restart old service;
  - print exact manual recovery steps if rollback cannot be fully automatic.
- [ ] Safety requirements:
  - default mode is `--dry-run`;
  - refuse to run without backup unless `--no-backup --force`;
  - print all planned file/service changes before applying;
  - do not delete old data during first migration pass;
  - log migration report without secrets/setup keys/private keys.
- [ ] Прогнать end-to-end migration test с обычного NetBird на AnonBird:
  - baseline server: поднять обычный self-host NetBird management/signal/relay/dashboard на одном из testbed серверов;
  - baseline clients: подключить минимум два обычных NetBird клиента на остальных testbed серверах;
  - проверить до миграции peers/groups/policies/DNS/setup keys/status;
  - выполнить `anonbird migrate server --dry-run`, затем `--apply`;
  - выполнить `anonbird migrate client --dry-run`, затем `--apply` или `--apply --rejoin "anonbird://join?..."`;
  - проверить после миграции management/dashboard login, setup-key enrollment, peer connectivity, DNS/ACL, anonymous-check;
  - выполнить rollback test хотя бы один раз и подтвердить восстановление старого NetBird baseline;
  - сохранить migration report без setup keys/private keys/secrets и с точным списком изменённых путей/services.

### 22.6. Release/open-source readiness validation

- [ ] Провести полный release-readiness audit перед публичным open-source релизом:
  - README/quickstart/install docs соответствуют AnonBird, а не NetBird cloud/package hosts;
  - LICENSE/NOTICE/CONTRIBUTING/SECURITY/CODE_OF_CONDUCT готовы к публикации;
  - Go module/import-path compatibility decision задокументирован;
  - GitHub repository rename/remotes/CI status badges/release URLs обновлены;
  - dashboard repository/image names/release docs обновлены;
  - secrets, setup keys, private I2P destinations, onion private keys и тестовые credentials отсутствуют в git history/artifacts;
  - issue/PR templates, workflows, release signing, package signing и container publishing работают на fork infrastructure.
  - release verdict должен явно ответить, можно ли заменить обычный NetBird на AnonBird/anonbird на тестовом проекте без ручных патчей.
- [ ] Проверить replace-in-place сценарий на реальном baseline проекте:
  - поднять обычный upstream/self-hosted NetBird server + dashboard + минимум два клиента на testbed;
  - зафиксировать baseline состояние до миграции: dashboard login, peers, groups, policies, DNS, setup keys, routes и app connectivity;
  - заменить server/dashboard/client surface на AnonBird через documented install/migration commands;
  - подтвердить, что dashboard, management, signal, relay, DNS/ACL и peer connectivity работают после замены;
  - отдельно зафиксировать все несовместимости, ручные действия и blockers, если simple replacement пока невозможен.
- [ ] Прогнать расширенный test suite:
  - targeted Go tests по anonymous/auth/management/relay/client/debug/release surfaces;
  - minimum smoke на `go test ./...` или документированный список исключённых heavy/flaky пакетов с причиной;
  - dashboard `npm run build`;
  - proxy web build;
  - package scripts syntax/lint;
  - compose config validation;
  - focused rg leak sweep по old upstream hosts/secrets/private keys;
  - secret scan по git tree/artifacts на setup keys, private I2P destinations, onion private keys, OAuth/JWT secrets и реальные testbed credentials;
  - release artifact checksums, package install/uninstall smoke и container image pull/run smoke;
  - remote smoke на testbed после установки release artifacts, а не dev binaries.

Статус 2026-05-31: package/install локальный preflight после AnonBird naming pass закрыт (`sh -n`, `shellcheck -S error`, YAML parse, WiX XML parse, targeted updater/cmd Go tests). Дополнительно закрыт self-host quickstart preflight и RC stack smoke: `getting-started.sh --render-only` генерирует валидный Compose stack, `--preflight-only`/image override path даёт fail-fast проверку release images до запуска контейнеров, а `93.177.116.58` подтвердил реальный dashboard/management startup через Traefik с RC-local images и без старых upstream release/geolocation fetches после fix. В `Code/dashboard` исправлен GHCR publishing workflow: PR builds no longer push, release tags publish semver/ref tags and `latest`, package metadata/docs match `anonbird-dashboard`; verified with `actionlint`, YAML parse and `npm run build`. В `netbird` release workflow добавлены explicit `contents: write`/`packages: write` permissions и GHCR login fallback на `github.token`; затем workflow очищен до `actionlint` OK. Текущий GHCR manifest preflight для default quickstart images пока возвращает `manifest unknown`, поэтому published-image clean install остаётся открытым до tag/release run и public pull smoke. Server-side migration E2E + rollback из upstream NetBird `v0.64.6` теперь проверены на `93.177.116.58`. Это не заменяет полный release suite: remote artifact install из опубликованных server/dashboard images, dashboard/proxy build, client migration E2E и Marton overlay test остаются обязательными.
- [ ] Поднять реальный тестовый проект через AnonBird virtual network:
  - развернуть Marton-server на одном testbed peer;
  - подключить другой peer как клиент к Marton-server только по overlay IP/DNS имени AnonBird;
  - проверить TCP/HTTP/WebSocket или другой фактический protocol Marton-server;
  - зафиксировать latency/throughput/errors через Tor relay-only и, отдельно, I2P datagram;
  - подтвердить, что service не доступен через real public IP и что логи AnonBird не раскрывают real peer IPs;
  - зафиксировать, какие ports/protocols Marton использовал, команды запуска и команды проверки с обеих сторон.

Статус 2026-05-31: ранний поиск ошибочно считал Marton artifact недоступным; повторная сверка нашла `/Users/kirill/Code/marten-server`, `/opt/marten-server-test` и `/opt/marten` на testbed, поэтому Marton-specific gate переведён из blocker в реальный release test case. Для базовой application-layer проверки также был добавлен повторяемый тестовый сервис `scripts/anonbird-overlay-smoke-server.py` (HTTP `/health`, HTTP `/echo`, WebSocket `/ws`) и прогнан через AnonBird I2P overlay:

- server peer: `45.138.103.224`, overlay bind только `100.119.114.3:18080`, systemd unit `anonbird-overlay-smoke.service` временно создан и после теста удалён;
- client peer: `185.246.220.249` (`100.119.42.220`);
- listener evidence: `ss -ltnp` показывал только `100.119.114.3:18080`, public `45.138.103.224:18080` не слушался;
- HTTP overlay: `curl http://100.119.114.3:18080/health` -> `{"status":"ok","service":"anonbird-overlay-smoke",...}`;
- HTTP echo overlay: `/echo?message=anonbird-overlay` -> `{"echo":"anonbird-overlay"}`;
- WebSocket overlay: `/ws` echo -> `anonbird-ws:anonbird-ws-check`;
- public IP negative test from `185.246.220.249`: `curl http://45.138.103.224:18080/health` failed with connection refused;
- overlay ping during app test: `185 -> 45` `4/4`, avg `1058.082 ms` during transient I2P latency spike; subsequent status on both peers `Peers count: 2/2 Connected`, `i2p-datagram/i2p-datagram`, `anonymous-check` OK;
- journal grep on `45`, `185` and `213` for public IPs `45.138.103.224|185.246.220.249|213.108.3.228|93.177.116.58` after the app test was empty. Smoke app logs intentionally omit peer addresses and contain only request lines.

Статус 2026-05-31: реальный Marton edge-server overlay smoke частично закрыт на testbed без заглушек:

- source/artifact: `/Users/kirill/Code/marten-server/cmd/edge-server`, remote tree `/opt/marten-server-test` на `45.138.103.224`;
- build note: обычный `CGO_ENABLED=0` build непригоден, потому что `github.com/mattn/go-sqlite3` требует CGO; Alpine dynamic build тоже непереносим на Ubuntu host из-за musl loader, поэтому для smoke собран static CGO linux/amd64 binary;
- server peer: `45.138.103.224`, Marton edge bind только `100.119.114.3:18082`, metrics bind только `100.119.114.3:19092`;
- client peer: `185.246.220.249` (`100.119.42.220`);
- listener evidence: `ss -ltnp` показывал `100.119.114.3:18082` и `100.119.114.3:19092`, без public `0.0.0.0`/`45.138.103.224` listener;
- overlay ping `185 -> 45`: `4/4`, avg `439.845 ms`;
- Marton health over overlay: `curl http://100.119.114.3:18082/healthz` -> `200 {"status":"ok"}`;
- Marton import route over overlay: `curl -D- "http://100.119.114.3:18082/sub/anonbird-test/import?name=AnonBird"` -> `302 Location: marten://import?...url=http%3A%2F%2F100.119.114.3%3A18082%2Fsub%2Fanonbird-test`;
- Marton metrics over overlay: `curl http://100.119.114.3:19092/metrics` -> `200`, Prometheus metrics emitted;
- public IP negative test from `185.246.220.249`: `curl http://45.138.103.224:18082/healthz` failed with connection refused / HTTP code `000`;
- `anonbird debug anonymous-check` на `45` и `185` после test: OK, `i2p-datagram`, management/signal/relay `i2p`, STUN/ICE/direct UDP/fallback disabled, published endpoints none;
- journal grep on `45`, `185` and I2P server `213` after Marton test was empty for public testbed IPs, old upstream hosts and STUN/ICE/NAT discovery patterns;
- temporary Marton process stopped after test; static test binary left in `/opt/marten-server-test/marten-edge-overlay` for repeatability.

Статус 2026-05-31: Marton master-backed subscription flow через AnonBird I2P overlay закрыт на распределённом testbed:

- master peer: `45.138.103.224`, isolated Postgres test container bound only to `127.0.0.1:15432`, Marton master bound only to overlay `100.119.114.3:18081` and gRPC `100.119.114.3:18091`;
- edge peer: `185.246.220.249`, Marton edge bound only to overlay `100.119.42.220:18082` and metrics `100.119.42.220:19092`;
- edge -> master control path: `cache invalidation stream connected` to `100.119.114.3:18091`, and master logs show `/internal/bind` + `/internal/sub/{token}` from remote_addr `100.119.42.220`;
- client -> edge path: request from `45.138.103.224` to `http://100.119.42.220:18082/sub/<redacted>` returned `200` with real Marton config payload: `outbounds=1`, first outbound `type=wireguard`, server `100.119.114.3`;
- import route over overlay returned `302 marten://import?...url=http://100.119.42.220:18082/sub/<redacted>`;
- edge metrics over overlay returned `200`; master health over overlay from `185.246.220.249` returned `200 {"status":"ok","db":"ok"}`;
- public negative tests: `http://45.138.103.224:18081/healthz` and `http://185.246.220.249:18082/healthz` both failed with connection refused / HTTP code `000`;
- `anonbird debug anonymous-check` on `45` and `185`: OK, `i2p-datagram`, management/signal/relay `i2p`, STUN/ICE/direct UDP/fallback disabled, published endpoints none;
- Marton master/edge logs contain only overlay remote addresses (`100.119.42.220`, `100.119.114.3`) for the tested paths; grep for public testbed IPs and old upstream/STUN/ICE/NAT patterns was empty on Marton logs, AnonBird client journals and the I2P server journal;
- temporary Marton processes stopped, Postgres test container removed, and files containing the test subscription token removed. Built test binaries/logs without subscription secret were left for repeatability/debug.

Открыто: повторить Marton master-backed flow через Tor relay-only profile and record latency/throughput/errors separately, because the Marton full-flow proof above covers the I2P datagram overlay path only. Для финального release gate также нужно повторить этот тест из release artifacts, not dev/test binaries.
- [ ] Выполнить release-candidate install/upgrade flow:
  - clean install server/dashboard/client из release packages/images;
  - upgrade с предыдущего AnonBird dev build;
  - migration с обычного NetBird baseline;
  - uninstall/reinstall;
  - rollback;
  - повторный join после rollback/upgrade.

Статус 2026-05-31: client-side RC artifact clean install + upgrade smoke частично закрыт на testbed:

- clean install: `45.138.103.224` из local release-style tarball `v0.0.0`, service active, I2P anonymous-check OK;
- upgrade: `ANONBIRD_RELEASE=v0.0.1 ANONBIRD_RELEASE_BASE_URL=file:///tmp/anonbird-rc-upgrade bash install.sh --update` обновил client до `0.0.1-rc-local`, переустановил и запустил `anonbird.service`, сохранил config/profile, binary owner `root:root 755`, compatibility symlink сохранился;
- после I2P reconnect `45.138.103.224` снова `Management/Signal Connected`, relay `.b32.i2p` available, `anonymous-check` OK;
- overlay validation после upgrade: `185.246.220.249` (`100.119.42.220`) -> `45.138.103.224` (`100.119.114.3`) ping `6/6`, avg `383.326 ms`; reverse `45 -> 185` ping `6/6`, avg `425.199 ms`; both sides show `i2p-datagram/i2p-datagram`, no STUN/ICE/direct UDP.

Статус 2026-05-31: client-side RC artifact uninstall/reinstall закрыт на `45.138.103.224` из release-style tarball `v0.0.2/anonbird_0.0.2_linux_amd64.tar.gz`, SHA256 `bb96b16b8c2363a30d2596678af3f9a26f92a8f24098c980fd7cb39cc3bae5f5`; installer `release_files/install.sh`, SHA256 `f4c4b209bca6ba92998e048b3a313093aec478216409220a9af1e94fb9bd07e3`. Первый reinstall pass выявил production blocker: из-за anonymous-by-default `anonbird service install` игнорировал сохранённый `i2p-datagram` profile и ставил `Wants/After=tor.service`; параллельно системный Ubuntu/Noble `i2pd.service` на `45` и `185` падал с core-dump, что подтвердило риск внешнего daemon dependency. Исправлено: service install/reconfigure теперь читает сохранённый profile, если anonymous flags не заданы явно; `i2p-datagram` + `external` получает `Wants/After=i2pd.service`, а `auto`/`managed` не получает внешнюю `i2pd.service` dependency и запускает managed process. Повторный reinstall: service active, unit содержит только `After=network.target syslog.target`, без `tor.service`/`i2pd.service`; AnonBird поднял managed `/usr/bin/i2pd --datadir=/var/lib/i2pd/anonbird`, `anonymous-check` OK, management/signal/relay connected, peers `2/2`. Overlay validation после warmup: `45 -> 185` ping `10/10`, avg `376.624 ms`; `185 -> 45` ping `10/10`, avg `576.365 ms`. Post-marker journal grep на `45` по old upstream/STUN/ICE/candidate/NAT/public testbed IP patterns пустой. Проверено локально: targeted `go test ./client/cmd -run 'TestConfiguredAnonymousRuntimeServiceDependencies|TestAnonymousRuntimeServiceDependencies|TestApplyServiceParams' -count=1`.

Открыто: clean install server/dashboard artifacts из опубликованных images, RHEL-like client install, двухклиентный NetBird baseline migration, Marton service test. Server-side NetBird baseline migration + rollback и single live-root client migration + rollback на RC-local/dev artifact закрыты отдельными E2E.
- [ ] Перед публичным релизом создать release report:
  - commit/tag;
  - artifact checksums;
  - test matrix;
  - known limitations;
  - migration notes;
  - security caveats;
  - production readiness verdict.

---

## 23. Короткий итог

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
