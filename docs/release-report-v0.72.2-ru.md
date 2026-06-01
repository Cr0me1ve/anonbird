# Релизный отчет AnonBird v0.72.2

Дата: 2026-06-01

Вердикт: релиз готов к production-использованию в рамках проверенного
open-source self-hosted anonymous mesh сценария. Тестовый проект может заменить
обычный self-hosted NetBird на AnonBird через задокументированные команды
миграции сервера и клиентов, после чего клиенты работают в anonymous mode по
умолчанию. Это не означает гарантированную "слепую" замену для каждой частной
кастомизации NetBird без чтения миграционных заметок.

## Идентичность релиза

- Основной репозиторий: `Cr0me1ve/anonbird`
- Tag: `v0.72.2`
- Commit: `d06b590cc9d0a08db724e2cdf967a735b8c467cd`
- GitHub Release: `https://github.com/Cr0me1ve/anonbird/releases/tag/v0.72.2`
- Dashboard репозиторий: `Cr0me1ve/anonbird-dashboard`
- Dashboard tag: `v0.72.2`
- Dashboard commit: `4f85d96d43b43f40e0bcdb69af165814590665c4`

`v0.72.2` заменяет `v0.72.0` и `v0.72.1` как актуальный стабильный релиз.
В `v0.72.0` был найден installer SemVer bug при обновлении с RC на stable.
`v0.72.1` исправил этот баг, но tag release gate не был полностью зеленым из-за
Windows installer `amd64`, который зависел от недоступного внешнего Mesa3D host.
В `v0.72.2` Windows Mesa3D source закреплен на checksum-verified GitHub release,
и полный tag release gate прошел успешно.

## Основные артефакты

| Артефакт | SHA256 |
| --- | --- |
| `install.sh` | `8388aeac08121d3644306561072e4a58910cd8b118bf654da45bc1aa54b28e2d` |
| `getting-started.sh` | `b62f3f9227213016ba6640b02a945973a8db8dfd8dcf3d2f0910ef170559b15f` |
| `anonbird_0.72.2_checksums.txt` | `16d1adb9856bc6ca354f99f532083bc618daecbdbee23d225bea59efa29c15bd` |
| `anonbird_0.72.2_linux_amd64.tar.gz` | `ea8be967b734926cdfc8b3a1acfa2fdf53c303a3e781d563659f8ed7f518fca9` |
| `anonbird_0.72.2_linux_amd64.deb` | `b8e4930e9f958337cd252aee7c7ae5e752676d17bf021e8b6858d45a846fccef` |
| `anonbird_0.72.2_linux_amd64.rpm` | `48fbcd48ea2a8001f5a1abfef7fadb6575728cc1b24b8627080541434b2173a0` |
| `anonbird_0.72.2_windows_amd64.tar.gz` | `edec0e25e127bb5c5859dc5db941d211c24cdc1d79e794efc05510b167d1346b` |
| `anonbird-ui-windows_0.72.2_windows_amd64.tar.gz` | `682b2ecab3b4093cbd2d0aedce7903f91dbc12aec82bbdf1b4be1d315b90ddbd` |

`releases/latest/download/install.sh` и
`releases/latest/download/getting-started.sh` редиректят на `v0.72.2`.

## CI и release gate

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

После добавления финального release report основной `main` CI также был
перепроверен и прошел зеленым: Linux, Windows, Darwin, FreeBSD, Mobile, Wasm,
install tests и infrastructure tests.

## Container images

Проверенные manifests:

- `ghcr.io/cr0me1ve/anonbird-server:0.72.2`: `linux/amd64`, `linux/arm64`
- `ghcr.io/cr0me1ve/anonbird-reverse-proxy:0.72.2`: `linux/amd64`, `linux/arm64`
- `ghcr.io/cr0me1ve/anonbird-dashboard:0.72.2`: `linux/amd64`, `linux/arm64`, `linux/arm/v7`
- `ghcr.io/cr0me1ve/anonbird-server:latest`: `linux/amd64`, `linux/arm64`
- `ghcr.io/cr0me1ve/anonbird-dashboard:latest`: `linux/amd64`, `linux/arm64`, `linux/arm/v7`

## Ручные release tests

Testbed:

- Self-host smoke сервера: `213.108.3.228`
- Пара клиентов: `185.246.220.249`, `45.138.103.224`
- Дополнительные release testbed servers из плана:
  `93.177.116.58`, `213.108.3.228`, `185.246.220.249`,
  `45.138.103.224`
- Ручной Windows fallback build/test host: `gay--@192.168.0.100`

Self-host smoke из опубликованных артефактов:

- Источник команды:
  `https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh`
