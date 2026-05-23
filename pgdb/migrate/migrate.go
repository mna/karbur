// Package migrate implements a postgresql database migrations runner based on
// pgdb. It supports forward-only, multi-packages versioning with dependency
// resolving. In other words:
//
//   - it does not support rollbacks; to fix an issue, create a new migration
//     to apply on top of the existing ones.
//   - different packages can provide their migrations independently of others,
//     and new versions in one does not mess up versions in another.
//   - a package's migrations can declare that they need to be applied after
//     migrations in one or many other packages.
//
// https://twitter.com/DivineOps/status/1400568403124011008?s=20
package migrate

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"github.com/philopon/go-toposort"
)

const (
	// ErrCycle is the error returned when applying migrations where there is a
	// dependency cycle among the migration groups. Note that it may be wrapped,
	// use errors.Is to check.
	ErrCycle = errors.ConstError("cycle detected in migration groups")

	// ErrMissingGroup is the error returned when applying migrations where at
	// least one required group has not been registered. Note that it may be
	// wrapped, use errors.Is to check.
	ErrMissingGroup = errors.ConstError("migration group(s) not registered")

	// ErrGroupRegistered is the error returned when Register is called with a
	// group that is already registered. Note that it may be wrapped, use
	// errors.Is to check.
	ErrGroupRegistered = errors.ConstError("group already registered")
)

const (
	sqlAcquireLock = `
    SELECT pg_advisory_xact_lock($1)`

	sqlGroupLastVersion = `
    SELECT
      "version"
    FROM
      "migrate_versions"
    WHERE
      "group" = $1`

	sqlSetGroupVersion = `
    INSERT INTO
      "migrate_versions" (
        "group", "version"
      )
    VALUES
      ($1, $2)
    ON CONFLICT
      ("group")
    DO UPDATE SET
      "version" = excluded."version"`
)

//go:embed _embed
var migrations embed.FS

// Config allows configuration of the Migrator.
type Config struct {
	// AdvisoryLockID identifies the ID of the advisory lock to control exclusive
	// access to applying the migrations. It defaults to 0.
	AdvisoryLockID int
}

// New creates a new Migrator that will use btx to connect to the database
// and conf to configure its migrations.
func New(btx pgdb.BeginTxer, conf *Config) (*Migrator, error) {
	var copyConf Config
	if conf != nil {
		copyConf = *conf
	}
	return &Migrator{
		beginTxer: btx,
		config:    copyConf,
	}, nil
}

// Migrator implements the postgresql database migrations runner.  A migration
// run can be applied at any time, a postgres advisory lock is used to grant
// exclusive access. The lock ID can be configured by setting AdvisoryLockID,
// it defaults to 0. Only the Migrate method is safe to use concurrently.
type Migrator struct {
	beginTxer pgdb.BeginTxer
	config    Config

	execData  map[string]any
	groupMigs map[string]fs.FS
	groupDeps map[string]map[string]bool
}

// Register registers the migrations for the specified group.  Any dependency
// can be provided in the after argument which are then used to order the
// migrations. It returns an error if a group has already been registered.
//
// The migrations to apply are all the *.tpl files in the root directory of
// the migrations file-system. They are executed as Go's text/template,
// with a map of all registered groups' configurations as data, in addition
// to the Migrator's own configuration.
func (m *Migrator) Register(group string, conf any, migrations fs.FS, after ...string) error {
	if m.groupMigs == nil {
		m.groupMigs = make(map[string]fs.FS)
	}
	if m.groupDeps == nil {
		m.groupDeps = make(map[string]map[string]bool)
	}
	if m.execData == nil {
		m.execData = make(map[string]any)
	}

	if _, ok := m.groupMigs[group]; ok {
		return fmt.Errorf("migrate: group %s: %w", group, ErrGroupRegistered)
	}
	m.groupMigs[group] = migrations
	m.execData[group] = conf

	deps := m.groupDeps[group]
	if deps == nil {
		deps = make(map[string]bool, len(after))
	}
	for _, dep := range after {
		deps[dep] = true
	}
	m.groupDeps[group] = deps

	return nil
}

