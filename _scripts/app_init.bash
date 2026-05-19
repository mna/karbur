#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "$0")/common.bash"

if [[ "$#" -ne 1 ]]; then
	echo "Target directory where the app will be initialized is required."
	echo "Usage: $0 DIR"
	exit 1
fi

TARGET="${1%/}"

if [[ -d "$TARGET" ]] && [[ ! -z "$(ls -A "$TARGET")" ]]; then
	echo "Target directory must be empty or must not exist."
	echo "Usage: $0 DIR"
	exit 1
fi

mkdir -p "$TARGET/_db"

# copy all scripts, will all be useful in the app that uses karbur
cp -r _scripts "$TARGET/"
# copy the database configuration as a starting point
cp _db/.gitignore "$TARGET/_db/"
cp -r _db/config "$TARGET/_db/"

# copy the main gitignore and docker compose file
cp .gitignore "$TARGET/"
cp compose.yaml "$TARGET/"

# print the final instructions
echo "To finalize app initialization: "
echo 
echo "   > cd ${TARGET}"
echo "   > _scripts/env_init.bash"
echo
echo "   # review .envrc, adjust as necessary using 'direnv edit .'"
echo "   # review _db/config/postgres.conf, adjust as necessary"
echo
echo "   > _scripts/db_init.bash"
echo "   # if necessary - e.g. using localhost certs in development:"
echo "   > _scripts/cert_init.bash"
echo
echo "   # initialize the Go app and dependency:"
echo "   > go mod init"
echo "   > go get codeberg.org/mna/karbur"
echo 
echo "   # review .gitignore and adjust as necessary:"
echo "   > git init"
echo
echo " 🎉🎉🎉"