- Domain: `anonbird-v0722.213.108.3.228.sslip.io`
- `--preflight-only`: `AnonBird Docker image preflight passed`
- Полный `--yes` install: stack поднялся из опубликованных `latest` images
- Dashboard `/`: HTTP `200`
- OIDC discovery: HTTP `200`
- Unauthenticated `/api/users`: HTTP `401`
- `setup-key bootstrap`: success; setup key намеренно не включен в отчет
- Config подтвержден: `stunPorts: []`, `disableAnonymousMetrics: true`,
  `disableGeoliteUpdate: true`, `disableVersionCheck: true`
- Compose/config grep подтвердил отсутствие `3478/udp`
- Server log grep по upstream/version/geolocation leak patterns: пусто
- Тестовый stack и volumes удалены после фиксации evidence

Published client update smoke:

- `185.246.220.249`: обновлен с `0.72.0-rc.7` до `0.72.2` через
  `releases/latest/download/install.sh --update`
- `45.138.103.224`: обновлен с `0.72.0-rc.7` до `0.72.2` через
  `releases/latest/download/install.sh --update`
- Оба клиента: `command -v anonbird` -> `/usr/bin/anonbird`
- Оба клиента: `anonbird.service` active, legacy `netbird.service` inactive
- Оба клиента: `/etc/anonbird/install.conf` содержит `package_manager=apt`
- Оба клиента: `anonbird debug anonymous-check` вернул `OK`
- Оба клиента: management, signal и relay работают через Tor; STUN, ICE,
  direct UDP и clearnet fallback выключены; published endpoints отсутствуют;
  WireGuard работает в userspace mode

Overlay connectivity после update:

- `185.246.220.249` (`100.79.32.188`) -> `45.138.103.224`
  (`100.79.20.213`): ping `4/4`, average `961.040 ms`
- `45.138.103.224` (`100.79.20.213`) -> `185.246.220.249`
  (`100.79.32.188`): ping `4/4`, average `1040.185 ms`
- Обе стороны видят проверяемый peer как `Relayed`
- Relay server: `.onion`
- ICE candidate endpoints: `-/-`
- Post-update journal grep за свежий интервал по upstream, version-check,
  STUN/ICE/NAT discovery, public testbed IP и shortcut leak patterns: пусто

Windows Mesa3D hardening:

- Новый source:
  `https://github.com/pal1000/mesa-dist-win/releases/download/26.1.1/mesa3d-26.1.1-release-msvc.7z`
- SHA256:
  `d5e90e9ae4d620313b61fbbf8e9a55761454e38b6501c39be6d93449c88780e1`
- Проверено локально и на `gay--@192.168.0.100`
- Извлечены required files: `x64/opengl32.dll`, `x64/libgallium_wgl.dll`
- Local PE import check подтвердил, что `opengl32.dll` импортирует
  `libgallium_wgl.dll`

Migration и application-layer gates:

- Server-side migration с upstream NetBird `v0.64.6`, rollback и restore
  выполнены на `93.177.116.58`; детали записаны в
  `anonbird_netbird_fork_plan.md`
- Two-client NetBird-to-AnonBird migration, rollback/reapply и anonymous rejoin
  выполнены; детали записаны в `anonbird_netbird_fork_plan.md`
- Marton master/edge subscription flow был проверен через AnonBird Tor
  relay-only overlay из опубликованных RC7 client artifacts; stable update до
  `v0.72.2` сохранил ту же проверенную client pair и anonymous transport
  properties

## Известные ограничения

- Anonymous mode защищает AnonBird peer connectivity от публикации реальных IP
  peers, если клиенты используют onion/I2P management, signal и relay endpoints.
  Это не заявка на анонимность против глобального пассивного наблюдателя,
  скомпрометированных endpoints или malware на хосте.
- Использование Tor/I2P может быть видно локальному провайдеру сети, если
  оператор отдельно не добавил pluggable transports или слой обхода блокировок.
- Clearnet admin dashboard поддерживается для удобства оператора, но peer
  onboarding должен использовать onion/I2P service URL, если нужно сохранить
  свойство "dashboard и management не узнают public IP клиентов через клиентский
  путь AnonBird".
- Non-anonymous/clearnet modes оставлены для совместимости, но они не являются
  default и требуют явного unsafe confirmation.
- Windows и macOS artifacts собираются и пакуются, но production code
  signing/notarization остается opt-in и зависит от credentials оператора.
  Release gate проверил unsigned installer/package builds.
- Часть stale peers от ранних тестов может оставаться в test account в состоянии
  `Connecting`. Вердикт релиза основан на свежей проверенной паре peers и
  миграционных сценариях.

## Вердикт по замене NetBird

Для тестового проекта на обычном self-hosted NetBird замена на AnonBird
реалистична и поддержана задокументированным migration flow:

1. Запустить one-command self-host install или server migration command.
2. Сгенерировать или перенести setup keys.
3. Выполнить client migration с anonymous rejoin.
4. Проверить `anonbird debug anonymous-check`.
5. Проверить application traffic через overlay IP/DNS.

Проверенные server, clients и Marton application flow не потребовали ручных
code patches после описанных migration/update steps.
