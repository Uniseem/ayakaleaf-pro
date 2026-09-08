#!/bin/sh
set -e

# Mongo and Redis, before anything that needs them.
#
# Without this every service fails one at a time and restarts for ever, which
# is a much harder thing to read than one message here.

echo "Checking can connect to mongo and redis"
/sbin/setuser www-data /overleaf/bin/go/setup check-databases
echo "All checks passed"
