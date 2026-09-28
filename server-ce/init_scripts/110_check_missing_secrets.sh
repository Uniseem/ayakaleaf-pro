#!/bin/bash

set -e

# Runs after 100_generate_secrets.sh, and checks its work rather than the
# operator's: nothing here has to be configured any more. A secret still
# missing at this point means the generator could not do its job -- almost
# always a data volume it cannot write to -- and starting up anyway would give
# a confusing failure much later on.

REQUIRED_SECRETS="
WEB_API_PASSWORD
CRYPTO_RANDOM
OT_JWT_AUTH_KEY
STAGING_PASSWORD
V1_HISTORY_PASSWORD
OVERLEAF_SESSION_SECRET
OVERLEAF_INVITE_TOKEN_SECRET
"

MISSING_ITEMS=""
for name in ${REQUIRED_SECRETS}; do
  if [[ -z "${!name}" ]]; then
    MISSING_ITEMS="$MISSING_ITEMS $name"
  fi
done

if [[ "$MISSING_ITEMS" == "" ]]; then
  exit 0
fi

MISSING_ITEMS=$(echo "$MISSING_ITEMS" | xargs -n1 | sed 's/^/      - /')
N=$(echo "$MISSING_ITEMS" | wc -l)
cat <<EOF
------------------------------------------------------------------------

                     Missing required secrets
                     ------------------------

  $N secret(s) could not be generated:
$MISSING_ITEMS

  These are generated on first startup and kept in

      /var/lib/overleaf/data/secrets

  so that they stay the same across restarts and upgrades. Losing them
   signs everybody out and invalidates invite tokens already sent.

  This almost always means that directory could not be written to.
   Check that the data volume is mounted and writable:

      docker compose exec ayakaleaf touch /var/lib/overleaf/data/secrets/test

  You can also set any of them yourself, in the environment of the
   ayakaleaf service, and that value will be used instead:

      openssl rand -base64 32


  Refusing to startup, exiting in 10s.

------------------------------------------------------------------------
EOF

sleep 10
exit 101
