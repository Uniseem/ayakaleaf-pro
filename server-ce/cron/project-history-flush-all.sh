#!/usr/bin/env bash

set -eu

echo "-------------------------------"
echo "Flush every project's history"
echo "-------------------------------"
date

PROJECT_HISTORY_URL='http://127.0.0.1:3054'

# timeout=0 takes every project rather than only the ones that have been
# waiting a while, which is what makes this the nightly sweep rather than the
# twenty-minute one.
curl -X POST "${PROJECT_HISTORY_URL}/flush/old?timeout=0&limit=100000&background=1"

echo "Done."
