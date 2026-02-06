#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://127.0.0.1"
ADMIN_EMAIL=""
ADMIN_PASSWORD=""
DOMAIN=""
PHP_VERSION="8.3"
KEEP_RESOURCES=0

COOKIE_JAR=""
SITE_ID=""
DB_ID=""
ROOT_DIR=""
SYSTEM_USER=""
DB_NAME=""
DB_USER=""
SUDO_CMD=""

usage() {
  cat <<USAGE
Usage:
  scripts/smoke-sprint2.sh --admin-email <email> --admin-password <password> [options]

Required:
  --admin-email <email>       Admin email used for /api/auth/login
  --admin-password <password> Admin password used for /api/auth/login

Options:
  --base-url <url>            API base URL (default: http://127.0.0.1)
  --domain <domain>           Domain to create (default: smoke-<ts>.example.test)
  --php-version <version>     PHP version for site (default: 8.3)
  --keep                      Keep created site/database for manual inspection
  -h, --help                  Show this help

Examples:
  sudo scripts/smoke-sprint2.sh \\
    --admin-email admin@example.com \\
    --admin-password 'ChangeMe12345!'

  sudo scripts/smoke-sprint2.sh \\
    --base-url http://127.0.0.1 \\
    --admin-email admin@example.com \\
    --admin-password 'ChangeMe12345!' \\
    --domain smoke-20260206.example.test \\
    --php-version 8.4
USAGE
}

log() {
  printf '==> %s\n' "$*"
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

json_quote() {
  local v="$1"
  v=${v//\\/\\\\}
  v=${v//\"/\\\"}
  v=${v//$'\n'/\\n}
  printf '"%s"' "$v"
}

extract_json_number() {
  local body="$1"
  local key="$2"
  printf '%s' "$body" | tr -d '\n' | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p" | head -n 1
}

extract_json_string() {
  local body="$1"
  local key="$2"
  printf '%s' "$body" | tr -d '\n' | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -n 1
}

root_run() {
  if [[ -n "$SUDO_CMD" ]]; then
    "$SUDO_CMD" "$@"
  else
    "$@"
  fi
}

api_request() {
  local method="$1"
  local path="$2"
  local expected_status="$3"
  local payload="${4:-}"

  local tmp_body
  local status
  tmp_body=$(mktemp)

  local -a curl_args
  curl_args=(
    -sS
    -o "$tmp_body"
    -w '%{http_code}'
    -X "$method"
    -H 'Accept: application/json'
    -b "$COOKIE_JAR"
    -c "$COOKIE_JAR"
    "${BASE_URL%/}${path}"
  )

  if [[ -n "$payload" ]]; then
    curl_args+=(-H 'Content-Type: application/json' --data "$payload")
  fi

  if ! status=$(curl "${curl_args[@]}"); then
    RESPONSE_BODY=$(cat "$tmp_body" 2>/dev/null || true)
    rm -f "$tmp_body"
    printf 'Request failed: %s %s (transport error)\n' "$method" "$path" >&2
    printf 'Response: %s\n' "$RESPONSE_BODY" >&2
    return 1
  fi
  RESPONSE_BODY=$(cat "$tmp_body")
  rm -f "$tmp_body"

  if [[ "$status" != "$expected_status" ]]; then
    printf 'Request failed: %s %s (expected %s, got %s)\n' "$method" "$path" "$expected_status" "$status" >&2
    printf 'Response: %s\n' "$RESPONSE_BODY" >&2
    return 1
  fi
}

compute_pool_name() {
  local domain="$1"
  local php_version="$2"
  local ver="${php_version//./}"
  local token

  token=$(printf '%s' "$domain" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9._-]/-/g; s/[._]/-/g; s/--*/-/g; s/^-*//; s/-*$//')
  if [[ -z "$token" ]]; then
    token="site"
  fi

  local name="${token}-php${ver}"
  if (( ${#name} > 48 )); then
    name="${name:0:48}"
  fi
  printf '%s' "$name"
}

verify_prereqs() {
  command -v curl >/dev/null 2>&1 || fail "curl is required"
  command -v sed >/dev/null 2>&1 || fail "sed is required"
  command -v systemctl >/dev/null 2>&1 || fail "systemctl is required"
  command -v mariadb >/dev/null 2>&1 || fail "mariadb client is required"

  if [[ ${EUID} -ne 0 ]]; then
    command -v sudo >/dev/null 2>&1 || fail "run as root or install sudo"
    SUDO_CMD="sudo"
  fi
}

cleanup() {
  local exit_code=$?

  if [[ "$KEEP_RESOURCES" -eq 0 ]]; then
    if [[ -n "$DB_ID" && -n "$COOKIE_JAR" ]]; then
      log "Cleanup: deleting database id=${DB_ID}"
      if ! api_request "DELETE" "/api/databases/${DB_ID}" "204" ""; then
        printf 'WARN: cleanup database delete failed\n' >&2
      fi
      DB_ID=""
    fi

    if [[ -n "$SITE_ID" && -n "$COOKIE_JAR" ]]; then
      log "Cleanup: deleting site id=${SITE_ID}"
      if ! api_request "DELETE" "/api/sites/${SITE_ID}" "204" ""; then
        printf 'WARN: cleanup site delete failed\n' >&2
      fi
      SITE_ID=""
    fi
  fi

  if [[ -n "$COOKIE_JAR" ]]; then
    rm -f "$COOKIE_JAR"
  fi

  if [[ $exit_code -ne 0 ]]; then
    printf 'Smoke test FAILED\n' >&2
  fi
}

main() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --base-url)
        BASE_URL="${2:-}"
        shift 2
        ;;
      --admin-email)
        ADMIN_EMAIL="${2:-}"
        shift 2
        ;;
      --admin-password)
        ADMIN_PASSWORD="${2:-}"
        shift 2
        ;;
      --domain)
        DOMAIN="${2:-}"
        shift 2
        ;;
      --php-version)
        PHP_VERSION="${2:-}"
        shift 2
        ;;
      --keep)
        KEEP_RESOURCES=1
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done

  [[ -n "$ADMIN_EMAIL" ]] || fail "--admin-email is required"
  [[ -n "$ADMIN_PASSWORD" ]] || fail "--admin-password is required"

  if [[ -z "$DOMAIN" ]]; then
    DOMAIN="smoke-$(date +%s).example.test"
  fi

  COOKIE_JAR=$(mktemp)
  trap cleanup EXIT

  verify_prereqs

  log "Checking required services"
  root_run systemctl is-active --quiet nginx || fail "nginx is not active"
  root_run systemctl is-active --quiet mariadb || fail "mariadb is not active"
  root_run systemctl is-active --quiet "php${PHP_VERSION}-fpm" || fail "php${PHP_VERSION}-fpm is not active"

  log "Checking panel health"
  curl -fsS "${BASE_URL%/}/health" >/dev/null || fail "health endpoint is not reachable"

  log "Logging in as admin"
  api_request "POST" "/api/auth/login" "200" "{\"email\":$(json_quote "$ADMIN_EMAIL"),\"password\":$(json_quote "$ADMIN_PASSWORD")}" || fail "login failed"

  log "Creating site domain=${DOMAIN} php=${PHP_VERSION}"
  api_request "POST" "/api/sites" "201" "{\"domain\":$(json_quote "$DOMAIN"),\"php_version\":$(json_quote "$PHP_VERSION")}" || fail "site create failed"
  SITE_ID=$(extract_json_number "$RESPONSE_BODY" "id")
  ROOT_DIR=$(extract_json_string "$RESPONSE_BODY" "root_dir")
  SYSTEM_USER=$(extract_json_string "$RESPONSE_BODY" "system_user")
  [[ -n "$SITE_ID" ]] || fail "cannot parse site id"
  [[ -n "$ROOT_DIR" ]] || fail "cannot parse root_dir"

  log "Verifying site API read"
  api_request "GET" "/api/sites/${SITE_ID}" "200" "" || fail "site get failed"
  printf '%s' "$RESPONSE_BODY" | grep -q "\"domain\":\"${DOMAIN}\"" || fail "site payload does not contain expected domain"

  local vhost_path="/etc/nginx/sites-available/${DOMAIN}.conf"
  local symlink_path="/etc/nginx/sites-enabled/${DOMAIN}.conf"
  local pool_name
  local pool_path

  pool_name=$(compute_pool_name "$DOMAIN" "$PHP_VERSION")
  pool_path="/etc/php/${PHP_VERSION}/fpm/pool.d/${pool_name}.conf"

  log "Verifying system artifacts (vhost, symlink, pool, docroot)"
  root_run test -f "$vhost_path" || fail "missing nginx vhost: $vhost_path"
  root_run test -L "$symlink_path" || fail "missing nginx enabled symlink: $symlink_path"
  root_run test -f "$pool_path" || fail "missing php-fpm pool: $pool_path"
  root_run test -d "$ROOT_DIR" || fail "missing docroot: $ROOT_DIR"

  log "Writing index.php and checking HTTP serving through nginx + php-fpm"
  root_run bash -lc "cat > '$ROOT_DIR/index.php' <<'PHP'
<?php
echo 'aipanel-smoke-ok';
PHP"
  if [[ -n "$SYSTEM_USER" ]]; then
    root_run chown "$SYSTEM_USER:$SYSTEM_USER" "$ROOT_DIR/index.php" || true
  fi

  local body
  body=$(curl -fsS -H "Host: ${DOMAIN}" "http://127.0.0.1/") || fail "site HTTP check failed"
  [[ "$body" == "aipanel-smoke-ok" ]] || fail "unexpected site response: $body"

  DB_NAME="db_$(date +%s)"
  log "Creating MariaDB database db_name=${DB_NAME}"
  api_request "POST" "/api/sites/${SITE_ID}/databases" "201" "{\"db_name\":$(json_quote "$DB_NAME")}" || fail "database create failed"
  DB_ID=$(extract_json_number "$RESPONSE_BODY" "id")
  DB_USER=$(extract_json_string "$RESPONSE_BODY" "db_user")
  local db_password
  db_password=$(extract_json_string "$RESPONSE_BODY" "password")
  [[ -n "$DB_ID" ]] || fail "cannot parse database id"
  [[ -n "$DB_USER" ]] || fail "cannot parse db user"
  [[ -n "$db_password" ]] || fail "cannot parse one-time db password"

  log "Verifying database exists in MariaDB"
  root_run mariadb -N -e "SHOW DATABASES LIKE '${DB_NAME}';" | grep -qx "${DB_NAME}" || fail "database not found in MariaDB"
  root_run mariadb -N -e "SELECT User FROM mysql.user WHERE User='${DB_USER}' AND Host='localhost';" | grep -qx "${DB_USER}" || fail "database user not found in MariaDB"

  log "Deleting database id=${DB_ID}"
  api_request "DELETE" "/api/databases/${DB_ID}" "204" "" || fail "database delete failed"
  DB_ID=""
  root_run mariadb -N -e "SHOW DATABASES LIKE '${DB_NAME}';" | grep -q . && fail "database still exists after delete" || true
  root_run mariadb -N -e "SELECT User FROM mysql.user WHERE User='${DB_USER}' AND Host='localhost';" | grep -q . && fail "database user still exists after delete" || true

  log "Deleting site id=${SITE_ID}"
  api_request "DELETE" "/api/sites/${SITE_ID}" "204" "" || fail "site delete failed"
  SITE_ID=""

  log "Verifying cleanup of system artifacts"
  root_run test ! -e "$vhost_path" || fail "nginx vhost still exists"
  root_run test ! -e "$symlink_path" || fail "nginx symlink still exists"
  root_run test ! -e "$pool_path" || fail "php-fpm pool still exists"
  root_run test ! -d "$(dirname "$ROOT_DIR")" || fail "site directory still exists"

  log "Smoke test PASSED"
}

main "$@"
