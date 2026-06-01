#!/usr/bin/env bash
# AnonBird installer for release artifacts published by the fork.
set -e

CONFIG_FOLDER="${ANONBIRD_CONFIG_FOLDER:-/etc/anonbird}"
CONFIG_FILE="$CONFIG_FOLDER/install.conf"

OWNER="${ANONBIRD_GITHUB_OWNER:-Cr0me1ve}"
REPO="${ANONBIRD_GITHUB_REPO:-anonbird}"
CLI_APP="anonbird"
UI_APP="anonbird-ui"
PROJECT_URL="${ANONBIRD_PROJECT_URL:-https://github.com/${OWNER}/${REPO}}"
RELEASE_API_URL="${ANONBIRD_RELEASE_API_URL:-https://api.github.com/repos/${OWNER}/${REPO}/releases/latest}"
RELEASE_BASE_URL="${ANONBIRD_RELEASE_BASE_URL:-${PROJECT_URL}/releases/download}"
COMPAT_SYMLINK="${ANONBIRD_COMPAT_SYMLINK:-false}"
FORCE_COMPAT_SYMLINK="${ANONBIRD_COMPAT_SYMLINK_FORCE:-false}"
SKIP_SERVICE_INSTALL="${ANONBIRD_SKIP_SERVICE:-false}"
SKIP_SERVICE_START="${ANONBIRD_SKIP_SERVICE_START:-false}"

# Set default variable
OS_NAME=""
OS_TYPE=""
ARCH="$(uname -m)"
PACKAGE_MANAGER="bin"
INSTALL_DIR="${ANONBIRD_INSTALL_DIR:-}"
SUDO=""


if command -v sudo > /dev/null && [ "$(id -u)" -ne 0 ]; then
    SUDO="sudo"
elif command -v doas > /dev/null && [ "$(id -u)" -ne 0 ]; then
    SUDO="doas"
fi

if [ -z ${ANONBIRD_RELEASE+x} ]; then
    ANONBIRD_RELEASE="latest"
fi

TAG_NAME=""

usage() {
    cat <<EOF
AnonBird installer

Usage:
  install.sh [--update] [--compat-symlink] [--no-service] [--no-start]

Environment:
  ANONBIRD_RELEASE=<latest|vX.Y.Z>       Release to install (default: latest)
  ANONBIRD_GITHUB_OWNER=<owner>          GitHub owner (default: Cr0me1ve)
  ANONBIRD_GITHUB_REPO=<repo>            GitHub repo (default: anonbird)
  ANONBIRD_RELEASE_BASE_URL=<url>        Release artifact base URL
  ANONBIRD_INSTALL_DIR=<path>            Binary install directory
  ANONBIRD_COMPAT_SYMLINK=true           Add netbird -> anonbird compatibility symlink
  ANONBIRD_COMPAT_SYMLINK_FORCE=true     Replace an existing compatibility symlink/path
  ANONBIRD_SKIP_SERVICE=true             Install binaries only, do not install/start service
  ANONBIRD_SKIP_SERVICE_START=true       Install service but do not start it
  SKIP_UI_APP=true                       Skip desktop UI binary
EOF
}

is_true() {
    case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
        1|true|yes|y|on)
            return 0
        ;;
        *)
            return 1
        ;;
    esac
}

set_default_install_dir() {
    if [ -z "$INSTALL_DIR" ]; then
        INSTALL_DIR="$1"
    fi
}

normalize_installed_binary() {
    installed_path="$1"
    if [ ! -e "$installed_path" ]; then
        return 0
    fi

    ${SUDO} chmod 0755 "$installed_path"
    case "$OS_TYPE" in
        linux)
            ${SUDO} chown root:root "$installed_path" 2>/dev/null || true
        ;;
        darwin)
            ${SUDO} chown root:wheel "$installed_path" 2>/dev/null || true
        ;;
    esac
}

