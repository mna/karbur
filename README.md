# karbur

## Local Development

You may initialize your local environment for developing this package with
the following command, executed in the root of the repository:

```
$ ./scripts/env_init.bash
$ ./scripts/db_init.bash
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

## License

The [BSD 3-Clause license](http://opensource.org/licenses/BSD-3-Clause).

