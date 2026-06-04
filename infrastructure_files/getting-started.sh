#!/bin/bash

set -e

# AnonBird Getting Started with Embedded IdP (Dex)
# This script sets up AnonBird with the embedded Dex identity provider
# No separate Dex container or reverse proxy needed - IdP is built into management server

# Sed pattern to strip base64 padding characters
SED_STRIP_PADDING='s/=//g'

# Constants for repeated string literals
readonly MSG_STARTING_SERVICES="\nStarting AnonBird services\n"
readonly MSG_DONE="\nDone!\n"
readonly MSG_NEXT_STEPS="Next steps:"
readonly MSG_SEPARATOR="=========================================="
readonly DEFAULT_WORKDIR="/opt/anonbird"

############################################
# CLI Arguments
############################################

print_usage() {
  cat <<'EOF'
AnonBird one-command self-host installer

Usage:
  getting-started.sh [options]

Interactive production quickstart:
  curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh | bash

Unattended production quickstart:
  curl -fsSL https://github.com/Cr0me1ve/anonbird/releases/latest/download/getting-started.sh \
    | bash -s -- --domain anonbird.your-domain.com --email admin@your-domain.com --yes

Options:
  --domain DOMAIN              Public dashboard/management domain.
  --use-ip                     Use this host's primary IP instead of a DNS name.
  --email EMAIL                Let's Encrypt/ACME email for built-in Traefik.
  --proxy TYPE                 Reverse proxy: traefik, external-traefik, nginx, npm, caddy, manual.
  --enable-proxy               Enable the AnonBird reverse proxy service.
  --enable-crowdsec            Enable CrowdSec for reverse proxy protection.
  --enable-clearnet-stun       Expose STUN/UDP for explicit non-anonymous legacy clients.
  --bind-localhost             Bind exposed container ports to 127.0.0.1.
  --bind-public                Bind exposed container ports to 0.0.0.0.
  --traefik-network NAME       External Traefik Docker network.
  --traefik-entrypoint NAME    External Traefik HTTPS entrypoint.
  --traefik-certresolver NAME  External Traefik certificate resolver.
  --external-proxy-network N   Docker network for nginx/npm/caddy reverse proxy.
  --anonymous-transport MODE   Peer management transport: tor, i2p, both, manual, none (default: tor).
  --peer-management-endpoint URL
                              Existing onion/I2P management endpoint for manual mode.
  --workdir DIR                Deployment working directory (default: /opt/anonbird).
  --render-only                Render files and exit without starting containers.
  --preflight-only             Check required AnonBird Docker images and exit.
  --skip-image-preflight       Start without checking custom AnonBird images first.
  --yes, -y                    Non-interactive mode; fail instead of prompting.
  --help, -h                   Show this help.

Environment aliases:
  ANONBIRD_DOMAIN              Same as --domain when NETBIRD_DOMAIN is unset.
  ANONBIRD_ADMIN_EMAIL         Same as --email when TRAEFIK_ACME_EMAIL is unset.
  ANONBIRD_NONINTERACTIVE=true Same as --yes.
  ANONBIRD_WORKDIR             Deployment working directory (default: /opt/anonbird).
  ANONBIRD_ANONYMOUS_TRANSPORT
                                Peer management transport: tor, i2p, both, manual, none.
  ANONBIRD_PEER_MANAGEMENT_ENDPOINT
                                Existing onion/I2P endpoint for manual mode.
  ANONBIRD_I2PD_IMAGE          Override managed I2P sidecar image.
  ANONBIRD_DASHBOARD_IMAGE     Override dashboard image.
  ANONBIRD_SERVER_IMAGE        Override combined server image.
  ANONBIRD_PROXY_IMAGE         Override reverse proxy image.
  ANONBIRD_ENABLE_CLEARNET_STUN=true
                                Same as --enable-clearnet-stun.
  ANONBIRD_SKIP_IMAGE_PREFLIGHT=true
                                Same as --skip-image-preflight.
EOF
}

print_interactive_intro() {
  cat > /dev/stderr <<'EOF'
AnonBird one-command self-host installer

The installer will ask for:
  - public dashboard/management domain;
  - automatic Tor/I2P peer management endpoint mode;
  - reverse proxy mode;
  - Let's Encrypt email when built-in Traefik is used;
  - optional AnonBird Proxy and CrowdSec settings.

Generated files and Docker Compose state live in /opt/anonbird by default.

For automation, run with --domain, --email and --yes.

EOF
}

proxy_choice_from_name() {
  case "$1" in
    0|traefik|builtin-traefik) echo "0" ;;
    1|external-traefik) echo "1" ;;
    2|nginx) echo "2" ;;
    3|npm|nginx-proxy-manager) echo "3" ;;
    4|caddy|external-caddy) echo "4" ;;
    5|manual|other) echo "5" ;;
    *)
      echo "Unsupported proxy type: $1" > /dev/stderr
      print_usage > /dev/stderr
      exit 2
      ;;
  esac
}

anonymous_transport_from_name() {
  case "$1" in
    tor|onion|0) echo "tor" ;;
    i2p|1) echo "i2p" ;;
    both|2) echo "both" ;;
    manual|existing|3) echo "manual" ;;
    none|disabled|off|4) echo "none" ;;
    *)
      echo "Unsupported anonymous transport mode: $1" > /dev/stderr
      print_usage > /dev/stderr
      exit 2
      ;;
  esac
}

require_arg() {
  local flag="$1"
  local value="${2:-}"
  if [[ -z "$value" || "$value" == --* ]]; then
    echo "${flag} requires a value." > /dev/stderr
    print_usage > /dev/stderr
    exit 2
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --domain)
        require_arg "$1" "${2:-}"
        NETBIRD_DOMAIN="$2"
        shift 2
        ;;
      --use-ip)
        NETBIRD_DOMAIN="use-ip"
        shift
        ;;
      --email|--admin-email|--acme-email)
        require_arg "$1" "${2:-}"
        TRAEFIK_ACME_EMAIL="$2"
        shift 2
        ;;
      --proxy)
        require_arg "$1" "${2:-}"
        REVERSE_PROXY_TYPE=$(proxy_choice_from_name "$2")
        shift 2
        ;;
      --enable-proxy)
        ENABLE_PROXY="true"
        shift
        ;;
      --enable-crowdsec)
        ENABLE_PROXY="true"
        ENABLE_CROWDSEC="true"
        shift
        ;;
      --enable-clearnet-stun)
        ENABLE_CLEARNET_STUN="true"
        shift
        ;;
      --anonymous-transport|--peer-transport)
        require_arg "$1" "${2:-}"
        ANONBIRD_ANONYMOUS_TRANSPORT=$(anonymous_transport_from_name "$2")
        shift 2
        ;;
      --bind-localhost)
        BIND_LOCALHOST_ONLY="true"
        shift
        ;;
      --bind-public)
        BIND_LOCALHOST_ONLY="false"
        shift
        ;;
      --traefik-network)
        require_arg "$1" "${2:-}"
        TRAEFIK_EXTERNAL_NETWORK="$2"
        REVERSE_PROXY_TYPE="1"
        shift 2
        ;;
      --traefik-entrypoint)
        require_arg "$1" "${2:-}"
        TRAEFIK_ENTRYPOINT="$2"
        shift 2
        ;;
      --traefik-certresolver)
        require_arg "$1" "${2:-}"
        TRAEFIK_CERTRESOLVER="$2"
        shift 2
        ;;
      --external-proxy-network)
        require_arg "$1" "${2:-}"
        EXTERNAL_PROXY_NETWORK="$2"
        shift 2
        ;;
      --peer-management-endpoint|--anonymous-management-endpoint)
        require_arg "$1" "${2:-}"
        ANONBIRD_PEER_MANAGEMENT_ENDPOINT="$2"
        ANONBIRD_ANONYMOUS_TRANSPORT="manual"
        shift 2
        ;;
      --workdir)
        require_arg "$1" "${2:-}"
        ANONBIRD_WORKDIR="$2"
        shift 2
        ;;
      --render-only)
        RENDER_ONLY="true"
        NONINTERACTIVE="true"
        shift
        ;;
      --preflight-only)
        PREFLIGHT_ONLY="true"
        NONINTERACTIVE="true"
        shift
        ;;
      --skip-image-preflight)
        SKIP_IMAGE_PREFLIGHT="true"
        shift
        ;;
      --yes|-y|--non-interactive)
        NONINTERACTIVE="true"
        shift
        ;;
      --help|-h)
        print_usage
        exit 0
        ;;
      *)
        echo "Unknown option: $1" > /dev/stderr
        print_usage > /dev/stderr
        exit 2
        ;;
    esac
  done
}

############################################
# Utility Functions
############################################

check_docker_compose() {
  if command -v docker-compose &> /dev/null
  then
      echo "docker-compose"
      return
  fi
  if docker compose --help &> /dev/null
  then
      echo "docker compose"
      return
  fi

  echo "docker-compose is not installed or not in PATH. Please follow the steps from the official guide: https://docs.docker.com/engine/install/" > /dev/stderr
  exit 1
}

check_jq() {
  if ! command -v jq &> /dev/null
  then
    echo "jq is not installed or not in PATH, please install with your package manager. e.g. sudo apt install jq" > /dev/stderr
    exit 1
  fi
  return 0
}