UPDATE_FLAG=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        --update)
            UPDATE_FLAG="--update"
        ;;
        --compat-symlink)
            COMPAT_SYMLINK=true
        ;;
        --force-compat-symlink)
            COMPAT_SYMLINK=true
            FORCE_COMPAT_SYMLINK=true
        ;;
        --no-service)
            SKIP_SERVICE_INSTALL=true
            SKIP_SERVICE_START=true
        ;;
        --no-start)
            SKIP_SERVICE_START=true
        ;;
        -h|--help)
            usage
            exit 0
        ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 2
        ;;
    esac
    shift
done

get_release() {
    local RELEASE=$1
    if [ "$RELEASE" = "latest" ]; then
        local URL="${RELEASE_API_URL}"
    else
        if echo "${RELEASE}" | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$'; then
            RELEASE=$(echo "${RELEASE}" | sed 's/^/v/' | sed 's/^vv/v/')
            TAG_NAME="\"tag_name\":\"${RELEASE}\""
            echo "${RELEASE}"
            return 0
        fi
        echo "Unsupported release tag: ${RELEASE}" >&2
        return 1
    fi
	OUTPUT=""
    if [ -n "$GITHUB_TOKEN" ]; then
          OUTPUT=$(curl -fH  "Authorization: token ${GITHUB_TOKEN}" -s "${URL}" || true)
    else
          OUTPUT=$(curl -fsSL "${URL}" || true)
    fi
    if [ -z "$OUTPUT" ]; then
        URL="https://api.github.com/repos/${OWNER}/${REPO}/releases?per_page=1"
        if [ -n "$GITHUB_TOKEN" ]; then
              OUTPUT=$(curl -fH  "Authorization: token ${GITHUB_TOKEN}" -s "${URL}" || true)
        else
              OUTPUT=$(curl -fsSL "${URL}" || true)
        fi
    fi
	TAG_NAME=$(echo ${OUTPUT} |  grep -Eo '\"tag_name\":\s*\"v?([0-9]+\.){2}[0-9]+([-+][0-9A-Za-z.-]+)?\"' | head -n 1)
	echo "${TAG_NAME}" | grep -oE 'v?[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?' | sed 's/^/v/' | sed 's/^vv/v/'
}

download_release_binary() {
    VERSION=$(get_release "$ANONBIRD_RELEASE")
    if [ -z "$VERSION" ]; then
      echo "Failed to resolve AnonBird release ${ANONBIRD_RELEASE}"
      exit 1
    fi
	echo "Using the following tag name for binary installation: ${TAG_NAME}"
    BASE_URL="${RELEASE_BASE_URL}"
    BINARY_BASE_NAME="${VERSION#v}_${OS_TYPE}_${ARCH}.tar.gz"

    if [ "$1" = "$UI_APP" ]; then
       BINARY_NAME="$1-${OS_TYPE}_${BINARY_BASE_NAME}"
       if [ "$OS_TYPE" = "darwin" ]; then
         BINARY_NAME="$1_${BINARY_BASE_NAME}"
       fi
    else
       BINARY_NAME="$1_${BINARY_BASE_NAME}"
    fi

    DOWNLOAD_URL="${BASE_URL}/${VERSION}/${BINARY_NAME}"

    echo "Installing $1 from $DOWNLOAD_URL"
    if [ -n "$GITHUB_TOKEN" ]; then
      cd /tmp && curl -fH  "Authorization: token ${GITHUB_TOKEN}" -LO "$DOWNLOAD_URL"
    else
      cd /tmp && curl -fLO "$DOWNLOAD_URL"
    fi


    if [ "$OS_TYPE" = "darwin" ] && [ "$1" = "$UI_APP" ] && echo "$BINARY_NAME" | grep -q '\.zip$'; then
        INSTALL_DIR="/Applications/AnonBird UI.app"

        if test -d "$INSTALL_DIR" ; then
          echo "removing $INSTALL_DIR"
          rm -rfv "$INSTALL_DIR"
        fi

        # Unzip the app and move to INSTALL_DIR
        unzip -q -o "$BINARY_NAME"
        mv -v "anonbird_ui_${OS_TYPE}/" "$INSTALL_DIR/" || \
          mv -v "anonbird_ui_${OS_TYPE}_${ARCH}/" "$INSTALL_DIR/"
    else
        ${SUDO} mkdir -p "$INSTALL_DIR"
        tar -xzvf "$BINARY_NAME"
        ${SUDO} mv "${1%_"${BINARY_BASE_NAME}"}" "$INSTALL_DIR/"
        normalize_installed_binary "${INSTALL_DIR%/}/$1"
    fi
}

