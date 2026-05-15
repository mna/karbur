# pgdb

Package `pgdb` streamlines the database API by exposing only the context-aware
version of methods and enhances querying by supporting scanning into structs
and slices. It abstract the various `postgresql` database drivers that may be
used, the standard library's database/sql and github.com/jackc/pgx/v5 are
supported and the sqladapt or pgxadapt packages can be used to convert from
the specific type to the abstraction.

