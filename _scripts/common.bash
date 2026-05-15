envfile=".envrc"
dbrootdir="_db"
certsdir="_certs"
pgpassfile="${dbrootdir}/secrets/pgpass"
pgpwdfile="${dbrootdir}/secrets/pgpwd"

# check that the script is executed from the root of the repository
if [[ ! -d "${dbrootdir}" ]]; then
	echo "The scripts must be executed from the root of the repository."
	exit 1
fi