add_apt_repo() {
    echo "AnonBird does not configure old upstream apt repositories."
    echo "Using AnonBird release binaries from ${RELEASE_BASE_URL}."
    PACKAGE_MANAGER="bin"
}

add_rpm_repo() {
    echo "AnonBird does not configure old upstream RPM repositories."
    echo "Using AnonBird release binaries from ${RELEASE_BASE_URL}."
    PACKAGE_MANAGER="bin"
}

prepare_tun_module() {
  # Create the necessary file structure for /dev/net/tun
  if [ ! -c /dev/net/tun ]; then
    if [ ! -d /dev/net ]; then
      mkdir -m 755 /dev/net
    fi
    mknod /dev/net/tun c 10 200
    chmod 0755 /dev/net/tun
  fi

  # Load the tun module if not already loaded
  if ! lsmod | grep -q "^tun\s"; then
    insmod /lib/modules/tun.ko
  fi
}

install_native_binaries() {
    # Checks  for supported architecture
    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
        ;;
        i?86|x86)
            ARCH="386"
        ;;
        aarch64|arm64)
            ARCH="arm64"
        ;;
        *)
            echo "Architecture ${ARCH} not supported"
            exit 2
        ;;
    esac

    # download and copy binaries to INSTALL_DIR
    download_release_binary "$CLI_APP"
    if ! $SKIP_UI_APP; then
        download_release_binary "$UI_APP"
    fi
}

create_compat_symlink() {
    if ! is_true "$COMPAT_SYMLINK"; then
        return 0
    fi

    if [ "$OS_TYPE" != "linux" ]; then
        echo "Compatibility symlink is only supported on Linux; skipping netbird -> anonbird"
        return 0
    fi

    target="${INSTALL_DIR%/}/$CLI_APP"
    link="${INSTALL_DIR%/}/netbird"

    if [ ! -x "$target" ]; then
        echo "AnonBird binary not found at $target; cannot create compatibility symlink" >&2
        return 1
    fi

    if [ -e "$link" ] || [ -L "$link" ]; then
        current_target="$(readlink "$link" 2>/dev/null || true)"
        if [ "$current_target" = "$target" ]; then
            echo "Compatibility symlink already exists: $link -> $target"
            return 0
        fi
        if ! is_true "$FORCE_COMPAT_SYMLINK"; then
            echo "Not overwriting existing $link. Set ANONBIRD_COMPAT_SYMLINK_FORCE=true or pass --force-compat-symlink to replace it." >&2
            return 0
        fi
        ${SUDO} rm -f "$link"
    fi

    ${SUDO} ln -s "$target" "$link"
    echo "Created temporary compatibility symlink: $link -> $target"
}

# Handle macOS .pkg installer
install_pkg() {
  case "$(uname -m)" in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported macOS arch: $(uname -m)" >&2; exit 1 ;;
  esac

  if [ -z "${ANONBIRD_MACOS_PKG_URL:-}" ]; then
    echo "ANONBIRD_MACOS_PKG_URL is required for macOS pkg installation."
    echo "Set USE_BIN_INSTALL=true to install AnonBird release binaries instead."
    exit 1
  fi
  echo "Downloading AnonBird macOS installer from ${ANONBIRD_MACOS_PKG_URL}"
  curl -fsSL -o /tmp/anonbird.pkg "${ANONBIRD_MACOS_PKG_URL}"
  ${SUDO} installer -pkg /tmp/anonbird.pkg -target /
  rm -f /tmp/anonbird.pkg
}

