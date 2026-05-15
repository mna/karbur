#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "$0")/common.bash"

# fail if the .envrc file does not exist
if [[ ! -f "${envfile}" ]]; then
	echo "The ${envfile} does not exist, use the 'env_init' script "
	echo "first to initialize environment variables."
	exit 1
fi

if ! command -v direnv >/dev/null 2>&1; then
	echo "The 'direnv' command is required, see https://direnv.net/ for instructions."
	exit 1
fi

# the mkcert command is required to generate test certs
if ! command -v mkcert >/dev/null 2>&1; then
	echo "The 'mkcert' command is required, see https://github.com/filosottile/mkcert for instructions."
	exit 1
fi

mkdir -p "${certsdir}"

certfile="${certsdir}/cert.pem"
keyfile="${certsdir}/key.pem"
cat <<- EOF >> "${envfile}"
	export KARBUR_TEST_LOCALHOST_CERT=${certfile}
	export KARBUR_TEST_LOCALHOST_KEY=${keyfile}
	EOF

direnv allow .

mkcert -install
mkcert -cert-file "$certfile" -key-file "$keyfile" localhost 127.0.0.1 ::1
