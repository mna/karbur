#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "$0")/common.bash"

# fail if the database secrets or data directory do not exist
if [[ ! -d "${dbrootdir}/secrets" && ! -d "${dbrootdir}/data" ]]; then
	echo "The ${dbrootdir}/{secrets,data} directory do not exist, "
	echo "no database to destroy. Use the 'db_init' script to initialize one."
	exit 1
fi
#
# the docker command is required to bring down the database
if ! command -v docker >/dev/null 2>&1; then
	echo "The 'docker' command is required, see https://docs.docker.com/get-started/get-docker/ for instructions."
	exit 1
fi

docker compose down -v
rm -rf "${dbrootdir}/secrets"
sudo --prompt="(sudo required to remove the data directory): " \
	--reset-timestamp \
	rm -rf "${dbrootdir}/data"
