#!/bin/sh

# Step 1, decide if we should use systemd or init/upstart
use_systemctl="True"
systemd_version=0
if ! command -V systemctl >/dev/null 2>&1; then
  use_systemctl="False"
else
    systemd_version=$(systemctl --version | head -1 | sed 's/systemd //g')
fi

cleanInstall() {
    printf "\033[32m Post Install of an clean install\033[0m\n"
    # Step 3 (clean install), enable the service in the proper way for this platform
    /usr/bin/anonbird service install
    /usr/bin/anonbird service start
    installCompatSymlink
}

upgrade() {
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    if [ "${use_systemctl}" = "True" ]; then
      printf "\033[32m Stopping the service\033[0m\n"
      systemctl stop anonbird 2> /dev/null || true
    fi
    if [ -e /lib/systemd/system/anonbird.service ]; then
      rm -f /lib/systemd/system/anonbird.service
      systemctl daemon-reload
    fi
    # will throw an error until everyone upgrades
    /usr/bin/anonbird service uninstall 2> /dev/null || true
    /usr/bin/anonbird service install
    /usr/bin/anonbird service start
    installCompatSymlink
}

isTrue() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|y|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

installCompatSymlink() {
  if ! isTrue "${ANONBIRD_COMPAT_SYMLINK:-false}"; then
    return 0
  fi

  link="/usr/bin/netbird"
  target="/usr/bin/anonbird"

  if [ ! -x "$target" ]; then
    printf "\033[33m Cannot create compatibility symlink; %s is missing\033[0m\n" "$target"
    return 1
  fi

  if [ -e "$link" ] || [ -L "$link" ]; then
    current_target="$(readlink "$link" 2>/dev/null || true)"
    if [ "$current_target" = "$target" ]; then
      printf "\033[32m Compatibility symlink already exists: %s -> %s\033[0m\n" "$link" "$target"
      return 0
    fi
    if ! isTrue "${ANONBIRD_COMPAT_SYMLINK_FORCE:-false}"; then
      printf "\033[33m Not overwriting existing %s. Set ANONBIRD_COMPAT_SYMLINK_FORCE=true to replace it.\033[0m\n" "$link"
      return 0
    fi
    rm -f "$link"
  fi

  ln -s "$target" "$link"
  printf "\033[32m Created temporary compatibility symlink: %s -> %s\033[0m\n" "$link" "$target"
}

# Check if this is a clean install or an upgrade
action="$1"
if  [ "$1" = "configure" ] && [ -z "$2" ]; then
  # Alpine linux does not pass args, and deb passes $1=configure
  action="install"
elif [ "$1" = "configure" ] && [ -n "$2" ]; then
    # deb passes $1=configure $2=<current version>
    action="upgrade"
fi

case "$action" in
  "1" | "install")
    cleanInstall
    ;;
  "2" | "upgrade")
    upgrade
    ;;
  *)
    cleanInstall
    ;;
esac
