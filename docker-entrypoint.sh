#!/bin/sh
set -e

if [ ! -w /data ]; then
    echo "ERROR: /data is not writable by uptop user (UID $(id -u))." >&2
    echo "" >&2
    echo "If upgrading from a previous version that ran as root:" >&2
    echo "  docker run --rm -v <your_volume>:/data alpine chown -R 1000:1000 /data" >&2
    exit 1
fi

mkdir -p /data/.ssh

exec "$@"
