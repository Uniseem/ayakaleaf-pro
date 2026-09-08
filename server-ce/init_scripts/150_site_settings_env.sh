#!/bin/sh
set -e

# Materialises the settings an administrator changed into the environment every
# service is started with. Runs before the nginx config is generated, because
# the upload limit is one of them.
#
# It never fails the startup: a database that is not up yet is the business of
# 500_check_db_access.sh, which says so properly.

echo "Applying site settings from the database"
# Runs as root: /etc/container_environment is root's, and this writes into it.
/overleaf/bin/go/setup environment || true
