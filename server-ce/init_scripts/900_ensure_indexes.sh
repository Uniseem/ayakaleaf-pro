#!/bin/sh
set -e

# The indexes every service relies on.
#
# These used to be a set of migrations run by a Node tool: one file per change,
# going back years. What survives is the end state, which is the only part a
# deployment needs -- the steps in between were about moving old data from one
# shape to another, and creating an index that is already there does nothing.

echo "Ensuring database indexes"
/sbin/setuser www-data /overleaf/bin/go/setup ensure-indexes
