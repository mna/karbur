package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

var ctx = context.Background()

// NewSQL creates a temporary test database and returns a *sql.DB
// that connects to it. The database is automatically dropped in the test
// cleanup phase, unless the PGDB_KEEP_TESTDB environment variable is set.
// If prefix is the empty string, "test" is used by default.
//
// If any fns are provided, they are called in order with a connection to
// the newly created test database to execute any setup required before the
// test. The MockPgcronSQL function from this package is one such function that
// can be used in this context.
func NewSQL(t testing.TB, connStr, prefix string, fns ...func(*sql.Conn) error) *sql.DB {
	if prefix == "" {
		prefix = "test"
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatal(err)
	}

	tempDB := createTestDBAndUser(t, &sqlDBExecer{db: db}, prefix)
	t.Cleanup(func() {
		if os.Getenv("PGDB_KEEP_TESTDB") == "" {
			_, _ = db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+tempDB+" WITH (FORCE)")
			_, _ = db.ExecContext(ctx, "DROP USER IF EXISTS "+tempDB)
		}
		_ = db.Close()
	})

	conf := connConfig(t, connStr, tempDB)
	connStrTemp := stdlib.RegisterConnConfig(conf)
	tdb, err := sql.Open("pgx", connStrTemp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tdb.Close() })
	if err := tdb.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	if len(fns) > 0 {
		conn, err := tdb.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close() //nolint
		for _, fn := range fns {
			if err := fn(conn); err != nil {
				t.Fatal(err)
			}
		}
	}
	return tdb
}