check_use_bin_variable() {
    if [ "${USE_BIN_INSTALL}-x" = "true-x" ]; then
      echo "The installation will be performed using binary files"
      return 0
    fi
    return 1
}

install_anonbird() {
    if [ -x "$(command -v anonbird)" ]; then
      status_output="$(anonbird status 2>&1 || true)"

      if echo "$status_output" | grep -q 'failed to connect to daemon error: context deadline exceeded'; then
          echo "Warning: could not reach AnonBird daemon (timeout), proceeding anyway"
      else
          if echo "$status_output" | grep -q 'Management: Connected' && \
              echo "$status_output" | grep -q 'Signal: Connected'; then
              echo "AnonBird service is running, please stop it before proceeding"
              exit 1
          fi

          if [ -n "$status_output" ]; then
              echo "AnonBird seems to be installed already, please remove it before proceeding"
              exit 1
          fi
      fi
    fi

    # Run the installation, if a desktop environment is not detected
    # only the CLI will be installed
    case "$PACKAGE_MANAGER" in
    apt)
        add_apt_repo
        install_native_binaries
    ;;
    yum)
        add_rpm_repo
        install_native_binaries
    ;;
    dnf)
        add_rpm_repo
        install_native_binaries
    ;;
    rpm-ostree)
        add_rpm_repo
        install_native_binaries
    ;;
    pkg)
        # Check if the package is already installed
        if [ -f /Library/Receipts/anonbird.pkg ]; then
            echo "AnonBird is already installed. Please remove it before proceeding."
            exit 1
        fi

        # Install the package
        install_pkg
    ;;
    brew)
        if [ -z "${ANONBIRD_HOMEBREW_FORMULA:-}" ]; then
            echo "ANONBIRD_HOMEBREW_FORMULA is required for Homebrew installation."
            echo "Set USE_BIN_INSTALL=true to install AnonBird release binaries instead."
            exit 1
        fi
        if brew ls --versions anonbird >/dev/null 2>&1; then
            echo "Removing existing anonbird client"

            # Stop and uninstall daemon service:
            anonbird service stop
            anonbird service uninstall

            # Unlink the app
            brew unlink anonbird
        fi

        brew install "${ANONBIRD_HOMEBREW_FORMULA}"
        if ! $SKIP_UI_APP && [ -n "${ANONBIRD_HOMEBREW_UI_FORMULA:-}" ]; then
            brew install --cask "${ANONBIRD_HOMEBREW_UI_FORMULA}"
        fi
    ;;
    *)
      if [ "$OS_NAME" = "nixos" ];then
        echo "Please add AnonBird to your NixOS configuration.nix directly:"
			  echo ""
			  echo "# Build AnonBird from ${PROJECT_URL} or use a pinned AnonBird package overlay."

        if ! $SKIP_UI_APP; then
          echo "# Add anonbird-ui from the same overlay if you need the desktop UI."
        fi

        echo "Build and apply new configuration:"
        echo ""
        echo "${SUDO} nixos-rebuild switch"
			  exit 0
      fi

        install_native_binaries
    ;;
    esac

    if [ "$OS_NAME" = "synology" ]; then
        prepare_tun_module
    fi

    # Add package manager to config
    ${SUDO} mkdir -p "$CONFIG_FOLDER"
    echo "package_manager=$PACKAGE_MANAGER" | ${SUDO} tee "$CONFIG_FILE" > /dev/null
    remove_shadowing_legacy_binary
    create_compat_symlink

    # Load and start anonbird service
    if ! is_true "$SKIP_SERVICE_INSTALL" && [ "$PACKAGE_MANAGER" != "rpm-ostree" ] && [ "$PACKAGE_MANAGER" != "pkg" ]; then
        if ! run_installed_anonbird service install 2>&1; then
            echo "AnonBird service has already been loaded"
        fi
        if is_true "$SKIP_SERVICE_START"; then
            echo "AnonBird service installed but not started because ANONBIRD_SKIP_SERVICE_START is enabled"
        else
            if ! run_installed_anonbird service start 2>&1; then
                echo "AnonBird service has already been started"
            fi
        fi
    elif is_true "$SKIP_SERVICE_INSTALL"; then
        echo "AnonBird service install/start skipped because ANONBIRD_SKIP_SERVICE is enabled"
    fi


    echo "Installation has been finished. To connect, you need to run AnonBird by executing the following command:"
    echo ""
    echo "anonbird up"
}

