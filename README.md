# karbur

## Local Development

### Requirements

* Docker or Podman with docker compatibility
* Direnv: https://direnv.net/docs/installation.html
* mkcert: `go install filippo.io/mkcert@latest`

### Setup

You may initialize your local environment for developing this package with
the following command, executed in the root of the repository:

```
$ ./_scripts/env_init.bash
$ ./_scripts/db_init.bash
$ ./_scripts/cert_init.bash
```

### Environment Variables

A `direnv`-managed `.envrc` file should exist at the root of the repository,
although that file is not version-controlled. The `env_init.bash` script
creates it with default values. Required variables are (values may be adjusted
as needed):

* `PGPASSFILE=$(pwd)/_db/secrets/pgpass`
* `PGHOST=localhost`
* `PGPORT=5432`
* `PGCONNECT_TIMEOUT=10`
* `PGUSER=postgres`
* `PGDATABASE=postgres`
* `PGSSLMODE=prefer`

Extra variables are required for the TLS tests, created automatically by the
`cert_init.bash` script:

* `KARBUR_TEST_LOCALHOST_CERT=$(pwd)/_certs/cert.pem`
* `KARBUR_TEST_LOCALHOST_KEY=$(pwd)/_certs/key.pem`

## License

The [BSD 3-Clause license](http://opensource.org/licenses/BSD-3-Clause).

