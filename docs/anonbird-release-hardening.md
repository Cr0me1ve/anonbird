# AnonBird Release Hardening

This document records the release/update endpoints that must be configured by
AnonBird operators. The fork must not silently fall back to upstream NetBird
package, telemetry, upload, geolocation, or release-check hosts.

## Client and Management Version Checks

Remote release checks are opt-in.

| Component | Variable | Expected value |
|---|---|---|
| Client/daemon update watcher | `ANONBIRD_VERSION_URL` | Plain-text latest version endpoint. Empty disables the watcher fetch. |
| Desktop project link | `ANONBIRD_PROJECT_URL` | Project/source URL opened from the tray. Defaults to the AnonBird fork repository. |
| Desktop download link | `ANONBIRD_DOWNLOAD_URL` | Human download/release page. Defaults to the AnonBird fork releases page. |
| Management instance API | `ANONBIRD_MANAGEMENT_VERSION_URL` | Plain-text management latest version endpoint. Empty disables the API-side remote check. |
| Management instance API | `ANONBIRD_DASHBOARD_RELEASE_URL` | JSON release endpoint with `tag_name`. Empty disables dashboard remote check. |

## Auto-Update Artifacts

Installer downloads and signing material are separated. Production builds do not
embed an upstream signing-key host.

| Variable | Expected value |
|---|---|
| `ANONBIRD_RELEASE_BASE_URL` | Base URL containing release artifacts under `v<version>/...`. Defaults to the AnonBird fork GitHub release base. |
| `ANONBIRD_SIGNING_KEYS_BASE_URL` | Base URL for reposign keys and artifact signatures. Required before downloadable auto-update installers run. |
| `ANONBIRD_HOMEBREW_FORMULA` | Homebrew formula to upgrade for Homebrew-installed macOS clients. Required for Homebrew auto-update. |
| `ANONBIRD_HOMEBREW_UI_FORMULA` | Optional Homebrew UI formula upgraded when installed. |
| `ANONBIRD_HOMEBREW_TAP_PATH` | Optional Homebrew tap directory used to identify the brew owner. |

If `ANONBIRD_SIGNING_KEYS_BASE_URL` is not set, downloadable installer update
attempts fail closed before downloading artifacts.

## Debug Upload

Debug bundle upload is opt-in.

| Variable | Expected value |
|---|---|
| `ANONBIRD_DEBUG_UPLOAD_URL` | Full debug upload URL endpoint, including `/upload-url` if the service uses the bundled upload server API. |

CLI upload commands require `--upload-bundle-url` or `ANONBIRD_DEBUG_UPLOAD_URL`.
Remote bundle jobs fail closed when the upload URL is not configured.
Anonymous management URLs still reject clearnet upload and presigned URLs.

## Geolocation Databases

Management and proxy geolocation downloads are opt-in. Operators can also
pre-seed the database files in the configured data directory.

| Component | Variable | Expected value |
|---|---|---|
| Management | `ANONBIRD_GEOLITE_CITY_TAR_URL` | GeoLite2 City MMDB tarball URL. |
| Management | `ANONBIRD_GEOLITE_CITY_TAR_SHA256_URL` | SHA256 checksum URL for the MMDB tarball. |
| Management | `ANONBIRD_GEOLITE_CITY_CSV_ZIP_URL` | GeoLite2 City CSV zip URL. |
| Management | `ANONBIRD_GEOLITE_CITY_CSV_ZIP_SHA256_URL` | SHA256 checksum URL for the CSV zip. |
| Proxy | `ANONBIRD_PROXY_GEOLITE_MMDB_URL` | Proxy MMDB tarball URL. |
| Proxy | `ANONBIRD_PROXY_GEOLITE_MMDB_SHA256_URL` | SHA256 checksum URL for the proxy MMDB tarball. |

Anonymous `.onion`/`.i2p` management deployments already auto-disable geolite
updates; these variables are for explicitly configured non-anonymous release
operations.

## Reverse Proxy

The standalone reverse proxy no longer defaults to the upstream NetBird cloud.
Operators must set `--mgmt` or `NB_PROXY_MANAGEMENT_ADDRESS`.

## Release Candidate Audit Command

Use a targeted search before tagging an AnonBird build:

```bash
rg -n 'https?://[^"` ]*(pkgs\.netbird\.io|api\.netbird\.io|app\.netbird\.io|upload\.debug\.netbird\.io|publickeys\.netbird\.io|github\.com/netbirdio/netbird/releases|api\.github\.com/repos/netbirdio)|netbirdio/tap' \
  version client/server client/cmd client/internal/updater client/internal/debug client/jobexec client/android \
  management/server/instance management/server/geolocation management/internals/server \
  proxy/cmd proxy/internal/geolocation upload-server --glob '!**/*_test.go'
```

The command should return no matches for runtime/update code. Compatibility
identifiers such as Go module import paths are audited separately from external
runtime endpoints.