ensure_workdir() {
  local original_dir
  original_dir=$(pwd -P)

  if [[ -z "${ANONBIRD_WORKDIR:-}" ]]; then
    ANONBIRD_WORKDIR="$DEFAULT_WORKDIR"
  fi

  case "$ANONBIRD_WORKDIR" in
    /*) ;;
    *)
      echo "ANONBIRD_WORKDIR must be an absolute path. Got: $ANONBIRD_WORKDIR" > /dev/stderr
      exit 1
      ;;
  esac

  if [[ -e "$ANONBIRD_WORKDIR" && ! -d "$ANONBIRD_WORKDIR" ]]; then
    echo "ANONBIRD_WORKDIR exists but is not a directory: $ANONBIRD_WORKDIR" > /dev/stderr
    exit 1
  fi

  if ! mkdir -p "$ANONBIRD_WORKDIR" 2>/dev/null; then
    if [[ "$(id -u)" -ne 0 ]] && command -v sudo >/dev/null 2>&1; then
      echo "Creating $ANONBIRD_WORKDIR requires elevated privileges; sudo may ask for your password."
      sudo mkdir -p "$ANONBIRD_WORKDIR"
      sudo chown "$(id -u):$(id -g)" "$ANONBIRD_WORKDIR"
    else
      echo "Cannot create $ANONBIRD_WORKDIR. Re-run as root or install sudo." > /dev/stderr
      exit 1
    fi
  fi

  if [[ ! -w "$ANONBIRD_WORKDIR" ]]; then
    if [[ "$(id -u)" -ne 0 ]] && command -v sudo >/dev/null 2>&1; then
      echo "Making $ANONBIRD_WORKDIR writable by the current user; sudo may ask for your password."
      sudo chown "$(id -u):$(id -g)" "$ANONBIRD_WORKDIR"
    fi
  fi

  if [[ ! -w "$ANONBIRD_WORKDIR" ]]; then
    echo "Cannot write to $ANONBIRD_WORKDIR." > /dev/stderr
    exit 1
  fi

  local target_dir
  target_dir=$(cd "$ANONBIRD_WORKDIR" && pwd -P)
  ANONBIRD_WORKDIR="$target_dir"

  if [[ "$original_dir" != "$ANONBIRD_WORKDIR" && -f "$original_dir/config.yaml" && ! -f "$ANONBIRD_WORKDIR/config.yaml" ]]; then
    echo "Existing AnonBird configuration was found in $original_dir, but new installs use $ANONBIRD_WORKDIR." > /dev/stderr
    echo "To avoid starting a second stack with the same container names, move the existing files first:" > /dev/stderr
    echo "  sudo mkdir -p $ANONBIRD_WORKDIR" > /dev/stderr
    echo "  sudo cp -a $original_dir/docker-compose.yml $original_dir/dashboard.env $original_dir/config.yaml $ANONBIRD_WORKDIR/" > /dev/stderr
    echo "Then run from $ANONBIRD_WORKDIR, or set ANONBIRD_WORKDIR=$original_dir to keep the existing location." > /dev/stderr
    exit 1
  fi

  cd "$ANONBIRD_WORKDIR"
  echo "Using AnonBird working directory: $ANONBIRD_WORKDIR"
  return 0
}

run_with_timeout() {
  local timeout_seconds="$1"
  shift

  "$@" >/dev/null 2>&1 &
  local command_pid=$!
  local elapsed=0

  while kill -0 "$command_pid" 2>/dev/null; do
    if [[ "$elapsed" -ge "$timeout_seconds" ]]; then
      kill "$command_pid" >/dev/null 2>&1 || true
      sleep 1
      kill -9 "$command_pid" >/dev/null 2>&1 || true
      wait "$command_pid" 2>/dev/null || true
      return 124
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  wait "$command_pid"
  return $?
}

check_docker_image_available() {
  local image="$1"
  local timeout_seconds="${ANONBIRD_IMAGE_PREFLIGHT_TIMEOUT:-20}"

  if docker image inspect "$image" >/dev/null 2>&1; then
    return 0
  fi

  run_with_timeout "$timeout_seconds" docker manifest inspect "$image"
  local manifest_status=$?
  if [[ "$manifest_status" -eq 0 ]]; then
    return 0
  fi

  return "$manifest_status"
}

preflight_required_images() {
  if [[ "$SKIP_IMAGE_PREFLIGHT" == "true" ]]; then
    echo "Skipping AnonBird Docker image preflight because ANONBIRD_SKIP_IMAGE_PREFLIGHT=true or --skip-image-preflight was set."
    return 0
  fi

  local images=("$DASHBOARD_IMAGE" "$NETBIRD_SERVER_IMAGE")
  if [[ "$ENABLE_PROXY" == "true" ]]; then
    images+=("$NETBIRD_PROXY_IMAGE")
  fi
  if managed_i2p_enabled; then
    images+=("$ANONBIRD_I2PD_IMAGE")
  fi

  local missing_images=()
  local image
  for image in "${images[@]}"; do
    if ! check_docker_image_available "$image"; then
      missing_images+=("$image")
    fi
  done

  if [[ ${#missing_images[@]} -eq 0 ]]; then
    echo "AnonBird Docker image preflight passed."
    return 0
  fi

  echo "ERROR: AnonBird Docker image preflight failed." > /dev/stderr
  echo "The following required image(s) are not available locally and could not be inspected remotely:" > /dev/stderr
  for image in "${missing_images[@]}"; do
    echo "  - $image" > /dev/stderr
  done
  echo "" > /dev/stderr
  echo "Publish the release images, run 'docker login ghcr.io' if they are private, or override them with:" > /dev/stderr
  echo "  ANONBIRD_DASHBOARD_IMAGE=<image>" > /dev/stderr
  echo "  ANONBIRD_SERVER_IMAGE=<image>" > /dev/stderr
  echo "  ANONBIRD_PROXY_IMAGE=<image>  # only when --enable-proxy is used" > /dev/stderr
  echo "" > /dev/stderr
  echo "For a local development stack with images already handled another way, set ANONBIRD_SKIP_IMAGE_PREFLIGHT=true." > /dev/stderr
  exit 1
}

clearnet_stun_enabled() {
  [[ "${ENABLE_CLEARNET_STUN:-false}" == "true" ]]
}

managed_tor_enabled() {
  [[ "${ANONBIRD_ANONYMOUS_TRANSPORT:-tor}" == "tor" || "${ANONBIRD_ANONYMOUS_TRANSPORT:-tor}" == "both" ]]
}

managed_i2p_enabled() {
  [[ "${ANONBIRD_ANONYMOUS_TRANSPORT:-tor}" == "i2p" || "${ANONBIRD_ANONYMOUS_TRANSPORT:-tor}" == "both" ]]
}

managed_anonymous_transport_enabled() {
  managed_tor_enabled || managed_i2p_enabled
}

render_stun_ports_section() {
  if clearnet_stun_enabled; then
    cat <<EOF
    ports:
      - '$NETBIRD_STUN_PORT:$NETBIRD_STUN_PORT/udp'
EOF
  fi
}

render_stun_port_line() {
  if clearnet_stun_enabled; then
    echo "      - '$NETBIRD_STUN_PORT:$NETBIRD_STUN_PORT/udp'"
  fi
}

render_stun_ports_yaml() {
  if clearnet_stun_enabled; then
    cat <<EOF
  stunPorts:
    - $NETBIRD_STUN_PORT
EOF
  else
    echo "  stunPorts: []"
  fi
}

render_anonymous_services() {
  local service_networks="$1"

  if managed_tor_enabled; then
    cat <<EOF

  # Managed Tor onion service for peer management commands
  tor:
    build:
      context: .
      dockerfile: tor.Dockerfile
    image: anonbird-tor:local
    container_name: anonbird-tor
    restart: unless-stopped
    networks: $service_networks
    volumes:
      - anonbird_tor_data:/var/lib/tor
      - ./torrc:/etc/tor/torrc:ro
    logging:
      driver: "json-file"
      options:
        max-size: "200m"
        max-file: "2"
EOF
  fi

  if managed_i2p_enabled; then
    cat <<EOF

  # Managed I2P server tunnel for peer management commands
  i2pd:
    image: $ANONBIRD_I2PD_IMAGE
    container_name: anonbird-i2pd
    restart: unless-stopped
    networks: $service_networks
    volumes:
      - anonbird_i2pd_data:/home/i2pd/data
      - ./i2pd.conf:/home/i2pd/data/i2pd.conf:ro
      - ./i2pd-tunnels.conf:/home/i2pd/data/tunnels.conf:ro
    logging:
      driver: "json-file"
      options:
        max-size: "200m"
        max-file: "2"
EOF
  fi
}

render_anonymous_volumes() {
  if managed_tor_enabled; then
    cat <<EOF
  anonbird_tor_data:
EOF
  fi
  if managed_i2p_enabled; then
    cat <<EOF
  anonbird_i2pd_data:
EOF
  fi
}

get_main_ip_address() {
  if [[ "$OSTYPE" == "darwin"* ]]; then
    interface=$(route -n get default | grep 'interface:' | awk '{print $2}')
    ip_address=$(ifconfig "$interface" | grep 'inet ' | awk '{print $2}')
  else
    interface=$(ip route | grep default | awk '{print $5}' | head -n 1)
    ip_address=$(ip addr show "$interface" | grep 'inet ' | awk '{print $2}' | cut -d'/' -f1)
  fi

  echo "$ip_address"
  return 0
}

check_nb_domain() {
  DOMAIN=$1
  if [[ "$DOMAIN-x" == "-x" ]]; then
    echo "The NETBIRD_DOMAIN variable cannot be empty." > /dev/stderr
    return 1
  fi

  if [[ "$DOMAIN" == "anonbird.example.com" ]]; then
    echo "The NETBIRD_DOMAIN cannot be anonbird.example.com" > /dev/stderr
    return 1
  fi
  return 0
}

check_peer_management_endpoint() {
  local endpoint="${1:-}"
  if [[ -z "$endpoint" ]]; then
    return 0
  fi

  case "$endpoint" in
    http://*.onion|http://*.onion/*|https://*.onion|https://*.onion/*|http://*.i2p|http://*.i2p/*|https://*.i2p|https://*.i2p/*)
      return 0
      ;;
    *)
      echo "Peer management endpoint must be an http(s) .onion or .i2p URL, or left empty." > /dev/stderr
      return 1
      ;;
  esac
}

check_required_peer_management_endpoint() {
  local endpoint="${1:-}"
  if [[ -z "$endpoint" ]]; then
    echo "Peer management endpoint cannot be empty." > /dev/stderr
    return 1
  fi
  check_peer_management_endpoint "$endpoint"
}

read_peer_management_endpoint() {
  local endpoint
  echo "" > /dev/stderr
  echo "Enter the existing Tor/I2P management endpoint that peers should use." > /dev/stderr
  echo "The dashboard can stay on clearnet; copied peer setup commands will use this URL." > /dev/stderr
  echo -n "Peer management endpoint: " > /dev/stderr
  read -r endpoint < /dev/tty
  if ! check_peer_management_endpoint "$endpoint"; then
    read_peer_management_endpoint
    return
  fi
  if [[ -z "$endpoint" ]]; then
    echo "Peer management endpoint cannot be empty in manual mode." > /dev/stderr
    read_peer_management_endpoint
    return
  fi
  echo "$endpoint"
  return 0
}

read_anonymous_transport() {
  local choice
  echo "" > /dev/stderr
  echo "How should peers reach the management server anonymously?" > /dev/stderr
  echo "  [0] Tor onion service (recommended - created automatically)" > /dev/stderr
  echo "  [1] I2P server tunnel (created automatically)" > /dev/stderr
  echo "  [2] Both Tor and I2P (created automatically, Tor used by default)" > /dev/stderr
  echo "  [3] I already have an onion/I2P endpoint" > /dev/stderr
  echo "  [4] Configure later from the dashboard" > /dev/stderr
  echo "" > /dev/stderr
  echo -n "Enter choice [0-4] (default: 0): " > /dev/stderr
  read -r choice < /dev/tty

  if [[ -z "$choice" ]]; then
    choice="0"
  fi

  if [[ ! "$choice" =~ ^[0-4]$ ]]; then
    echo "Invalid choice. Please enter a number between 0 and 4." > /dev/stderr
    read_anonymous_transport
    return
  fi

  anonymous_transport_from_name "$choice"
  return 0
}

read_nb_domain() {
  READ_NETBIRD_DOMAIN=""
  echo -n "Enter the domain you want to use for AnonBird (e.g. anonbird.my-domain.com): " > /dev/stderr
  read -r READ_NETBIRD_DOMAIN < /dev/tty
  if ! check_nb_domain "$READ_NETBIRD_DOMAIN"; then
    read_nb_domain
  fi
  echo "$READ_NETBIRD_DOMAIN"
  return 0
}

read_reverse_proxy_type() {
  echo "" > /dev/stderr
  echo "Which reverse proxy will you use?" > /dev/stderr
  echo "  [0] Traefik (recommended - automatic TLS, included in Docker Compose)" > /dev/stderr
  echo "  [1] Existing Traefik (labels for external Traefik instance)" > /dev/stderr
  echo "  [2] Nginx (generates config template)" > /dev/stderr
  echo "  [3] Nginx Proxy Manager (generates config + instructions)" > /dev/stderr
  echo "  [4] External Caddy (generates Caddyfile snippet)" > /dev/stderr
  echo "  [5] Other/Manual (displays setup documentation)" > /dev/stderr
  echo "" > /dev/stderr
  echo -n "Enter choice [0-5] (default: 0): " > /dev/stderr
  read -r CHOICE < /dev/tty

  if [[ -z "$CHOICE" ]]; then
    CHOICE="0"
  fi

  if [[ ! "$CHOICE" =~ ^[0-5]$ ]]; then
    echo "Invalid choice. Please enter a number between 0 and 5." > /dev/stderr
    read_reverse_proxy_type
    return
  fi

  echo "$CHOICE"
  return 0
}

read_traefik_network() {
  echo "" > /dev/stderr
  echo "If you have an existing Traefik instance, enter its external network name." > /dev/stderr
  echo -n "External network (leave empty to create 'anonbird' network): " > /dev/stderr
  read -r NETWORK < /dev/tty
  echo "$NETWORK"
  return 0
}

read_traefik_entrypoint() {
  echo "" > /dev/stderr
  echo "Enter the name of your Traefik HTTPS entrypoint." > /dev/stderr
  echo -n "HTTPS entrypoint name (default: websecure): " > /dev/stderr
  read -r ENTRYPOINT < /dev/tty
  if [[ -z "$ENTRYPOINT" ]]; then
    ENTRYPOINT="websecure"
  fi
  echo "$ENTRYPOINT"
  return 0
}

read_traefik_certresolver() {
  echo "" > /dev/stderr
  echo "Enter the name of your Traefik certificate resolver (for automatic TLS)." > /dev/stderr
  echo "Leave empty if you handle TLS termination elsewhere or use a wildcard cert." > /dev/stderr
  echo -n "Certificate resolver name (e.g., letsencrypt): " > /dev/stderr
  read -r RESOLVER < /dev/tty
  echo "$RESOLVER"
  return 0
}

read_port_binding_preference() {
  echo "" > /dev/stderr
  echo "Should container ports be bound to localhost only (127.0.0.1)?" > /dev/stderr
  echo "Choose 'yes' if your reverse proxy runs on the same host (more secure)." > /dev/stderr
  echo -n "Bind to localhost only? [Y/n]: " > /dev/stderr
  read -r CHOICE < /dev/tty

  if [[ "$CHOICE" =~ ^[Nn]$ ]]; then
    echo "false"
  else
    echo "true"
  fi
  return 0
}

read_proxy_docker_network() {
  local proxy_name="$1"
  echo "" > /dev/stderr
  echo "Is ${proxy_name} running in Docker?" > /dev/stderr
  echo "If yes, enter the Docker network ${proxy_name} is on (AnonBird will join it)." > /dev/stderr
  echo -n "Docker network (leave empty if not in Docker): " > /dev/stderr
  read -r NETWORK < /dev/tty
  echo "$NETWORK"
  return 0
}

read_enable_proxy() {
  echo "" > /dev/stderr
  echo "Do you want to enable the AnonBird Proxy service?" > /dev/stderr
  echo "The proxy allows you to selectively expose internal AnonBird network resources" > /dev/stderr
  echo "to the internet. You control which resources are exposed through the dashboard." > /dev/stderr
  echo -n "Enable proxy? [y/N]: " > /dev/stderr
  read -r CHOICE < /dev/tty

  if [[ "$CHOICE" =~ ^[Yy]$ ]]; then
    echo "true"
  else
    echo "false"
  fi
  return 0
}

read_enable_crowdsec() {
  echo "" > /dev/stderr
  echo "Do you want to enable CrowdSec IP reputation blocking?" > /dev/stderr
  echo "CrowdSec checks client IPs against a community threat intelligence database" > /dev/stderr
  echo "and blocks known malicious sources before they reach your services." > /dev/stderr
  echo "A local CrowdSec LAPI container will be added to your deployment." > /dev/stderr
  echo -n "Enable CrowdSec? [y/N]: " > /dev/stderr
  read -r CHOICE < /dev/tty

  if [[ "$CHOICE" =~ ^[Yy]$ ]]; then
    echo "true"
  else
    echo "false"
  fi
  return 0
}

read_traefik_acme_email() {
  echo "" > /dev/stderr
  echo "Enter your email for Let's Encrypt certificate notifications." > /dev/stderr
  echo -n "Email address: " > /dev/stderr
  read -r EMAIL < /dev/tty
  if [[ -z "$EMAIL" ]]; then
    echo "Email is required for Let's Encrypt." > /dev/stderr
    read_traefik_acme_email
    return
  fi
  echo "$EMAIL"
  return 0
}

get_bind_address() {
  if [[ "$BIND_LOCALHOST_ONLY" == "true" ]]; then
    echo "127.0.0.1"
  else
    echo "0.0.0.0"
  fi
  return 0
}

get_upstream_host() {
  # Always return 127.0.0.1 for health checks and upstream targets
  # Cannot use 0.0.0.0 as a connection target
  echo "127.0.0.1"
  return 0
}

wait_management_proxy() {
  local proxy_container="${1:-traefik}"
  local use_docker_logs=false
  set +e

  if [[ "$proxy_container" == "detect-traefik" ]]; then
    proxy_container=$(docker ps --format "{{.ID}}\t{{.Image}}\t{{.Ports}}" \
    | awk -F'\t' '$2 ~ /traefik/ && $3 ~ /:(80|443)->/ {print $1; exit}')

    if [[ -z "$proxy_container" ]]; then
      echo "Warning: could not auto-detect Traefik container, log output will be skipped on timeout." > /dev/stderr
    else
      use_docker_logs=true
    fi
  fi

  echo -n "Waiting for AnonBird server to become ready"
  counter=1
  while true; do
    # Check the embedded IdP endpoint through the reverse proxy
    if curl -sk -f -o /dev/null "$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN/oauth2/.well-known/openid-configuration" 2>/dev/null; then
      break
    fi
    if [[ $counter -eq 60 ]]; then
      echo ""
      echo "Taking too long. Checking logs..."
      if [[ -n "$proxy_container" ]]; then
        if [[ "$use_docker_logs" == "true" ]]; then
          docker logs --tail=20 "$proxy_container"
        else
          $DOCKER_COMPOSE_COMMAND logs --tail=20 "$proxy_container"
        fi
      fi
      $DOCKER_COMPOSE_COMMAND logs --tail=20 anonbird-server
    fi
    echo -n " ."
    sleep 2
    counter=$((counter + 1))
  done
  echo " done"
  set -e
  return 0
}

wait_management_direct() {
  set +e
  local upstream_host=$(get_upstream_host)
  echo -n "Waiting for AnonBird server to become ready"
  counter=1
  while true; do
    # Check the embedded IdP endpoint directly (no reverse proxy)
    if curl -sk -f -o /dev/null "http://${upstream_host}:${MANAGEMENT_HOST_PORT}/oauth2/.well-known/openid-configuration" 2>/dev/null; then
      break
    fi
    if [[ $counter -eq 60 ]]; then
      echo ""
      echo "Taking too long. Checking logs..."
      $DOCKER_COMPOSE_COMMAND logs --tail=20 anonbird-server
    fi
    echo -n " ."
    sleep 2
    counter=$((counter + 1))
  done
  echo " done"
  set -e
  return 0
}

############################################
# Initialization and Configuration
############################################

initialize_default_values() {
  if [[ -z "${NETBIRD_DOMAIN:-}" && -n "${ANONBIRD_DOMAIN:-}" ]]; then
    NETBIRD_DOMAIN="$ANONBIRD_DOMAIN"
  fi

  NONINTERACTIVE="${ANONBIRD_NONINTERACTIVE:-false}"
  RENDER_ONLY="${ANONBIRD_RENDER_ONLY:-false}"
  PREFLIGHT_ONLY="${ANONBIRD_PREFLIGHT_ONLY:-false}"
  SKIP_IMAGE_PREFLIGHT="${ANONBIRD_SKIP_IMAGE_PREFLIGHT:-false}"
  ANONBIRD_WORKDIR="${ANONBIRD_WORKDIR:-$DEFAULT_WORKDIR}"

  NETBIRD_PORT=80
  NETBIRD_HTTP_PROTOCOL="http"
  NETBIRD_RELAY_PROTO="rel"
  NETBIRD_RELAY_AUTH_SECRET=$(openssl rand -base64 32 | sed "$SED_STRIP_PADDING")
  # Note: DataStoreEncryptionKey must keep base64 padding (=) for Go's base64.StdEncoding
  DATASTORE_ENCRYPTION_KEY=$(openssl rand -base64 32)
  NETBIRD_STUN_PORT=3478

  # Docker images
  DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-${ANONBIRD_DASHBOARD_IMAGE:-ghcr.io/cr0me1ve/anonbird-dashboard:latest}}"
  # Combined server replaces separate signal, relay, and management containers
  NETBIRD_SERVER_IMAGE="${NETBIRD_SERVER_IMAGE:-${ANONBIRD_SERVER_IMAGE:-ghcr.io/cr0me1ve/anonbird-server:latest}}"
  NETBIRD_PROXY_IMAGE="${NETBIRD_PROXY_IMAGE:-${ANONBIRD_PROXY_IMAGE:-ghcr.io/cr0me1ve/anonbird-reverse-proxy:latest}}"
  ANONBIRD_I2PD_IMAGE="${ANONBIRD_I2PD_IMAGE:-ghcr.io/purplei2p/i2pd:release-2.60.0}"

  # Reverse proxy configuration
  REVERSE_PROXY_TYPE="${ANONBIRD_REVERSE_PROXY_TYPE:-0}"
  REVERSE_PROXY_TYPE=$(proxy_choice_from_name "$REVERSE_PROXY_TYPE")
  TRAEFIK_EXTERNAL_NETWORK="${TRAEFIK_EXTERNAL_NETWORK:-}"
  TRAEFIK_ENTRYPOINT="${TRAEFIK_ENTRYPOINT:-websecure}"
  TRAEFIK_CERTRESOLVER="${TRAEFIK_CERTRESOLVER:-}"
  TRAEFIK_ACME_EMAIL="${TRAEFIK_ACME_EMAIL:-${ANONBIRD_ADMIN_EMAIL:-}}"
  DASHBOARD_HOST_PORT="${DASHBOARD_HOST_PORT:-8080}"
  MANAGEMENT_HOST_PORT="${MANAGEMENT_HOST_PORT:-8081}"  # Combined server port (management + signal + relay)
  BIND_LOCALHOST_ONLY="${BIND_LOCALHOST_ONLY:-true}"
  EXTERNAL_PROXY_NETWORK="${EXTERNAL_PROXY_NETWORK:-}"
  ANONBIRD_PEER_MANAGEMENT_ENDPOINT="${ANONBIRD_PEER_MANAGEMENT_ENDPOINT:-}"
  ANONBIRD_ANONYMOUS_TRANSPORT="${ANONBIRD_ANONYMOUS_TRANSPORT:-tor}"
  ANONBIRD_ANONYMOUS_TRANSPORT=$(anonymous_transport_from_name "$ANONBIRD_ANONYMOUS_TRANSPORT")

  # Traefik static IP within the internal bridge network
  TRAEFIK_IP="172.30.0.10"

  # AnonBird Proxy configuration
  ENABLE_PROXY="${ANONBIRD_ENABLE_PROXY:-false}"
  ENABLE_CLEARNET_STUN="${ANONBIRD_ENABLE_CLEARNET_STUN:-false}"
  PROXY_TOKEN=""

  # CrowdSec configuration
  ENABLE_CROWDSEC="${ANONBIRD_ENABLE_CROWDSEC:-false}"
  CROWDSEC_BOUNCER_KEY=""
  return 0
}

configure_domain() {
  if ! check_nb_domain "$NETBIRD_DOMAIN"; then
    if [[ "$NONINTERACTIVE" == "true" ]]; then
      echo "Set --domain, --use-ip, NETBIRD_DOMAIN, or ANONBIRD_DOMAIN for non-interactive setup." > /dev/stderr
      exit 1
    fi
    NETBIRD_DOMAIN=$(read_nb_domain)
  fi

  if [[ "$NETBIRD_DOMAIN" == "use-ip" ]]; then
    NETBIRD_DOMAIN=$(get_main_ip_address)
    BASE_DOMAIN=$NETBIRD_DOMAIN
  else
    NETBIRD_PORT=443
    NETBIRD_HTTP_PROTOCOL="https"
    NETBIRD_RELAY_PROTO="rels"
    BASE_DOMAIN=$(echo $NETBIRD_DOMAIN | sed -E 's/^[^.]+\.//')
  fi
  return 0
}

configure_reverse_proxy() {
  if [[ "$NONINTERACTIVE" == "true" ]]; then
    if [[ "$REVERSE_PROXY_TYPE" == "0" && "$NETBIRD_HTTP_PROTOCOL" == "https" && -z "$TRAEFIK_ACME_EMAIL" ]]; then
      echo "Built-in Traefik mode needs --email or ANONBIRD_ADMIN_EMAIL for Let's Encrypt." > /dev/stderr
      exit 1
    fi
    return 0
  fi

  # Prompt for reverse proxy type
  REVERSE_PROXY_TYPE=$(read_reverse_proxy_type)

  # Handle built-in Traefik prompts (option 0)
  if [[ "$REVERSE_PROXY_TYPE" == "0" ]]; then
    TRAEFIK_ACME_EMAIL=$(read_traefik_acme_email)
    ENABLE_PROXY=$(read_enable_proxy)
    if [[ "$ENABLE_PROXY" == "true" ]]; then
      ENABLE_CROWDSEC=$(read_enable_crowdsec)
    fi
  fi

  # Handle external Traefik-specific prompts (option 1)
  if [[ "$REVERSE_PROXY_TYPE" == "1" ]]; then
    TRAEFIK_EXTERNAL_NETWORK=$(read_traefik_network)
    TRAEFIK_ENTRYPOINT=$(read_traefik_entrypoint)
    TRAEFIK_CERTRESOLVER=$(read_traefik_certresolver)
  fi

  # Handle port binding for external proxy options (2-5)
  if [[ "$REVERSE_PROXY_TYPE" -ge 2 ]]; then
    BIND_LOCALHOST_ONLY=$(read_port_binding_preference)
  fi

  # Handle Docker network prompts for external proxies (options 2-4)
  case "$REVERSE_PROXY_TYPE" in
    2) EXTERNAL_PROXY_NETWORK=$(read_proxy_docker_network "Nginx") ;;
    3) EXTERNAL_PROXY_NETWORK=$(read_proxy_docker_network "Nginx Proxy Manager") ;;
    4) EXTERNAL_PROXY_NETWORK=$(read_proxy_docker_network "Caddy") ;;
    *) ;; # No network prompt for other options
  esac
  return 0
}

configure_anonymous_transport() {
  if [[ -n "$ANONBIRD_PEER_MANAGEMENT_ENDPOINT" ]]; then
    if ! check_peer_management_endpoint "$ANONBIRD_PEER_MANAGEMENT_ENDPOINT"; then
      exit 1
    fi
    ANONBIRD_ANONYMOUS_TRANSPORT="manual"
    return 0
  fi

  if [[ "$NONINTERACTIVE" != "true" ]]; then
    ANONBIRD_ANONYMOUS_TRANSPORT=$(read_anonymous_transport)
  fi

  if [[ "$ANONBIRD_ANONYMOUS_TRANSPORT" == "manual" ]]; then
    if [[ "$NONINTERACTIVE" == "true" ]]; then
      echo "Manual anonymous transport needs --peer-management-endpoint." > /dev/stderr
      exit 1
    fi
    ANONBIRD_PEER_MANAGEMENT_ENDPOINT=$(read_peer_management_endpoint)
    return 0
  fi

  if [[ "$ANONBIRD_ANONYMOUS_TRANSPORT" == "none" ]]; then
    ANONBIRD_PEER_MANAGEMENT_ENDPOINT=""
  fi
  return 0
}

check_existing_installation() {
  if [[ -f config.yaml ]]; then
    echo "Generated files already exist in $ANONBIRD_WORKDIR."
    echo "If you want to reinitialize the environment, please remove them first."
    echo "You can use the following commands:"
    echo "  cd $ANONBIRD_WORKDIR"
    echo "  $DOCKER_COMPOSE_COMMAND down --volumes # to remove all containers and volumes"
    echo "  rm -f docker-compose.yml dashboard.env config.yaml anonymous-endpoints.env tor.Dockerfile torrc i2pd.conf i2pd-tunnels.conf proxy.env traefik-dynamic.yaml nginx-anonbird.conf caddyfile-anonbird.txt npm-advanced-config.txt && rm -rf crowdsec/"
    echo "Be aware that this will remove all data from the database, and you will have to reconfigure the dashboard."
    exit 1
  fi
  return 0
}

generate_configuration_files() {
  echo Rendering initial files...

  # Render docker-compose and proxy config based on selection
  case "$REVERSE_PROXY_TYPE" in
    0)
      render_docker_compose_traefik_builtin > docker-compose.yml
      if [[ "$ENABLE_PROXY" == "true" ]]; then
        # Create placeholder proxy.env so docker-compose can validate
        # This will be overwritten with the actual token after anonbird-server starts
        echo "# Placeholder - will be updated with token after anonbird-server starts" > proxy.env
        echo "NB_PROXY_TOKEN=placeholder" >> proxy.env
        # TCP ServersTransport for PROXY protocol v2 to the proxy backend
        render_traefik_dynamic > traefik-dynamic.yaml
        if [[ "$ENABLE_CROWDSEC" == "true" ]]; then
          mkdir -p crowdsec
        fi
      fi
      ;;
    1)
      render_docker_compose_traefik > docker-compose.yml
      ;;
    2)
      render_docker_compose_exposed_ports > docker-compose.yml
      render_nginx_conf > nginx-anonbird.conf
      ;;
    3)
      render_docker_compose_exposed_ports > docker-compose.yml
      render_npm_advanced_config > npm-advanced-config.txt
      ;;
    4)
      render_docker_compose_exposed_ports > docker-compose.yml
      render_external_caddyfile > caddyfile-anonbird.txt
      ;;
    5)
      render_docker_compose_exposed_ports > docker-compose.yml
      ;;
    *)
      echo "Invalid reverse proxy type: $REVERSE_PROXY_TYPE" > /dev/stderr
      exit 1
      ;;
  esac

  # Common files for all configurations
  render_dashboard_env > dashboard.env
  render_combined_yaml > config.yaml
  if managed_tor_enabled; then
    render_tor_dockerfile > tor.Dockerfile
    render_torrc > torrc
  fi
  if managed_i2p_enabled; then
    render_i2pd_conf > i2pd.conf
    render_i2pd_tunnels_conf > i2pd-tunnels.conf
  fi
  return 0
}

start_services_and_show_instructions() {
  # For built-in Traefik, start containers immediately
  # For NPM, start containers first (NPM needs services running to create proxy)
  # For other external proxies, show instructions first and wait for user confirmation
  if [[ "$REVERSE_PROXY_TYPE" == "0" ]]; then
    # Built-in Traefik - two-phase startup if proxy is enabled
    echo -e "$MSG_STARTING_SERVICES"

    if [[ "$ENABLE_PROXY" == "true" ]]; then
      # Phase 1: Start core services (without proxy)
      local core_services="traefik dashboard anonbird-server"
      if [[ "$ENABLE_CROWDSEC" == "true" ]]; then
        core_services="$core_services crowdsec"
      fi
      echo "Starting core services..."
      $DOCKER_COMPOSE_COMMAND up -d $core_services

      sleep 3
      wait_management_proxy traefik

      # Phase 2: Create proxy token and start proxy
      echo ""
      echo "Creating proxy access token..."
      # Use docker exec with bash to run the token command directly
      PROXY_TOKEN=$($DOCKER_COMPOSE_COMMAND exec -T anonbird-server \
        /go/bin/anonbird-server token create --name "default-proxy" --config /etc/anonbird/config.yaml 2>/dev/null | grep "^Token:" | awk '{print $2}')

      if [[ -z "$PROXY_TOKEN" ]]; then
        echo "ERROR: Failed to create proxy token. Check anonbird-server logs." > /dev/stderr
        $DOCKER_COMPOSE_COMMAND logs --tail=20 anonbird-server
        exit 1
      fi

      echo "Proxy token created successfully."

      if [[ "$ENABLE_CROWDSEC" == "true" ]]; then
        echo "Registering CrowdSec bouncer..."
        local cs_retries=0
        while ! $DOCKER_COMPOSE_COMMAND exec -T crowdsec cscli lapi status >/dev/null 2>&1; do
          cs_retries=$((cs_retries + 1))
          if [[ $cs_retries -ge 30 ]]; then
            echo "WARNING: CrowdSec did not become ready. Skipping CrowdSec setup." > /dev/stderr
            echo "You can register a bouncer manually later with:" > /dev/stderr
            echo "  docker exec anonbird-crowdsec cscli bouncers add anonbird-proxy -o raw" > /dev/stderr
            ENABLE_CROWDSEC="false"
            break
          fi
          sleep 2
        done

        if [[ "$ENABLE_CROWDSEC" == "true" ]]; then
          CROWDSEC_BOUNCER_KEY=$($DOCKER_COMPOSE_COMMAND exec -T crowdsec \
            cscli bouncers add anonbird-proxy -o raw 2>/dev/null)
          if [[ -z "$CROWDSEC_BOUNCER_KEY" ]]; then
            echo "WARNING: Failed to create CrowdSec bouncer key. Skipping CrowdSec setup." > /dev/stderr
            ENABLE_CROWDSEC="false"
          else
            echo "CrowdSec bouncer registered."
          fi
        fi
      fi

      render_proxy_env > proxy.env

      # Start proxy service
      echo "Starting proxy service..."
      $DOCKER_COMPOSE_COMMAND up -d proxy
    else
      # No proxy - start all services at once
      $DOCKER_COMPOSE_COMMAND up -d

      sleep 3
      wait_management_proxy traefik
    fi

    echo -e "$MSG_DONE"
    print_post_setup_instructions
  elif [[ "$REVERSE_PROXY_TYPE" == "1" ]]; then
    # External Traefik - start containers, then show instructions
    # Traefik discovers services via Docker labels, so containers must be running
    echo -e "$MSG_STARTING_SERVICES"
    $DOCKER_COMPOSE_COMMAND up -d

    sleep 3
    wait_management_proxy detect-traefik

    echo -e "$MSG_DONE"
    print_post_setup_instructions
    echo ""
    echo "AnonBird containers are running. Once Traefik is connected, access the dashboard at:"
    echo "  $NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN"
  elif [[ "$REVERSE_PROXY_TYPE" == "3" ]]; then
    # NPM - start containers first, then show instructions
    # NPM requires backend services to be running before creating proxy hosts
    echo -e "$MSG_STARTING_SERVICES"
    $DOCKER_COMPOSE_COMMAND up -d

    sleep 3
    wait_management_direct

    echo -e "$MSG_DONE"
    print_post_setup_instructions
    echo ""
    echo "AnonBird containers are running. Configure NPM as shown above, then access:"
    echo "  $NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN"
  else
    # External proxies (nginx, external Caddy, other) - need manual config first
    print_post_setup_instructions

    if [[ "$NONINTERACTIVE" == "true" ]]; then
      echo ""
      echo "External proxy mode generated files and instructions. Configure your reverse proxy, then run:"
      echo "  $DOCKER_COMPOSE_COMMAND up -d"
      return 0
    fi

    echo ""
    echo -n "Press Enter when your reverse proxy is configured (or Ctrl+C to exit)... "
    read -r < /dev/tty

    echo -e "$MSG_STARTING_SERVICES"
    $DOCKER_COMPOSE_COMMAND up -d

    sleep 3
    wait_management_direct

    echo -e "$MSG_DONE"
    echo "AnonBird is now running. Access the dashboard at:"
    echo "  $NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN"
  fi
  return 0
}

wait_for_tor_endpoint() {
  local endpoint=""
  local counter=0
  echo -n "Waiting for managed Tor onion address" > /dev/stderr
  while [[ "$counter" -lt 90 ]]; do
    endpoint=$($DOCKER_COMPOSE_COMMAND exec -T tor sh -lc 'cat /var/lib/tor/anonbird-management/hostname 2>/dev/null' 2>/dev/null | tr -d '\r\n' || true)
    if check_required_peer_management_endpoint "http://$endpoint" >/dev/null 2>&1; then
      echo " done" > /dev/stderr
      echo "http://$endpoint"
      return 0
    fi
    echo -n " ." > /dev/stderr
    sleep 2
    counter=$((counter + 1))
  done
  echo "" > /dev/stderr
  echo "ERROR: Timed out waiting for Tor onion address. Check logs with:" > /dev/stderr
  echo "  $DOCKER_COMPOSE_COMMAND logs tor" > /dev/stderr
  return 1
}

wait_for_i2p_endpoint() {
  local endpoint=""
  local counter=0
  echo -n "Waiting for managed I2P b32 address" > /dev/stderr
  while [[ "$counter" -lt 120 ]]; do
    endpoint=$($DOCKER_COMPOSE_COMMAND exec -T i2pd sh -lc 'for f in /home/i2pd/data/destinations/*.dat; do b=$(basename "$f" 2>/dev/null); b=${b%%.*}; if echo "$b" | grep -Eq "^[a-z2-7]{52}$"; then echo "http://$b.b32.i2p"; exit 0; fi; done' 2>/dev/null | tr -d '\r\n' || true)
    if check_required_peer_management_endpoint "$endpoint" >/dev/null 2>&1; then
      echo " done" > /dev/stderr
      echo "$endpoint"
      return 0
    fi
    endpoint=$($DOCKER_COMPOSE_COMMAND logs --no-color --tail=200 i2pd 2>/dev/null | grep -Eo '[a-z2-7]{52}\.b32\.i2p' | head -n 1 || true)
    if check_required_peer_management_endpoint "http://$endpoint" >/dev/null 2>&1; then
      echo " done" > /dev/stderr
      echo "http://$endpoint"
      return 0
    fi
    echo -n " ." > /dev/stderr
    sleep 2
    counter=$((counter + 1))
  done
  echo "" > /dev/stderr
  echo "ERROR: Timed out waiting for I2P b32 address. Check logs with:" > /dev/stderr
  echo "  $DOCKER_COMPOSE_COMMAND logs i2pd" > /dev/stderr
  return 1
}

write_anonymous_endpoints_env() {
  cat > anonymous-endpoints.env <<EOF
ANONBIRD_ANONYMOUS_TRANSPORT=$ANONBIRD_ANONYMOUS_TRANSPORT
ANONBIRD_PEER_MANAGEMENT_ENDPOINT=$ANONBIRD_PEER_MANAGEMENT_ENDPOINT
ANONBIRD_TOR_MANAGEMENT_ENDPOINT=${ANONBIRD_TOR_MANAGEMENT_ENDPOINT:-}
ANONBIRD_I2P_MANAGEMENT_ENDPOINT=${ANONBIRD_I2P_MANAGEMENT_ENDPOINT:-}
EOF
}

prepare_anonymous_management_endpoint() {
  if [[ "$ANONBIRD_ANONYMOUS_TRANSPORT" == "manual" || "$ANONBIRD_ANONYMOUS_TRANSPORT" == "none" ]]; then
    render_dashboard_env > dashboard.env
    return 0
  fi

  if ! managed_anonymous_transport_enabled; then
    render_dashboard_env > dashboard.env
    return 0
  fi

  local services=""
  if managed_tor_enabled; then
    services="$services tor"
  fi
  if managed_i2p_enabled; then
    services="$services i2pd"
  fi

  echo "Starting managed anonymous peer endpoint service(s)..."
  $DOCKER_COMPOSE_COMMAND up -d $services

  if managed_tor_enabled; then
    ANONBIRD_TOR_MANAGEMENT_ENDPOINT=$(wait_for_tor_endpoint)
  fi
  if managed_i2p_enabled; then
    ANONBIRD_I2P_MANAGEMENT_ENDPOINT=$(wait_for_i2p_endpoint)
  fi

  case "$ANONBIRD_ANONYMOUS_TRANSPORT" in
    i2p)
      ANONBIRD_PEER_MANAGEMENT_ENDPOINT="$ANONBIRD_I2P_MANAGEMENT_ENDPOINT"
      ;;
    tor|both)
      ANONBIRD_PEER_MANAGEMENT_ENDPOINT="$ANONBIRD_TOR_MANAGEMENT_ENDPOINT"
      ;;
  esac

  if ! check_required_peer_management_endpoint "$ANONBIRD_PEER_MANAGEMENT_ENDPOINT"; then
    echo "ERROR: Failed to prepare a valid anonymous peer management endpoint." > /dev/stderr
    exit 1
  fi

  render_dashboard_env > dashboard.env
  write_anonymous_endpoints_env
  echo "Peer install commands will use:"
  echo "  $ANONBIRD_PEER_MANAGEMENT_ENDPOINT"
  if [[ -n "${ANONBIRD_I2P_MANAGEMENT_ENDPOINT:-}" && "$ANONBIRD_I2P_MANAGEMENT_ENDPOINT" != "$ANONBIRD_PEER_MANAGEMENT_ENDPOINT" ]]; then
    echo "Managed I2P endpoint is also available:"
    echo "  $ANONBIRD_I2P_MANAGEMENT_ENDPOINT"
  fi
  return 0
}

init_environment() {
  initialize_default_values
  parse_args "$@"
  if [[ "$NONINTERACTIVE" != "true" ]]; then
    print_interactive_intro
  fi
  configure_domain
  configure_anonymous_transport
  configure_reverse_proxy

  if [[ "$RENDER_ONLY" == "true" ]]; then
    DOCKER_COMPOSE_COMMAND="docker compose"
  elif [[ "$PREFLIGHT_ONLY" == "true" ]]; then
    DOCKER_COMPOSE_COMMAND=$(check_docker_compose)
  else
    check_jq
    DOCKER_COMPOSE_COMMAND=$(check_docker_compose)
  fi

  if [[ "$PREFLIGHT_ONLY" == "true" ]]; then
    preflight_required_images
    return 0
  fi

  ensure_workdir
  check_existing_installation
  generate_configuration_files
  if [[ "$RENDER_ONLY" == "true" ]]; then
    echo "Rendered AnonBird self-host files in $ANONBIRD_WORKDIR:"
    echo "  docker-compose.yml"
    echo "  dashboard.env"
    echo "  config.yaml"
    if managed_tor_enabled; then
      echo "  tor.Dockerfile"
      echo "  torrc"
    fi
    if managed_i2p_enabled; then
      echo "  i2pd.conf"
      echo "  i2pd-tunnels.conf"
    fi
    echo "Run with the same options without --render-only to start containers from this directory."
    return 0
  fi
  preflight_required_images
  prepare_anonymous_management_endpoint
  start_services_and_show_instructions
  return 0
}

############################################
# Configuration File Renderers
############################################

render_docker_compose_traefik_builtin() {
  # Generate proxy service section and Traefik dynamic config if enabled
  local proxy_service=""
  local proxy_volumes=""
  local crowdsec_service=""
  local crowdsec_volumes=""
  local traefik_file_provider=""
  local traefik_dynamic_volume=""
  local anonymous_services
  local anonymous_volumes
  anonymous_services=$(render_anonymous_services "[anonbird]")
  anonymous_volumes=$(render_anonymous_volumes)
  if [[ "$ENABLE_PROXY" == "true" ]]; then
    traefik_file_provider='      - "--providers.file.filename=/etc/traefik/dynamic.yaml"'
    traefik_dynamic_volume="      - ./traefik-dynamic.yaml:/etc/traefik/dynamic.yaml:ro"

    local proxy_depends="
      anonbird-server:
        condition: service_started"
    if [[ "$ENABLE_CROWDSEC" == "true" ]]; then
      proxy_depends="
      anonbird-server:
        condition: service_started
      crowdsec:
        condition: service_healthy"
    fi

    proxy_service="
  # AnonBird Proxy - exposes internal resources to the internet
  proxy:
    image: $NETBIRD_PROXY_IMAGE
    container_name: anonbird-proxy
    ports:
    - 51820:51820/udp
    restart: unless-stopped
    networks: [anonbird]
    depends_on:${proxy_depends}
    env_file:
      - ./proxy.env
    volumes:
      - anonbird_proxy_certs:/certs
    labels:
      # TCP passthrough for any unmatched domain (proxy handles its own TLS)
      - traefik.enable=true
      - traefik.tcp.routers.proxy-passthrough.entrypoints=websecure
      - traefik.tcp.routers.proxy-passthrough.rule=HostSNI(\`*\`)
      - traefik.tcp.routers.proxy-passthrough.tls.passthrough=true
      - traefik.tcp.routers.proxy-passthrough.service=proxy-tls
      - traefik.tcp.routers.proxy-passthrough.priority=1
      - traefik.tcp.services.proxy-tls.loadbalancer.server.port=8443
      - traefik.tcp.services.proxy-tls.loadbalancer.serverstransport=pp-v2@file
    logging:
      driver: \"json-file\"
      options:
        max-size: \"500m\"
        max-file: \"2\"
"
    proxy_volumes="
  anonbird_proxy_certs:"

    if [[ "$ENABLE_CROWDSEC" == "true" ]]; then
      crowdsec_service="
  crowdsec:
    image: crowdsecurity/crowdsec:v1.7.7
    container_name: anonbird-crowdsec
    restart: unless-stopped
    networks: [anonbird]
    environment:
      COLLECTIONS: crowdsecurity/linux
    volumes:
      - ./crowdsec:/etc/crowdsec
      - crowdsec_db:/var/lib/crowdsec/data
    healthcheck:
      test: ["CMD", "cscli", "lapi", "status"]
      interval: 10s
      timeout: 5s
      retries: 15
    labels:
      - traefik.enable=false
    logging:
      driver: \"json-file\"
      options:
        max-size: \"500m\"
        max-file: \"2\"
"
      crowdsec_volumes="
  crowdsec_db:"
    fi
  fi

  cat <<EOF
services:
  # Traefik reverse proxy (automatic TLS via Let's Encrypt)
  traefik:
    image: traefik:v3.6
    container_name: anonbird-traefik
    restart: unless-stopped
    networks:
      anonbird:
        ipv4_address: $TRAEFIK_IP
    command:
      # Logging
      - "--log.level=INFO"
      - "--accesslog=true"
      # Docker provider
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--providers.docker.network=anonbird"
      # Entrypoints
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--entrypoints.websecure.allowACMEByPass=true"
      # Disable timeouts for long-lived gRPC streams
      - "--entrypoints.websecure.transport.respondingTimeouts.readTimeout=0"
      - "--entrypoints.websecure.transport.respondingTimeouts.writeTimeout=0"
      - "--entrypoints.websecure.transport.respondingTimeouts.idleTimeout=0"
      # HTTP to HTTPS redirect
      - "--entrypoints.web.http.redirections.entrypoint.to=websecure"
      - "--entrypoints.web.http.redirections.entrypoint.scheme=https"
      # Let's Encrypt ACME
      - "--certificatesresolvers.letsencrypt.acme.email=$TRAEFIK_ACME_EMAIL"
      - "--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json"
      - "--certificatesresolvers.letsencrypt.acme.tlschallenge=true"
      # gRPC transport settings
      - "--serverstransport.forwardingtimeouts.responseheadertimeout=0s"
      - "--serverstransport.forwardingtimeouts.idleconntimeout=0s"
$traefik_file_provider
    ports:
      - '443:443'
      - '80:80'
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - anonbird_traefik_letsencrypt:/letsencrypt
$traefik_dynamic_volume
    logging:
      driver: "json-file"
      options:
        max-size: "500m"
        max-file: "2"

  # UI dashboard
  dashboard:
    image: $DASHBOARD_IMAGE
    container_name: anonbird-dashboard
    restart: unless-stopped
    networks: [anonbird]
    env_file:
      - ./dashboard.env
    labels:
      - traefik.enable=true
      - traefik.http.routers.anonbird-dashboard.rule=Host(\`$NETBIRD_DOMAIN\`)
      - traefik.http.routers.anonbird-dashboard.entrypoints=websecure
      - traefik.http.routers.anonbird-dashboard.tls=true
      - traefik.http.routers.anonbird-dashboard.tls.certresolver=letsencrypt
      - traefik.http.routers.anonbird-dashboard.service=dashboard
      - traefik.http.routers.anonbird-dashboard.priority=1
      - traefik.http.services.dashboard.loadbalancer.server.port=80
    logging:
      driver: "json-file"
      options:
        max-size: "500m"
        max-file: "2"

  # Combined server (Management + Signal + Relay)
  anonbird-server:
    image: $NETBIRD_SERVER_IMAGE
    container_name: anonbird-server
    restart: unless-stopped
    networks: [anonbird]
    env_file:
      - ./dashboard.env
    environment:
      NB_DISABLE_GEOLOCATION: "true"
      NB_PEER_UPDATE_INTERVAL_MS: "1"
      NB_PEER_UPDATE_STARTUP_PERIOD_S: "1"
$(render_stun_ports_section)
    volumes:
      - anonbird_data:/var/lib/anonbird
      - ./config.yaml:/etc/anonbird/config.yaml
    command: ["--config", "/etc/anonbird/config.yaml"]
    labels:
      - traefik.enable=true
      # gRPC router (needs h2c backend for HTTP/2 cleartext)
      - traefik.http.routers.anonbird-grpc.rule=Host(\`$NETBIRD_DOMAIN\`) && (PathPrefix(\`/signalexchange.SignalExchange/\`) || PathPrefix(\`/management.ManagementService/\`))
      - traefik.http.routers.anonbird-grpc.entrypoints=websecure
      - traefik.http.routers.anonbird-grpc.tls=true
      - traefik.http.routers.anonbird-grpc.tls.certresolver=letsencrypt
      - traefik.http.routers.anonbird-grpc.service=anonbird-server-h2c
      - traefik.http.routers.anonbird-grpc.priority=100
      # Backend router (relay, WebSocket, API, OAuth2)
      - traefik.http.routers.anonbird-backend.rule=Host(\`$NETBIRD_DOMAIN\`) && (PathPrefix(\`/relay\`) || PathPrefix(\`/ws-proxy/\`) || PathPrefix(\`/api\`) || PathPrefix(\`/oauth2\`))
      - traefik.http.routers.anonbird-backend.entrypoints=websecure
      - traefik.http.routers.anonbird-backend.tls=true
      - traefik.http.routers.anonbird-backend.tls.certresolver=letsencrypt
      - traefik.http.routers.anonbird-backend.service=anonbird-server
      - traefik.http.routers.anonbird-backend.priority=100
      # Services
      - traefik.http.services.anonbird-server.loadbalancer.server.port=80
      - traefik.http.services.anonbird-server-h2c.loadbalancer.server.port=80
      - traefik.http.services.anonbird-server-h2c.loadbalancer.server.scheme=h2c
    logging:
      driver: "json-file"
      options:
        max-size: "500m"
        max-file: "2"
${anonymous_services}${proxy_service}${crowdsec_service}
volumes:
  anonbird_data:
  anonbird_traefik_letsencrypt:
${anonymous_volumes}${proxy_volumes}${crowdsec_volumes}

networks:
  anonbird:
    name: anonbird
    driver: bridge
    ipam:
      config:
        - subnet: 172.30.0.0/24
          gateway: 172.30.0.1
EOF
  return 0
}

render_combined_yaml() {
  cat <<EOF
# Combined AnonBird Server Configuration (Simplified)
# Generated by getting-started.sh

server:
  listenAddress: ":80"
  exposedAddress: "$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN:$NETBIRD_PORT"
$(render_stun_ports_yaml)
  metricsPort: 9090
  healthcheckAddress: ":9000"
  logLevel: "info"
  logFile: "console"

  authSecret: "$NETBIRD_RELAY_AUTH_SECRET"
  dataDir: "/var/lib/anonbird"
  disableAnonymousMetrics: true
  disableGeoliteUpdate: true
  disableVersionCheck: true

  auth:
    issuer: "$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN/oauth2"
    signKeyRefreshEnabled: true
    dashboardRedirectURIs:
      - "$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN/nb-auth"
      - "$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN/nb-silent-auth"
    cliRedirectURIs:
      - "http://localhost:53000/"

  reverseProxy:
    trustedHTTPProxies:
      - "$TRAEFIK_IP/32"

  store:
    engine: "sqlite"
    encryptionKey: "$DATASTORE_ENCRYPTION_KEY"
EOF
  return 0
}

render_dashboard_env() {
  cat <<EOF
# Endpoints
NETBIRD_MGMT_API_ENDPOINT=$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN
NETBIRD_MGMT_GRPC_API_ENDPOINT=$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN
ANONBIRD_PEER_MANAGEMENT_ENDPOINT=$ANONBIRD_PEER_MANAGEMENT_ENDPOINT
# OIDC - using embedded IdP
# The embedded IdP keeps the inherited OAuth client ID for compatibility.
AUTH_AUDIENCE=netbird-dashboard
AUTH_CLIENT_ID=netbird-dashboard
AUTH_CLIENT_SECRET=
AUTH_AUTHORITY=$NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN/oauth2
USE_AUTH0=false
AUTH_SUPPORTED_SCOPES=openid profile email groups
AUTH_REDIRECT_URI=/nb-auth
AUTH_SILENT_REDIRECT_URI=/nb-silent-auth
# SSL
NGINX_SSL_PORT=443
# Letsencrypt
LETSENCRYPT_DOMAIN=none
EOF
  return 0
}

render_tor_dockerfile() {
  cat <<'EOF'
FROM alpine:3.22
RUN apk add --no-cache tor socat \
    && mkdir -p /var/lib/tor \
    && chown -R tor:tor /var/lib/tor
USER tor
ENTRYPOINT ["sh", "-ec", "socat TCP-LISTEN:8080,bind=127.0.0.1,fork,reuseaddr TCP:anonbird-server:80 & exec tor -f /etc/tor/torrc"]
EOF
  return 0
}

render_torrc() {
  cat <<'EOF'
DataDirectory /var/lib/tor
SocksPort 0
Log notice stdout

HiddenServiceDir /var/lib/tor/anonbird-management/
HiddenServiceVersion 3
# Tor requires a numeric target address here. The container-local proxy resolves
# anonbird-server through Docker DNS and forwards traffic to the combined server.
HiddenServicePort 80 127.0.0.1:8080
EOF
  return 0
}

render_i2pd_conf() {
  cat <<'EOF'
log = stdout
loglevel = info
notransit = true
upnp.enabled = false

[http]
enabled = false

[httpproxy]
enabled = false

[socksproxy]
enabled = false

[i2cp]
enabled = false

[sam]
enabled = true
address = 0.0.0.0
port = 7656
EOF
  return 0
}

render_i2pd_tunnels_conf() {
  cat <<'EOF'
[anonbird-management]
type = server
host = anonbird-server
port = 80
keys = anonbird-management.dat
signaturetype = 7
inbound.length = 1
outbound.length = 1
inbound.quantity = 3
outbound.quantity = 3
EOF
  return 0
}

render_traefik_dynamic() {
  cat <<'EOF'
tcp:
  serversTransports:
    pp-v2:
      proxyProtocol:
        version: 2
EOF
  return 0
}

render_proxy_env() {
  cat <<EOF
# AnonBird Proxy Configuration
NB_PROXY_DEBUG_LOGS=false
# Use internal Docker network to connect to management (avoids hairpin NAT issues)
NB_PROXY_MANAGEMENT_ADDRESS=http://anonbird-server:80
# Allow insecure gRPC connection to management (required for internal Docker network)
NB_PROXY_ALLOW_INSECURE=true
# Public URL where this proxy is reachable (used for cluster registration)
NB_PROXY_DOMAIN=$NETBIRD_DOMAIN
NB_PROXY_ADDRESS=:8443
NB_PROXY_TOKEN=$PROXY_TOKEN
NB_PROXY_CERTIFICATE_DIRECTORY=/certs
NB_PROXY_ACME_CERTIFICATES=true
NB_PROXY_ACME_CHALLENGE_TYPE=tls-alpn-01
NB_PROXY_FORWARDED_PROTO=https
# Enable PROXY protocol to preserve client IPs through L4 proxies (Traefik TCP passthrough)
NB_PROXY_PROXY_PROTOCOL=true
# Trust Traefik's IP for PROXY protocol headers
NB_PROXY_TRUSTED_PROXIES=$TRAEFIK_IP
EOF

  if [[ "$ENABLE_CROWDSEC" == "true" && -n "$CROWDSEC_BOUNCER_KEY" ]]; then
    cat <<EOF
NB_PROXY_CROWDSEC_API_URL=http://crowdsec:8080
NB_PROXY_CROWDSEC_API_KEY=$CROWDSEC_BOUNCER_KEY
EOF
  fi

  return 0
}

render_docker_compose_traefik() {
  local network_name="${TRAEFIK_EXTERNAL_NETWORK:-anonbird}"
  local network_config=""
  local anonymous_services
  local anonymous_volumes
  anonymous_services=$(render_anonymous_services "[$network_name]")
  anonymous_volumes=$(render_anonymous_volumes)
  if [[ -n "$TRAEFIK_EXTERNAL_NETWORK" ]]; then
    network_config="    external: true"
  fi

  # Build TLS labels - certresolver is optional
  local tls_labels=""
  if [[ -n "$TRAEFIK_CERTRESOLVER" ]]; then
    tls_labels="tls.certresolver=${TRAEFIK_CERTRESOLVER}"
  fi

  cat <<EOF
services:
  # UI dashboard
  dashboard:
    image: $DASHBOARD_IMAGE
    container_name: anonbird-dashboard
    restart: unless-stopped
    networks: [$network_name]
    env_file:
      - ./dashboard.env
    labels:
      - traefik.enable=true
      - traefik.http.routers.anonbird-dashboard.rule=Host(\`$NETBIRD_DOMAIN\`)
      - traefik.http.routers.anonbird-dashboard.entrypoints=$TRAEFIK_ENTRYPOINT
      - traefik.http.routers.anonbird-dashboard.tls=true
$(if [[ -n "$tls_labels" ]]; then echo "      - traefik.http.routers.anonbird-dashboard.${tls_labels}"; fi)
      - traefik.http.routers.anonbird-dashboard.priority=1
      - traefik.http.services.anonbird-dashboard.loadbalancer.server.port=80
    logging:
      driver: "json-file"
      options:
        max-size: "500m"
        max-file: "2"

  # Combined server (Management + Signal + Relay)
  anonbird-server:
    image: $NETBIRD_SERVER_IMAGE
    container_name: anonbird-server
    restart: unless-stopped
    networks: [$network_name]
    env_file:
      - ./dashboard.env
    environment:
      NB_DISABLE_GEOLOCATION: "true"
      NB_PEER_UPDATE_INTERVAL_MS: "1"
      NB_PEER_UPDATE_STARTUP_PERIOD_S: "1"
$(render_stun_ports_section)
    volumes:
      - anonbird_data:/var/lib/anonbird
      - ./config.yaml:/etc/anonbird/config.yaml
    command: ["--config", "/etc/anonbird/config.yaml"]
    labels:
      - traefik.enable=true
      # gRPC router (needs h2c backend for HTTP/2 cleartext)
      - traefik.http.routers.anonbird-grpc.rule=Host(\`$NETBIRD_DOMAIN\`) && (PathPrefix(\`/signalexchange.SignalExchange/\`) || PathPrefix(\`/management.ManagementService/\`))
      - traefik.http.routers.anonbird-grpc.entrypoints=$TRAEFIK_ENTRYPOINT
      - traefik.http.routers.anonbird-grpc.tls=true
$(if [[ -n "$tls_labels" ]]; then echo "      - traefik.http.routers.anonbird-grpc.${tls_labels}"; fi)
      - traefik.http.routers.anonbird-grpc.service=anonbird-server-h2c
      # Backend router (relay, WebSocket, API, OAuth2)
      - traefik.http.routers.anonbird-backend.rule=Host(\`$NETBIRD_DOMAIN\`) && (PathPrefix(\`/relay\`) || PathPrefix(\`/ws-proxy/\`) || PathPrefix(\`/api\`) || PathPrefix(\`/oauth2\`))
      - traefik.http.routers.anonbird-backend.entrypoints=$TRAEFIK_ENTRYPOINT
      - traefik.http.routers.anonbird-backend.tls=true
$(if [[ -n "$tls_labels" ]]; then echo "      - traefik.http.routers.anonbird-backend.${tls_labels}"; fi)
      - traefik.http.routers.anonbird-backend.service=anonbird-server
      # Services
      - traefik.http.services.anonbird-server.loadbalancer.server.port=80
      - traefik.http.services.anonbird-server-h2c.loadbalancer.server.port=80
      - traefik.http.services.anonbird-server-h2c.loadbalancer.server.scheme=h2c
    logging:
      driver: "json-file"
      options:
        max-size: "500m"
        max-file: "2"
${anonymous_services}

volumes:
  anonbird_data:
${anonymous_volumes}

networks:
  $network_name:
$network_config
EOF
  return 0
}

render_docker_compose_exposed_ports() {
  local bind_addr=$(get_bind_address)
  local networks="[anonbird]"
  local networks_config="networks:
  anonbird:"
  local anonymous_services
  local anonymous_volumes

  # If an external network is specified, add it and include in service networks
  if [[ -n "$EXTERNAL_PROXY_NETWORK" ]]; then
    networks="[anonbird, $EXTERNAL_PROXY_NETWORK]"
    networks_config="networks:
  anonbird:
  $EXTERNAL_PROXY_NETWORK:
    external: true"
  fi
  anonymous_services=$(render_anonymous_services "[anonbird]")
  anonymous_volumes=$(render_anonymous_volumes)

  cat <<EOF
services:
  # UI dashboard
  dashboard:
    image: $DASHBOARD_IMAGE
    container_name: anonbird-dashboard
    restart: unless-stopped
    networks: ${networks}
    ports:
      - '${bind_addr}:${DASHBOARD_HOST_PORT}:80'
    env_file:
      - ./dashboard.env
    logging:
      driver: "json-file"
      options:
        max-size: "500m"
        max-file: "2"

  # Combined server (Management + Signal + Relay)
  anonbird-server:
    image: $NETBIRD_SERVER_IMAGE
    container_name: anonbird-server
    restart: unless-stopped
    networks: ${networks}
    env_file:
      - ./dashboard.env
    environment:
      NB_DISABLE_GEOLOCATION: "true"
      NB_PEER_UPDATE_INTERVAL_MS: "1"
      NB_PEER_UPDATE_STARTUP_PERIOD_S: "1"
    ports:
      - '${bind_addr}:${MANAGEMENT_HOST_PORT}:80'
$(render_stun_port_line)
    volumes:
      - anonbird_data:/var/lib/anonbird
      - ./config.yaml:/etc/anonbird/config.yaml
    command: ["--config", "/etc/anonbird/config.yaml"]
    logging:
      driver: "json-file"
      options:
        max-size: "500m"
        max-file: "2"
${anonymous_services}

volumes:
  anonbird_data:
${anonymous_volumes}

${networks_config}
EOF
  return 0
}

render_nginx_conf() {
  local upstream_host=$(get_upstream_host)
  local dashboard_addr="${upstream_host}:${DASHBOARD_HOST_PORT}"
  local server_addr="${upstream_host}:${MANAGEMENT_HOST_PORT}"
  local install_note="# 1. Update SSL certificate paths below
# 2. Copy to your nginx config directory:
#    Debian/Ubuntu: /etc/nginx/sites-available/anonbird (then symlink to sites-enabled)
#    RHEL/CentOS:   /etc/nginx/conf.d/anonbird.conf
# 3. Test and reload: nginx -t && systemctl reload nginx"

  # If running in Docker network, use container names
  if [[ -n "$EXTERNAL_PROXY_NETWORK" ]]; then
    dashboard_addr="anonbird-dashboard:80"
    server_addr="anonbird-server:80"
    install_note="# This config uses container names since Nginx is on the same Docker network.
# Add this to your nginx.conf or include it from a separate file."
  fi

  cat <<EOF
# AnonBird Nginx Configuration
# Generated by getting-started.sh
#
${install_note}

upstream anonbird_dashboard {
    server ${dashboard_addr};
    keepalive 10;
}
upstream anonbird_server {
    server ${server_addr};
}

server {
    listen 80;
    server_name $NETBIRD_DOMAIN;

    location / {
        return 301 https://\$host\$request_uri;
    }
}

server {
    listen 443 ssl http2;
    server_name $NETBIRD_DOMAIN;

    # SSL/TLS Configuration
    # Update these paths based on your certificate source:
    #
    # Let's Encrypt (certbot):
    #   ssl_certificate /etc/letsencrypt/live/$NETBIRD_DOMAIN/fullchain.pem;
    #   ssl_certificate_key /etc/letsencrypt/live/$NETBIRD_DOMAIN/privkey.pem;
    #
    # Let's Encrypt (acme.sh):
    #   ssl_certificate /root/.acme.sh/$NETBIRD_DOMAIN/fullchain.cer;
    #   ssl_certificate_key /root/.acme.sh/$NETBIRD_DOMAIN/$NETBIRD_DOMAIN.key;
    #
    # Custom certificates:
    #   ssl_certificate /etc/ssl/certs/$NETBIRD_DOMAIN.crt;
    #   ssl_certificate_key /etc/ssl/private/$NETBIRD_DOMAIN.key;
    #
    ssl_certificate /path/to/your/fullchain.pem;
    ssl_certificate_key /path/to/your/privkey.pem;

    # Recommended SSL settings
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;

    # Required for long-lived gRPC connections
    client_header_timeout 1d;
    client_body_timeout 1d;

    # Common proxy headers
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Scheme \$scheme;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header X-Forwarded-Host \$host;
    grpc_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;

    # WebSocket connections (relay, signal, management)
    location ~ ^/(relay|ws-proxy/) {
        proxy_pass http://anonbird_server;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_set_header Host \$host;
        proxy_read_timeout 1d;
    }

    # Native gRPC (signal + management)
    location ~ ^/(signalexchange\.SignalExchange|management\.ManagementService)/ {
        grpc_pass grpc://anonbird_server;
        grpc_read_timeout 1d;
        grpc_send_timeout 1d;
        grpc_socket_keepalive on;
    }

    # HTTP routes (API + OAuth2)
    location ~ ^/(api|oauth2)/ {
        proxy_pass http://anonbird_server;
        proxy_set_header Host \$host;
    }

    # Dashboard (catch-all)
    location / {
        proxy_pass http://anonbird_dashboard;
    }
}
EOF
  return 0
}

render_external_caddyfile() {
  local upstream_host=$(get_upstream_host)
  local dashboard_addr="${upstream_host}:${DASHBOARD_HOST_PORT}"
  local server_addr="${upstream_host}:${MANAGEMENT_HOST_PORT}"
  local install_note="# Add this block to your existing Caddyfile and reload Caddy"

  # If running in Docker network, use container names
  if [[ -n "$EXTERNAL_PROXY_NETWORK" ]]; then
    dashboard_addr="anonbird-dashboard:80"
    server_addr="anonbird-server:80"
    install_note="# This config uses container names since Caddy is on the same Docker network.
# Add this block to your Caddyfile and reload Caddy."
  fi

  cat <<EOF
# AnonBird Caddyfile Snippet
# Generated by getting-started.sh
#
${install_note}

$NETBIRD_DOMAIN {
    # Native gRPC (needs HTTP/2 cleartext to backend)
    @grpc header Content-Type application/grpc*
    reverse_proxy @grpc h2c://${server_addr}

    # Combined server paths (relay, signal, management, OAuth2)
    @backend path /relay* /ws-proxy/* /api/* /oauth2/*
    reverse_proxy @backend ${server_addr}

    # Dashboard (everything else)
    reverse_proxy /* ${dashboard_addr}
}
EOF
  return 0
}

render_npm_advanced_config() {
  local upstream_host=$(get_upstream_host)
  local server_addr="${upstream_host}:${MANAGEMENT_HOST_PORT}"

  # If external network is specified, use container names instead of host addresses
  if [[ -n "$EXTERNAL_PROXY_NETWORK" ]]; then
    server_addr="anonbird-server:80"
  fi

  cat <<EOF
# Advanced Configuration for Nginx Proxy Manager
# Paste this into the "Advanced" tab of your Proxy Host configuration
#
# IMPORTANT: Enable "HTTP/2 Support" in the SSL tab for gRPC to work!

# Required for long-lived connections (gRPC and WebSocket)
client_header_timeout 1d;
client_body_timeout 1d;

# WebSocket connections (relay, signal, management)
location ~ ^/(relay|ws-proxy/) {
    proxy_pass http://${server_addr};
    proxy_http_version 1.1;
    proxy_set_header Upgrade \$http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_read_timeout 1d;
}

# Native gRPC (signal + management)
location ~ ^/(signalexchange\.SignalExchange|management\.ManagementService)/ {
    grpc_pass grpc://${server_addr};
    grpc_read_timeout 1d;
    grpc_send_timeout 1d;
    grpc_socket_keepalive on;
}

# HTTP routes (API + OAuth2)
location ~ ^/(api|oauth2)/ {
    proxy_pass http://${server_addr};
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
}
EOF
  return 0
}

############################################
# Post-Setup Instructions per Proxy Type
############################################

print_builtin_traefik_instructions() {
  echo ""
  echo "$MSG_SEPARATOR"
  echo "  ANONBIRD SETUP COMPLETE"
  echo "$MSG_SEPARATOR"
  echo ""
  echo "You can access the AnonBird dashboard at:"
  echo "  $NETBIRD_HTTP_PROTOCOL://$NETBIRD_DOMAIN"
  echo ""
  echo "Follow the onboarding steps to set up your AnonBird instance."
  echo ""
  echo "Traefik is handling TLS certificates automatically via Let's Encrypt."
  echo "If you see certificate warnings, wait a moment for certificate issuance to complete."
  echo ""
  echo "Open ports:"
  echo "  - 443/tcp   (HTTPS - all AnonBird services)"
  echo "  - 80/tcp    (HTTP - redirects to HTTPS)"
  if clearnet_stun_enabled; then
    echo "  - $NETBIRD_STUN_PORT/udp   (STUN - explicitly enabled for non-anonymous legacy clients)"
  else
    echo "  - STUN/UDP is disabled by default for anonymous deployments."
    echo "    Use --enable-clearnet-stun only if you deliberately accept legacy real-IP exposure."
  fi
  if [[ "$ENABLE_PROXY" == "true" ]]; then
    echo "  - 51820/udp (WIREGUARD - (optional) for P2P proxy connections)"
  fi
  echo ""
  echo "This setup is ideal for homelabs and smaller organization deployments."
  echo "For enterprise environments requiring high availability and advanced integrations,"
  echo "consider a commercial on-prem license or scaling your open source deployment:"
  echo ""
  echo "  Project: https://github.com/Cr0me1ve/anonbird"
  echo "  Docs:      https://github.com/Cr0me1ve/anonbird/tree/main/docs"
  echo ""
  if [[ "$ENABLE_PROXY" == "true" ]]; then
    echo "AnonBird Proxy:"
    echo "  The proxy service is enabled and running."
    echo "  Any domain NOT matching $NETBIRD_DOMAIN will be passed through to the proxy."
    echo "  The proxy handles its own TLS certificates via ACME TLS-ALPN-01 challenge."
    echo "  Point your proxy domain to this server's domain address like in the examples below:"
    echo ""
    echo "  *.$NETBIRD_DOMAIN    CNAME    $NETBIRD_DOMAIN"
    echo ""
    if [[ "$ENABLE_CROWDSEC" == "true" ]]; then
      echo "CrowdSec IP Reputation:"
      echo "  CrowdSec LAPI is running and connected to the community blocklist."
      echo "  The proxy will automatically check client IPs against known threats."
      echo "  Enable CrowdSec per-service in the dashboard under Access Control."
      echo ""
      echo "  To enroll in CrowdSec Console (optional, for dashboard and premium blocklists):"
      echo "    docker exec anonbird-crowdsec cscli console enroll <your-enrollment-key>"
      echo "  Get your enrollment key at: https://app.crowdsec.net"
      echo ""
    fi
  fi
  return 0
}

print_traefik_instructions() {
  echo ""
  echo "$MSG_SEPARATOR"
  echo "  TRAEFIK SETUP"
  echo "$MSG_SEPARATOR"
  echo ""
  echo "AnonBird containers are configured with Traefik labels."
  echo ""
  echo "Configuration:"
  echo "  Entrypoint: $TRAEFIK_ENTRYPOINT"
  if [[ -n "$TRAEFIK_CERTRESOLVER" ]]; then
    echo "  Certificate resolver: $TRAEFIK_CERTRESOLVER"
  fi
  if [[ -n "$TRAEFIK_EXTERNAL_NETWORK" ]]; then
    echo "  Network: $TRAEFIK_EXTERNAL_NETWORK (external)"
  else
    echo "  Network: anonbird"
  fi
  echo ""
  echo "$MSG_NEXT_STEPS"
  echo "  - Ensure Traefik is running and configured"
  if [[ -n "$TRAEFIK_EXTERNAL_NETWORK" ]]; then
    echo "  - Traefik must be on the '$TRAEFIK_EXTERNAL_NETWORK' network"
  fi
  echo "  - Entrypoint '$TRAEFIK_ENTRYPOINT' must be defined"
  if [[ -n "$TRAEFIK_CERTRESOLVER" ]]; then
    echo "  - Certificate resolver '$TRAEFIK_CERTRESOLVER' must be configured"
  fi
  echo "  - Disable read timeout on the entrypoint for gRPC streams:"
  echo "    --entrypoints.$TRAEFIK_ENTRYPOINT.transport.respondingTimeouts.readTimeout=0"
  echo "  - HTTP to HTTPS redirect (recommended)"
  return 0
}

print_nginx_instructions() {
  local bind_addr=$(get_bind_address)
  echo ""
  echo "$MSG_SEPARATOR"
  echo "  NGINX SETUP"
  echo "$MSG_SEPARATOR"
  echo ""
  echo "Generated: nginx-anonbird.conf"
  echo ""
  echo "IMPORTANT: Nginx requires manual TLS certificate setup."
  echo "You'll need to obtain SSL/TLS certificates and configure the paths in the"
  echo "generated config file. The config includes examples for common certificate sources."
  echo ""
  if [[ -n "$EXTERNAL_PROXY_NETWORK" ]]; then
    echo "AnonBird containers have joined the '$EXTERNAL_PROXY_NETWORK' Docker network."
    echo "The config uses container names for upstream servers."
    echo ""
    echo "$MSG_NEXT_STEPS"
    echo "  1. Ensure your Nginx container has access to SSL certificates"
    echo "     (mount certificate directory as volume if needed)"
    echo "  2. Edit nginx-anonbird.conf and update SSL certificate paths"
    echo "     The config includes examples for certbot, acme.sh, and custom certs"
    echo "  3. Include the config in your Nginx container's configuration"
    echo "  4. Reload Nginx"
  else
    echo "$MSG_NEXT_STEPS"
    echo "  1. Obtain SSL/TLS certificates (Let's Encrypt recommended)"
    echo "  2. Edit nginx-anonbird.conf and update certificate paths"
    echo "  3. Install to /etc/nginx/sites-available/ (Debian) or /etc/nginx/conf.d/ (RHEL)"
    echo "  4. Test and reload: nginx -t && systemctl reload nginx"
    echo ""
    echo "For detailed TLS setup instructions, see:"
    echo "https://github.com/Cr0me1ve/anonbird/tree/main/docs"
    echo ""
    echo "Container ports (bound to ${bind_addr}):"
    echo "  Dashboard:     ${DASHBOARD_HOST_PORT}"
    echo "  AnonBird Server: ${MANAGEMENT_HOST_PORT} (all services)"
  fi
  return 0
}

print_npm_instructions() {
  local bind_addr=$(get_bind_address)
  local upstream_host=$(get_upstream_host)
  echo ""
  echo "$MSG_SEPARATOR"
  echo "  NGINX PROXY MANAGER SETUP"
  echo "$MSG_SEPARATOR"
  echo ""
  echo "Generated: npm-advanced-config.txt"
  echo ""
  if [[ -n "$EXTERNAL_PROXY_NETWORK" ]]; then
    echo "AnonBird containers have joined the '$EXTERNAL_PROXY_NETWORK' Docker network."
    echo ""
    echo "In NPM, create a Proxy Host:"
    echo "  Domain: $NETBIRD_DOMAIN"
    echo "  Forward Hostname: anonbird-dashboard"
    echo "  Forward Port: 80"
    echo "  Block Common Exploits: enabled"
    echo ""
    echo "  SSL tab:"
    echo "    - Request or select existing certificate"
    echo "    - Enable 'HTTP/2 Support' (REQUIRED for gRPC)"
    echo ""
    echo "  Advanced tab:"
    echo "    - Paste contents of npm-advanced-config.txt"
  else
    echo "Container ports (bound to ${bind_addr}):"
    echo "  Dashboard:     ${DASHBOARD_HOST_PORT}"
    echo "  AnonBird Server: ${MANAGEMENT_HOST_PORT} (all services)"
    echo ""
    echo "In NPM, create a Proxy Host:"
    echo "  Domain: $NETBIRD_DOMAIN"
    echo "  Forward Hostname/IP: ${upstream_host}"
    echo "  Forward Port: ${DASHBOARD_HOST_PORT}"
    echo "  Block Common Exploits: enabled"
    echo ""
    echo "  SSL tab:"
    echo "    - Request or select existing certificate"
    echo "    - Enable 'HTTP/2 Support' (REQUIRED for gRPC)"
    echo ""
    echo "  Advanced tab:"
    echo "    - Paste contents of npm-advanced-config.txt"
  fi
  return 0
}

print_external_caddy_instructions() {
  local bind_addr=$(get_bind_address)
  echo ""
  echo "$MSG_SEPARATOR"
  echo "  EXTERNAL CADDY SETUP"
  echo "$MSG_SEPARATOR"
  echo ""
  echo "Generated: caddyfile-anonbird.txt"
  echo ""
  if [[ -n "$EXTERNAL_PROXY_NETWORK" ]]; then
    echo "AnonBird containers have joined the '$EXTERNAL_PROXY_NETWORK' Docker network."
    echo "The config uses container names for upstream servers."
    echo ""
    echo "$MSG_NEXT_STEPS"
    echo "  1. Add the contents of caddyfile-anonbird.txt to your Caddyfile"
    echo "  2. Reload Caddy"
  else
    echo "$MSG_NEXT_STEPS"
    echo "  1. Add the contents of caddyfile-anonbird.txt to your Caddyfile"
    echo "  2. Reload Caddy: caddy reload --config /path/to/Caddyfile"
    echo ""
    echo "Container ports (bound to ${bind_addr}):"
    echo "  Dashboard:     ${DASHBOARD_HOST_PORT}"
    echo "  AnonBird Server: ${MANAGEMENT_HOST_PORT} (all services)"
  fi
  return 0
}

print_manual_instructions() {
  local bind_addr=$(get_bind_address)
  local upstream_host=$(get_upstream_host)
  echo ""
  echo "$MSG_SEPARATOR"
  echo "  MANUAL REVERSE PROXY SETUP"
  echo "$MSG_SEPARATOR"
  echo ""
  echo "Container ports (bound to ${bind_addr}):"
  echo "  Dashboard:     ${DASHBOARD_HOST_PORT}"
  echo "  AnonBird Server: ${MANAGEMENT_HOST_PORT} (all services: management, signal, relay)"
  echo ""
  echo "Configure your reverse proxy with these routes (all go to the same backend):"
  echo ""
  echo "  WebSocket (relay, signal, management WS proxy):"
  echo "    /relay*, /ws-proxy/*           -> ${upstream_host}:${MANAGEMENT_HOST_PORT}"
  echo "    (HTTP with WebSocket upgrade, extended timeout)"
  echo ""
  echo "  Native gRPC (signal + management):"
  echo "    /signalexchange.SignalExchange/* -> ${upstream_host}:${MANAGEMENT_HOST_PORT}"
  echo "    /management.ManagementService/* -> ${upstream_host}:${MANAGEMENT_HOST_PORT}"
  echo "    (gRPC/h2c - plaintext HTTP/2)"
  echo ""
  echo "  HTTP (API + embedded IdP):"
  echo "    /api/*, /oauth2/*              -> ${upstream_host}:${MANAGEMENT_HOST_PORT}"
  echo ""
  echo "  Dashboard (catch-all):"
  echo "    /*                             -> ${upstream_host}:${DASHBOARD_HOST_PORT}"
  echo ""
  echo "IMPORTANT: gRPC routes require HTTP/2 (h2c) upstream support."
  echo "WebSocket and gRPC connections need extended timeouts (recommend 1 day)."
  return 0
}

print_post_setup_instructions() {
  echo ""
  echo "Working directory:"
  echo "  $ANONBIRD_WORKDIR"
  echo ""
  echo "Run Docker Compose commands from that directory:"
  echo "  cd $ANONBIRD_WORKDIR"
  if [[ -n "$ANONBIRD_PEER_MANAGEMENT_ENDPOINT" ]]; then
    echo ""
    echo "Anonymous peer management endpoint:"
    echo "  $ANONBIRD_PEER_MANAGEMENT_ENDPOINT"
    echo "Peer install commands copied from the dashboard will use this URL."
    if [[ -f anonymous-endpoints.env ]]; then
      echo "All generated anonymous endpoints were saved to anonymous-endpoints.env."
    fi
  fi

  case "$REVERSE_PROXY_TYPE" in
    0)
      print_builtin_traefik_instructions
      ;;
    1)
      print_traefik_instructions
      ;;
    2)
      print_nginx_instructions
      ;;
    3)
      print_npm_instructions
      ;;
    4)
      print_external_caddy_instructions
      ;;
    5)
      print_manual_instructions
      ;;
    *)
      echo "Unknown reverse proxy type: $REVERSE_PROXY_TYPE" > /dev/stderr
      ;;
  esac
  return 0
}

init_environment "$@"