is_semver_like() {
    echo "$1" | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$'
}

semver_core() {
    echo "$1" | sed 's/^v//' | sed 's/+.*//' | cut -d- -f1
}

semver_prerelease() {
    version="$(echo "$1" | sed 's/^v//' | sed 's/+.*//')"
    case "$version" in
        *-*) echo "${version#*-}" ;;
        *) echo "" ;;
    esac
}

compare_semver_core() {
    left="$(semver_core "$1")"
    right="$(semver_core "$2")"
    IFS=. read -r left_major left_minor left_patch <<EOF
$left
EOF
    IFS=. read -r right_major right_minor right_patch <<EOF
$right
EOF
    for part in major minor patch; do
        eval "left_value=\${left_${part}}"
        eval "right_value=\${right_${part}}"
        if [ "$left_value" -gt "$right_value" ]; then
            echo 1
            return 0
        fi
        if [ "$left_value" -lt "$right_value" ]; then
            echo -1
            return 0
        fi
    done
    echo 0
}

prerelease_greater_equal() {
    printf '%s\n%s\n' "$2" "$1" | sort -V -c >/dev/null 2>&1
}

version_greater_equal() {
    core_compare="$(compare_semver_core "$1" "$2")"
    if [ "$core_compare" -gt 0 ]; then
        return 0
    fi
    if [ "$core_compare" -lt 0 ]; then
        return 1
    fi

    left_pre="$(semver_prerelease "$1")"
    right_pre="$(semver_prerelease "$2")"
    if [ -z "$left_pre" ] && [ -n "$right_pre" ]; then
        return 0
    fi
    if [ -n "$left_pre" ] && [ -z "$right_pre" ]; then
        return 1
    fi
    if [ -z "$left_pre" ] && [ -z "$right_pre" ]; then
        return 0
    fi
    prerelease_greater_equal "$left_pre" "$right_pre"
}

should_update_version() {
  latest_version="$1"
  installed_version="$2"

  if [ "$latest_version" = "$installed_version" ]; then
    return 1
  fi
  if ! is_semver_like "$installed_version"; then
    return 0
  fi
  version_greater_equal "$latest_version" "$installed_version"
}

anonbird_binary_path() {
  if [ -n "$INSTALL_DIR" ]; then
    echo "${INSTALL_DIR%/}/${CLI_APP}"
  else
    command -v "$CLI_APP" 2>/dev/null || echo "/usr/bin/${CLI_APP}"
  fi
}

run_installed_anonbird() {
  binary_path="$(anonbird_binary_path)"
  if ! ${SUDO} test -x "$binary_path"; then
    binary_path="$(command -v "$CLI_APP" 2>/dev/null || true)"
  fi
  if [ -z "$binary_path" ]; then
    echo "AnonBird binary not found" >&2
    return 1
  fi
  ${SUDO} "$binary_path" "$@"
}

