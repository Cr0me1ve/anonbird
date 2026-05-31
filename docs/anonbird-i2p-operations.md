# AnonBird I2P operations guide

Дата: 2026-05-31

Этот документ фиксирует production guidance для режима `i2p-datagram`. Он основан на реальном стенде AnonBird с management/signal/relay через `.b32.i2p` и direct I2P datagram data-plane между двумя клиентами.

## Version policy

Для production используйте `i2pd` 2.60+ или предварительно проверяйте выбранный пакет на static server tunnels и SAM `STREAM`/`DATAGRAM`.

На тестовом Ubuntu/Noble server package `i2pd 2.49.0` падал на static server tunnel. После установки PurpleI2P PPA с `i2pd 2.60.0` тот же server tunnel и SAM bridge стали стабильными.

Минимальная проверка перед rollout:

```bash
i2pd --version
systemctl status i2pd
ss -ltnp | grep 7656
```

## Daemon modes

AnonBird client поддерживает три режима:

- `external`: клиент требует уже запущенный SAM bridge и падает без auto-start.
- `auto`: клиент сначала проверяет SAM bridge, затем пробует запустить `i2pd`, если SAM недоступен.
- `managed`: клиент всегда владеет запущенным `i2pd` process для текущего profile.

CLI flags:

```bash
anonbird up \
  --anonymous-mode \
  --anonymous-transport i2p-datagram \
  --i2p-sam 127.0.0.1:7656 \
  --i2p-tunnel-length 1 \
  --i2p-tunnel-quantity 3 \
  --i2p-daemon-mode auto \
  --i2pd-path i2pd
```

`anonbird join` принимает те же параметры в invite URL:

```text
anonbird://join?server=http://example.b32.i2p&setup_key=...&transport=i2p-datagram&i2p_sam=127.0.0.1%3A7656&i2p_tunnel_length=1&i2p_tunnel_quantity=3&i2p_daemon_mode=auto&i2pd_path=i2pd
```

## Managed layout

Если `--i2p-data-dir` не задан:

- Linux root: `/var/lib/i2pd/anonbird`
- Linux user: `~/.i2pd/anonbird`
- other OS: user config dir + `/anonbird/i2pd`

Managed AnonBird writes:

- `i2pd.conf`
- `tunnels.conf`
- `tunnels.d/`
- `i2pd.log`
- `i2pd.pid`

The client starts `i2pd` with explicit `--datadir`, `--conf`, `--tunconf`, `--tunnelsdir` and `--pidfile`. This avoids writing managed runtime files under `/etc/anonbird/i2pd` and is compatible with stricter Debian/Ubuntu AppArmor defaults.

The generated client config disables non-required local proxies:

- HTTP proxy: disabled
- SOCKS proxy: disabled
- I2CP: disabled
- UPnP: disabled
- transit: disabled for managed client mode
- SAM: enabled on the configured local address

## Systemd

Linux service installation adds runtime dependencies according to the selected I2P daemon mode:

- `i2p-datagram` + `external`: `Wants=i2pd.service` and `After=i2pd.service`
- `i2p-datagram` + `auto`/`managed`: no `i2pd.service` dependency; AnonBird starts and owns a managed `i2pd` process when SAM is unavailable
- `tor-relay-only`: `Wants=tor.service` and `After=tor.service`

For `external` mode, keep `i2pd.service` enabled:

```bash
systemctl enable --now i2pd
systemctl restart anonbird
```

For `auto`/`managed`, do not rely on a system `i2pd.service` as the runtime owner. This avoids a failure mode where AnonBird connects to a system SAM bridge during startup, then the system `i2pd` crashes later and the client loses I2P without owning a process it can restart. In these modes, verify the managed process instead:

```bash
pgrep -a i2pd
ss -ltnp | grep 7656
anonbird debug anonymous-check
```

## Server-side I2P tunnel

The management/signal/relay server should expose only localhost listeners to i2pd. The tested combined layout used:

- management/signal/relay combined listeners on `127.0.0.1`
- I2P server tunnel forwarding public `.b32.i2p:80` to local combined HTTP/relay port
- combined `/health` on localhost

Healthcheck example:

```bash
curl -fsS http://127.0.0.1:19000/health
```

For I2P-only combined servers, the healthcheck must probe the local multiplexed relay handler, not the public `.b32.i2p` address through normal DNS.

## Health and recovery

Client-side anonymous check:

```bash
anonbird debug anonymous-check
```

Expected I2P lines include:

```text
Anonymous mode: enabled
Anonymous transport: i2p-datagram
I2P SAM reachable: yes
STUN: disabled
ICE: disabled
Direct UDP: disabled
Clearnet fallback: disabled
Result: OK
```

Operational checks:

```bash
systemctl status i2pd anonbird
journalctl -u i2pd -u anonbird -n 200 --no-pager
ss -ltnp | grep 7656
```

If SAM starts timing out after restarts or load, restart I2P first, then AnonBird:

```bash
systemctl restart i2pd
systemctl restart anonbird
anonbird debug anonymous-check
```

On the test stand, one client recovered from SAM `DIAL` timeouts after `systemctl restart i2pd`; AnonBird direct-I2P retry then upgraded peers from relay fallback back to `P2P` `i2p-datagram`.

## Security checks

Do not expose SAM or i2pd admin ports publicly. Bind SAM to `127.0.0.1` unless there is a deliberate local-network deployment model and firewall.

In anonymous mode, these are failures:

- management/signal/relay URL is a clearnet host;
- STUN/TURN appears in the network map;
- ICE candidate signaling starts;
- WireGuard endpoint becomes a public `ip:port`;
- `anonymous-check` reports direct UDP or clearnet fallback;
- dashboard shows public connection IP for an anonymous peer.

The private I2P destination key must remain only in local profile config. Management stores and shares only the public destination needed by other anonymous peers.
