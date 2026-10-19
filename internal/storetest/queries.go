package storetest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync/atomic"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/store"
	"modernc.org/sqlite"
)

// Queries counts the statements stores send to SQLite: every query, exec,
// prepare and begin, whichever pool runs it.
type Queries struct{ sent atomic.Int64 }

func (q *Queries) Sent() int64 { return q.sent.Load() }
func (q *Queries) Reset()      { q.sent.Store(0) }

var drivers atomic.Int64 // names each counting driver; database/sql can't unregister one

// CountQueries makes the stores the test opens from here on count their
// statements into the Queries it returns.
func CountQueries(t testing.TB) *Queries {
	t.Helper()
	queries := &Queries{}
	name := fmt.Sprintf("sqlite-counted-%d", drivers.Add(1))
	sql.Register(name, countingDriver{queries})
	previous := store.Driver
	store.Driver = name
	t.Cleanup(func() { store.Driver = previous })
	return queries
}

type countingDriver struct{ queries *Queries }

// sqliteConn is what modernc's connection does that database/sql uses.
type sqliteConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.SessionResetter
	driver.Validator
}

func (d countingDriver) Open(name string) (driver.Conn, error) {
	c, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return countingConn{c.(sqliteConn), d.queries}, nil
}

type countingConn struct {
	sqliteConn
	queries *Queries
}

func (c countingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.queries.sent.Add(1)
	return c.sqliteConn.BeginTx(ctx, opts)
}

func (c countingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.queries.sent.Add(1)
	return c.sqliteConn.PrepareContext(ctx, query)
}

func (c countingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.queries.sent.Add(1)
	return c.sqliteConn.ExecContext(ctx, query, args)
}

func (c countingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.queries.sent.Add(1)
	return c.sqliteConn.QueryContext(ctx, query, args)
}