restore_active_connection_after_update() {
  was_service_active="$1"
  if ! is_true "$was_service_active" || is_true "$SKIP_SERVICE_START"; then
    return 0
  fi

  binary_path="$(anonbird_binary_path)"
  if ! ${SUDO} test -x "$binary_path"; then
    return 0
  fi

  echo "Restoring active AnonBird connection after update"
  if command -v timeout >/dev/null 2>&1; then
    if ! ${SUDO} timeout 120 "$binary_path" up --daemon-addr unix:///var/run/anonbird.sock; then
      echo "Warning: AnonBird update completed, but automatic reconnect did not finish. Run 'anonbird up' manually if the client stays disconnected." >&2
    fi
  elif ! run_installed_anonbird up --daemon-addr unix:///var/run/anonbird.sock; then
    echo "Warning: AnonBird update completed, but automatic reconnect did not finish. Run 'anonbird up' manually if the client stays disconnected." >&2
  fi
}

is_bin_package_manager() {
  if ${SUDO} test -f "$1" && ${SUDO} grep -q "package_manager=bin" "$1" ; then
    return 0
  else
    return 1
  fi
}

package_manager_owns_file() {
  file_path="$1"
  case "$PACKAGE_MANAGER" in
    apt)
      command -v dpkg-query >/dev/null 2>&1 && dpkg-query -S "$file_path" >/dev/null 2>&1
    ;;
    dnf|yum|rpm-ostree)
      command -v rpm >/dev/null 2>&1 && rpm -qf "$file_path" >/dev/null 2>&1
    ;;
    *)
      return 1
    ;;
  esac
}

is_release_binary_install() {
  if ${SUDO} test -f "$1"; then
    # AnonBird fork installs GitHub release binaries on Linux package-manager
    # distros too; the package-manager value records platform detection only.
    ${SUDO} grep -Eq "package_manager=(bin|apt|dnf|yum|rpm-ostree)" "$1"
    return $?
  fi

  binary_path="$(anonbird_binary_path)"
  if ${SUDO} test -x "$binary_path" && ! package_manager_owns_file "$binary_path"; then
    return 0
  fi
  return 1
}

remove_shadowing_legacy_binary() {
  if [ "$OS_TYPE" != "linux" ]; then
    return 0
  fi

  target_path="$(anonbird_binary_path)"
  resolved_path="$(command -v "$CLI_APP" 2>/dev/null || true)"
  if [ -z "$resolved_path" ] || [ "$resolved_path" = "$target_path" ]; then
    return 0
  fi
  if ! ${SUDO} test -f "$resolved_path"; then
    return 0
  fi
  if package_manager_owns_file "$resolved_path"; then
    return 0
  fi

  backup_path="${resolved_path}.pre-anonbird-update.$(date +%Y%m%d%H%M%S)"
  ${SUDO} mv "$resolved_path" "$backup_path"
  hash -r 2>/dev/null || true
  echo "Moved shadowing legacy AnonBird binary: $resolved_path -> $backup_path"
}

stop_running_anonbird_ui() {
  NB_UI_PROC=$(ps -ef | grep "[a]nonbird-ui" | awk '{print $2}')
  if [ -n "$NB_UI_PROC" ]; then
    echo "AnonBird UI is running with PID $NB_UI_PROC. Stopping it..."
    kill -9 "$NB_UI_PROC"
  fi
}

