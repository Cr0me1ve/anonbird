#!/bin/sh
# decide if we should use systemd or init/upstart
use_systemctl="True"
systemd_version=0
if ! command -V systemctl >/dev/null 2>&1; then
  use_systemctl="False"
else
    systemd_version=$(systemctl --version | head -1 | sed 's/systemd //g')
fi

remove() {
  printf "\033[32m Pre uninstall\033[0m\n"

  if [ "${use_systemctl}" = "True" ]; then
    printf "\033[32m Stopping the service\033[0m\n"
    systemctl stop anonbird || true

    if [ -e /lib/systemd/system/anonbird.service ]; then
      rm -f /lib/systemd/system/anonbird.service
      systemctl daemon-reload || true
    fi

  fi
  printf "\033[32m Uninstalling the service\033[0m\n"
  /usr/bin/anonbird service uninstall || true
  removeCompatSymlink


  if [ "${use_systemctl}" = "True" ]; then
     printf "\n\033[32m running daemon reload\033[0m\n"
     systemctl daemon-reload || true
  fi
}

removeCompatSymlink() {
  link="/usr/bin/netbird"
  target="/usr/bin/anonbird"

  if [ -L "$link" ] && [ "$(readlink "$link" 2>/dev/null || true)" = "$target" ]; then
    printf "\033[32m Removing compatibility symlink %s\033[0m\n" "$link"
    rm -f "$link"
  fi
}

action="$1"

case "$action" in
  "0" | "remove")
    remove
    ;;
  *)
    exit 0
    ;;
esac
