#!/usr/bin/env sh
set -e

if [ -z "${LABELGUN_ERR_THRESHOLD}" ]; then
  LABELGUN_ERR_THRESHOLD="WARNING"
fi

exec labelgun -stderrthreshold="${LABELGUN_ERR_THRESHOLD}"
