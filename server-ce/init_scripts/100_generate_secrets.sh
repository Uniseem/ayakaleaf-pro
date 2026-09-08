#!/bin/bash
set -e -o pipefail

# Generates the secrets the site needs, so that installing it takes no
# configuration at all.
# https://github.com/phusion/baseimage-docker#centrally-defining-your-own-environment-variables
#
# A secret that changes invalidates whatever was signed with it -- sessions,
# invite tokens -- so these are kept on the data volume rather than only inside
# the container: recreating the container to upgrade it must not sign everybody
# out. A value supplied in the environment still wins, which is what keeps an
# existing deployment on exactly the secrets it already had.

ENVIRONMENT_DIR=/etc/container_environment
SECRETS_DIR=/var/lib/overleaf/data/secrets

SECRETS="
WEB_API_PASSWORD
CRYPTO_RANDOM
OT_JWT_AUTH_KEY
OVERLEAF_SESSION_SECRET
OVERLEAF_INVITE_TOKEN_SECRET
"

generate_secret () {
  dd if=/dev/urandom bs=1 count=32 2>/dev/null | base64 -w 0 | rev | cut -b 2- | rev | tr -d '\n+/'
}

# Holds a secret if we do not already have one, and puts it in the environment
# the services are started with.
ensure_secret () {
  local name=$1

  if [ -s "${ENVIRONMENT_DIR}/${name}" ]; then
    # Supplied in the environment. That is the value, and it is not ours to keep.
    return
  fi

  if [ ! -s "${SECRETS_DIR}/${name}" ]; then
    generate_secret > "${SECRETS_DIR}/${name}"
    chmod 600 "${SECRETS_DIR}/${name}"
    echo "generated ${name}"
  fi

  cp "${SECRETS_DIR}/${name}" "${ENVIRONMENT_DIR}/${name}"
}

mkdir -p "${SECRETS_DIR}"
chmod 700 "${SECRETS_DIR}"

for name in ${SECRETS}; do
  ensure_secret "${name}"
done

# history-v1 checks the password it is handed against the one it was told to
# expect, so these two names are one secret.
ensure_secret STAGING_PASSWORD
if [ ! -s "${ENVIRONMENT_DIR}/V1_HISTORY_PASSWORD" ]; then
  cp "${ENVIRONMENT_DIR}/STAGING_PASSWORD" "${ENVIRONMENT_DIR}/V1_HISTORY_PASSWORD"
fi
