#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "$0")/common.bash"

# fail if the database secrets or data directory already exists
if [[ -d "${dbrootdir}/secrets" || -d "${dbrootdir}/data" ]]; then
	echo "The ${dbrootdir}/{secrets,data} directory already exists, "
	echo "run the 'db_destroy' script first if you want to re-initialize it."
	exit 1
fi

# fail if the .envrc file does not exist (PG env vars are required)
if [[ ! -f "${envfile}" ]]; then
	echo "The ${envfile} does not exist, use the 'env_init' script "
	echo "first to initialize environment variables."
	exit 1
fi

# the docker command is required to bring up the database
if ! command -v docker >/dev/null 2>&1; then
	echo "The 'docker' command is required, see https://docs.docker.com/get-started/get-docker/ for instructions."
	exit 1
fi

mkdir -p "${dbrootdir}"/{secrets,data}

# create the postgres password, avoid problematic chars
head -c 32 /dev/urandom | base64 | tr /:  _= > "${pgpwdfile}"
chmod 0600 "${pgpwdfile}"

if command -v chcon >/dev/null 2>&1; then
	chcon -Rt svirt_sandbox_file_t "${pgpwdfile}"
fi

# create the postgres connection file (pgpass)
echo -n "${PGHOST}:${PGPORT}:*:${PGUSER}:" > "${pgpassfile}"
cat "${pgpwdfile}" >> "${pgpassfile}"
chmod 0600 "${pgpassfile}"

# reload direnv in case an old db already existed with a different pwd
direnv reload

# bring the database up
docker compose up --build --detach --wait

