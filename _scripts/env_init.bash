#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "$0")/common.bash"

if [[ -f "${envfile}" ]]; then
	echo "The ${envfile} already exists, remove it to re-initialize "
	echo "(note that 'db_destroy' should be executed first if needed)."
	exit 1
fi

if ! command -v direnv >/dev/null 2>&1; then
	echo "The 'direnv' command is required, see https://direnv.net/ for instructions."
	exit 1
fi

cat <<- EOF > "${envfile}"
	export PGPASSFILE=\$(pwd)/${pgpassfile}
	export PGHOST=localhost
	export PGPORT=5432
	export PGCONNECT_TIMEOUT=10
	export PGUSER=postgres
	export PGDATABASE=postgres
	EOF

direnv allow .