update_anonbird() {
  if is_release_binary_install "$CONFIG_FILE"; then
    latest_release=$(get_release "$ANONBIRD_RELEASE")
    latest_version=${latest_release#v}
    installed_version=$(run_installed_anonbird version)

    if [ "$latest_version" = "$installed_version" ]; then
      echo "Installed AnonBird version ($installed_version) is up-to-date"
      exit 0
    fi

    if should_update_version "$latest_version" "$installed_version"; then
      echo "AnonBird new version ($latest_version) available. Updating..."
      echo ""
      echo "Initiating AnonBird update. This will stop the anonbird service and restart it after the update"

      service_was_active=false
      if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet anonbird.service; then
        service_was_active=true
      fi
      run_installed_anonbird service stop || true
      run_installed_anonbird service uninstall || true
      stop_running_anonbird_ui
      install_native_binaries
      remove_shadowing_legacy_binary
      ${SUDO} mkdir -p "$CONFIG_FOLDER"
      echo "package_manager=$PACKAGE_MANAGER" | ${SUDO} tee "$CONFIG_FILE" > /dev/null
      create_compat_symlink

      run_installed_anonbird service install
      if is_true "$SKIP_SERVICE_START"; then
        echo "AnonBird service update completed; start skipped because ANONBIRD_SKIP_SERVICE_START is enabled"
      else
        run_installed_anonbird service start
        restore_active_connection_after_update "$service_was_active"
      fi
    else
      echo "Installed AnonBird version ($installed_version) is newer than requested version ($latest_version); skipping update"
    fi
  else
     echo "AnonBird installation was done using a package manager. Please use your system's package manager to update"
  fi
}

# Checks if SKIP_UI_APP env is set
if [ -z "$SKIP_UI_APP" ]; then
    SKIP_UI_APP=false
else
    if $SKIP_UI_APP; then
      echo "SKIP_UI_APP has been set to true in the environment"
      echo "AnonBird UI installation will be omitted based on your preference"
    fi
fi

# Identify OS name and default package manager
if type uname >/dev/null 2>&1; then
	case "$(uname)" in
        Linux)
          OS_TYPE="linux"
          UNAME_OUTPUT="$(uname -a)"
          if echo "$UNAME_OUTPUT" | grep -qi "synology"; then
            OS_NAME="synology"
            set_default_install_dir "/usr/local/bin"
            PACKAGE_MANAGER="bin"
            SKIP_UI_APP=true
          else
            if [ -f /etc/os-release ]; then
              OS_NAME="$(. /etc/os-release && echo "$ID")"
              set_default_install_dir "/usr/bin"

              # Allow AnonBird UI installation for compatible CPU architectures only
              if [ "$ARCH" != "amd64" ] && [ "$ARCH" != "arm64" ] \
                  && [ "$ARCH" != "x86_64" ];then
                  SKIP_UI_APP=true
                  echo "AnonBird UI installation will be omitted as $ARCH is not a compatible architecture"
              fi

              # Allow AnonBird UI installation only when Linux runs a desktop environment
              if [ -z "$XDG_CURRENT_DESKTOP" ];then
                  SKIP_UI_APP=true
                  echo "AnonBird UI installation will be omitted as Linux does not run desktop environment"
              fi

              # Check the availability of a compatible package manager
              if check_use_bin_variable; then
                  PACKAGE_MANAGER="bin"
              elif [ -x "$(command -v apt-get)" ]; then
                  PACKAGE_MANAGER="apt"
                  echo "apt detected; AnonBird will install release binaries and avoid upstream package repositories"
              elif [ -x "$(command -v dnf)" ]; then
                  PACKAGE_MANAGER="dnf"
                  echo "dnf detected; AnonBird will install release binaries and avoid upstream package repositories"
              elif [ -x "$(command -v rpm-ostree)" ]; then
                  PACKAGE_MANAGER="rpm-ostree"
                  echo "rpm-ostree detected; AnonBird will install release binaries and avoid upstream package repositories"
              elif [ -x "$(command -v yum)" ]; then
                  PACKAGE_MANAGER="yum"
                  echo "yum detected; AnonBird will install release binaries and avoid upstream package repositories"
              fi
            else
              echo "Unable to determine OS type from /etc/os-release"
              exit 1
            fi
          fi


		;;
		Darwin)
            OS_NAME="macos"
			OS_TYPE="darwin"
            set_default_install_dir "/usr/local/bin"

            # Check the availability of a compatible package manager
            if check_use_bin_variable; then
                PACKAGE_MANAGER="bin"
            else
              PACKAGE_MANAGER="bin"
            fi
		;;
	esac
fi

if [ "${UPDATE_ANONBIRD}-x" = "true-x" ]; then
  UPDATE_FLAG="--update"
fi

case "$UPDATE_FLAG" in
    --update)
      update_anonbird
    ;;
    *)
      install_anonbird
esac