// NewPgx creates a temporary test database and returns a *pgxpool.Pool
// that connects to it. The database is automatically dropped in the test
// cleanup phase, unless the PGDB_KEEP_TESTDB environment variable is set.
// If prefix is the empty string, "test" is used by default.
//
// If any fns are provided, they are called in order with a connection to
// the newly created test database to execute any setup required before the
// test. The MockPgcronPgx function from this package is one such function that
// can be used in this context.
func NewPgx(t testing.TB, connStr, prefix string, fns ...func(*pgx.Conn) error) *pgxpool.Pool {
	if prefix == "" {
		prefix = "test"
	}

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		t.Fatal(err)
	}

	tempDB := createTestDBAndUser(t, &pgxExecer{conn: conn}, prefix)
	t.Logf("database name: %s", tempDB)
	t.Cleanup(func() {
		if os.Getenv("PGDB_KEEP_TESTDB") == "" {
			_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+tempDB+" WITH (FORCE)")
			_, _ = conn.Exec(ctx, "DROP USER IF EXISTS "+tempDB)
		}
		_ = conn.Close(ctx)
	})

	conf := poolConnConfig(t, connStr, tempDB)
	db, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if len(fns) > 0 {
		err = db.AcquireFunc(ctx, func(conn *pgxpool.Conn) error {
			for _, fn := range fns {
				if err := fn(conn.Conn()); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// MockPgcronPgx creates the database objects required to mock the pg_cron
// extension with a *pgx.Conn. Scheduling a job will not execute anything, but
// it will insert into the job table and update if the job name exists.
// Similarly, unschedule will remove the job from the table and fail if it does
// not exist.
//
// Because the pg_cron extension can only be created in the database
// configured as the pg_cron database in the postgres configuration file,
// it has to be mocked in any other database.
func MockPgcronPgx(conn *pgx.Conn) error {
	return mockPgcron(&pgxExecer{conn: conn})
}

// MockPgcronSQL creates the database objects required to mock the pg_cron
// extension with a *sql.Conn. Scheduling a job will not execute anything, but
// it will insert into the job table and update if the job name exists.
// Similarly, unschedule will remove the job from the table and fail if it does
// not exist.
//
// Because the pg_cron extension can only be created in the database
// configured as the pg_cron database in the postgres configuration file,
// it has to be mocked in any other database.
func MockPgcronSQL(conn *sql.Conn) error {
	return mockPgcron(&sqlConnExecer{conn: conn})
}

func mockPgcron(ex execer) error {
	stmts := []string{
		`
      CREATE SCHEMA cron
    `,
		`
      CREATE TABLE cron."job" (
        "jobid"     BIGSERIAL NOT NULL,
        "schedule"  TEXT      NOT NULL,
        "command"   TEXT      NOT NULL,
        "nodename"  TEXT      NOT NULL DEFAULT 'localhost',
        "nodeport"  INTEGER   NOT NULL DEFAULT inet_server_port(),
        "database"  TEXT      NOT NULL DEFAULT current_database(),
        "username"  TEXT      NOT NULL DEFAULT current_user,
        "active"    BOOLEAN   NOT NULL DEFAULT true,
        "jobname"   NAME      NULL,

        PRIMARY KEY ("jobid")
      )
    `,
		`
      CREATE UNIQUE INDEX
        "jobname_username_uniq"
      ON
        cron."job" ("jobname", "username")
    `,
		`
      CREATE POLICY
        "cron_job_policy"
      ON
        cron."job"
      USING
        ((username = CURRENT_USER))
    `,
		`
      CREATE TABLE cron."job_run_details" (
        "jobid"          BIGINT      NULL,
        "runid"          BIGSERIAL   NOT NULL,
        "job_pid"        INTEGER     NULL,
        "database"       TEXT        NULL,
        "username"       TEXT        NULL,
        "command"        TEXT        NULL,
        "status"         TEXT        NULL,
        "return_message" TEXT        NULL,
        "start_time"     TIMESTAMPTZ NULL,
        "end_time"       TIMESTAMPTZ NULL,

        PRIMARY KEY ("runid")
      )
    `,
		`
      CREATE POLICY
        "cron_job_run_details_policy"
      ON
        cron."job_run_details"
      USING
        ((username = CURRENT_USER))
    `,
		`
      CREATE FUNCTION cron.schedule(job_name text, schedule text, command text)
        RETURNS BIGINT VOLATILE LANGUAGE SQL
      AS $$
        INSERT INTO
          cron."job" (
            "jobname", "schedule", "command"
          )
        VALUES
          ($1, $2, $3)
        ON CONFLICT
          ("jobname", "username")
        DO UPDATE SET
          "schedule" = excluded."schedule",
          "command" = excluded."command"
        RETURNING
          "jobid"
      $$
    `,
		`
      CREATE FUNCTION cron.schedule(schedule text, command text)
        RETURNS BIGINT VOLATILE LANGUAGE SQL
      AS $$
        SELECT
          cron.schedule(NULL, schedule, command)
      $$
    `,
		`
      CREATE FUNCTION cron.unschedule(job_name text)
        RETURNS BOOLEAN VOLATILE LANGUAGE PLPGSQL
      AS $$
        BEGIN
          DELETE FROM
            cron."job"
          WHERE
            "jobname" = job_name;

          IF NOT FOUND THEN
            RAISE EXCEPTION 'could not find valid entry for job %', job_name;
          END IF;
          RETURN true;
        END
      $$
    `,
		`
      CREATE FUNCTION cron.unschedule(job_id bigint)
        RETURNS BOOLEAN VOLATILE LANGUAGE PLPGSQL
      AS $$
        BEGIN
          DELETE FROM
            cron."job"
          WHERE
            "jobid" = job_id;

          IF NOT FOUND THEN
            RAISE EXCEPTION 'could not find valid entry for job %', job_id;
          END IF;
          RETURN true;
        END
      $$
    `,
	}

	for _, stmt := range stmts {
		if _, err := ex.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func poolConnConfig(t testing.TB, original, tempDB string) *pgxpool.Config {
	conf, err := pgxpool.ParseConfig(original)
	if err != nil {
		t.Fatal(err)
	}

	conf.ConnConfig.Database = tempDB
	conf.ConnConfig.User = tempDB
	conf.ConnConfig.Password = tempDB
	m := conf.ConnConfig.RuntimeParams
	if m == nil {
		m = make(map[string]string)
		conf.ConnConfig.RuntimeParams = m
	}
	m["application_name"] = "testdb"

	return conf
}

func connConfig(t testing.TB, original, tempDB string) *pgx.ConnConfig {
	conf, err := pgx.ParseConfig(original)
	if err != nil {
		t.Fatal(err)
	}

	conf.Database = tempDB
	conf.User = tempDB
	conf.Password = tempDB
	m := conf.RuntimeParams
	if m == nil {
		m = make(map[string]string)
		conf.RuntimeParams = m
	}
	m["application_name"] = "testdb"

	return conf
}

type execer interface {
	Exec(context.Context, string, ...any) (any, error)
}

type sqlDBExecer struct {
	db *sql.DB
}

func (e *sqlDBExecer) Exec(ctx context.Context, stmt string, args ...any) (any, error) {
	return e.db.ExecContext(ctx, stmt, args...)
}

type sqlConnExecer struct {
	conn *sql.Conn
}

func (e *sqlConnExecer) Exec(ctx context.Context, stmt string, args ...any) (any, error) {
	return e.conn.ExecContext(ctx, stmt, args...)
}

type pgxExecer struct {
	conn *pgx.Conn
}

func (e *pgxExecer) Exec(ctx context.Context, stmt string, args ...any) (any, error) {
	return e.conn.Exec(ctx, stmt, args...)
}

func createTestDBAndUser(t testing.TB, ex execer, prefix string) string {
	rnd := randHex(t, 8)
	tempDB := strings.ToLower(prefix) + rnd
	if len(tempDB) > 63 {
		tempDB = tempDB[len(tempDB)-63:]
	}
	if _, err := ex.Exec(ctx, "CREATE DATABASE "+tempDB); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.Exec(ctx, "CREATE USER "+tempDB+" WITH PASSWORD'"+tempDB+"'"); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+tempDB+" TO "+tempDB); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.Exec(ctx, "ALTER DATABASE "+tempDB+" OWNER TO "+tempDB); err != nil {
		t.Fatal(err)
	}
	return tempDB
}

func randHex(t testing.TB, rawBytes int) string {
	b := make([]byte, rawBytes)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