// Export writes all registered migrations to dir, creating a sub-directory for
// each group and files for each migration step.
func (m *Migrator) Export(dir string) error {
	// apply ordering just to check for any cycle or missing group
	if _, err := m.orderMigrationGroups(); err != nil {
		return err
	}

	groupStmts, groupNames, err := m.prepareGroupMigrations()
	if err != nil {
		return err
	}
	for group, stmts := range groupStmts {
		gdir := filepath.Join(dir, group)
		if err := os.MkdirAll(gdir, 0o777); err != nil {
			return err
		}
		for i, stmt := range stmts {
			file := groupNames[group][i]
			if err := os.WriteFile(filepath.Join(gdir, file), []byte(stmt), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// Migrate applies all transactions that have not been applied yet to the
// database. Migrations are applied inside a transaction, any error will
// rollback the partially applied migrations.
func (m *Migrator) Migrate(ctx context.Context) error {
	order, err := m.orderMigrationGroups()
	if err != nil {
		return err
	}

	rootFS, err := fs.Sub(migrations, "_embed")
	if err != nil {
		return err
	}

	groupStmts, groupNames, err := m.prepareGroupMigrations()
	if err != nil {
		return err
	}
	rootStmts, rootNames, err := m.prepareMigrationsFS(rootFS)
	if err != nil {
		return err
	}

	return pgdb.Tx(ctx, m.beginTxer, nil, func(ctx context.Context, tx pgdb.Txer) error {
		// acquire the exclusive lock
		if _, err := tx.Exec(ctx, sqlAcquireLock, m.config.AdvisoryLockID); err != nil {
			return err
		}

		// always apply the migrator's own migrations, which are idempotent
		if err := applyMigrations(ctx, tx, rootStmts, rootNames); err != nil {
			return err
		}

		// TODO: tracing, logging and metrics, via ctx or something with OpenTelemetry
		for _, group := range order {
			last, err := groupLastVersion(ctx, tx, group)
			if err != nil {
				return err
			}

			stmts := groupStmts[group]
			names := groupNames[group]
			if len(stmts) > last+1 {
				stmts = stmts[last+1:]
				names = names[last+1:]
				if err := applyMigrations(ctx, tx, stmts, names); err != nil {
					return err
				}
				if err := setGroupLastVersion(ctx, tx, group, len(stmts)-1); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (m *Migrator) orderMigrationGroups() ([]string, error) {
	graph := toposort.NewGraph(len(m.groupDeps))

	// must add all nodes first, before any edges
	for k := range m.groupDeps {
		graph.AddNode(k)
	}

	// then add all edges (dependencies, where start point is the dependency)
	allGroups := make(map[string]bool, len(m.groupDeps))
	for to, deps := range m.groupDeps {
		allGroups[to] = true
		for dep := range deps {
			allGroups[dep] = true
			graph.AddEdge(dep, to)
		}
	}

	order, ok := graph.Toposort()
	if !ok {
		return nil, fmt.Errorf("migrate: %w", ErrCycle)
	}

	// ensure all dependencies exist as registered migration groups
	var missing []string
	for group := range allGroups {
		if _, ok := m.groupDeps[group]; !ok {
			missing = append(missing, group)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("migrate: groups %v: %w", missing, ErrMissingGroup)
	}
	return order, nil
}

// prepares migrations FS for all groups.
func (m *Migrator) prepareGroupMigrations() (groupStmts map[string][]string, groupNames map[string][]string, err error) {
	groupStmts = make(map[string][]string, len(m.groupMigs))
	groupNames = make(map[string][]string, len(m.groupMigs))
	for group, fs := range m.groupMigs {
		stmts, names, err := m.prepareMigrationsFS(fs)
		if err != nil {
			return nil, nil, err
		}
		groupStmts[group] = stmts
		groupNames[group] = names
	}
	return groupStmts, groupNames, nil
}

// parses and executes the templates in the migrations file-system, returning the
// resulting SQL statements as strings, in the lexical order of the files.
func (m *Migrator) prepareMigrationsFS(migrations fs.FS) (stmts []string, names []string, err error) {
	root := template.New("")
	if _, err := root.ParseFS(migrations, "*.tpl"); err != nil {
		if strings.Contains(err.Error(), "pattern matches no files") {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// process in name order
	tpls := root.Templates()
	sort.Slice(tpls, func(i, j int) bool {
		l, r := tpls[i], tpls[j]
		return l.Name() < r.Name()
	})

	var buf bytes.Buffer
	stmts = make([]string, 0, len(tpls))
	names = make([]string, 0, len(tpls))
	for _, t := range tpls {
		buf.Reset()
		if err := t.Execute(&buf, m.execData); err != nil {
			return nil, nil, err
		}
		stmts = append(stmts, buf.String())
		names = append(names, t.Name())
	}
	return stmts, names, nil
}

func groupLastVersion(ctx context.Context, q pgdb.Queryer, group string) (int, error) {
	var v int
	if err := q.QueryOne(ctx, &v, sqlGroupLastVersion, group); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return -1, nil
		}
		return 0, err
	}
	return v, nil
}

func setGroupLastVersion(ctx context.Context, q pgdb.Queryer, group string, version int) error {
	_, err := q.Exec(ctx, sqlSetGroupVersion, group, version)
	return err
}

func applyMigrations(ctx context.Context, q pgdb.Queryer, migs, names []string) error {
	for i, mig := range migs {
		if _, err := q.Exec(ctx, mig); err != nil {
			return fmt.Errorf("migrate: %s: %w", names[i], err)
		}
	}
	return nil
}
