// Bounded database/sql/driver: the CORE driver SPI real Go's own package
// defines -- `Driver`/`Conn`/`Stmt`/`Rows`/`Tx`/`Result`, plus the two
// optional fast-path interfaces (`Execer`/`Queryer`) that let a `Conn`
// skip the Prepare+Exec/Query+Close round trip. Deliberately NOT included:
// every `context.Context`-aware variant (`DriverContext`/`Connector`/
// `ExecerContext`/`QueryerContext`/`ConnPrepareContext`/`ConnBeginTx`/
// `StmtExecContext`/`StmtQueryContext`) -- this project's own `context`
// package is a concrete, cooperative struct with no real cancellation/
// deadline machinery (see its own tracker line), so there is nothing a
// context-aware driver method would actually DO differently here that
// the plain synchronous form doesn't already do. Also not included:
// `SessionResetter`/`Validator`/`NamedValueChecker`/`ColumnConverter`/
// `RowsNextResultSet`/the `RowsColumnType*` introspection interfaces --
// all optional, all connection-pool or advanced-type-system refinements
// with no bearing on whether a driver works at all.
//
// This package is real, working Go -- not a stub. What's actually
// impossible on wasm32-wasip1 is any CONCRETE driver.Conn that dials a
// real socket to an external database server (WASI preview 1 has no
// socket syscalls at all, see this project's own `net` package tracker
// line): the interfaces here don't do that themselves, so this package
// works correctly with any driver.Conn -- an in-memory one included, and
// that's exactly how it's verified (see database/sql's own tracker line).
package driver

import "errors"

// Value is a value a driver must be able to handle: nil, int64, float64,
// bool, []byte, or string. Real Go's Value also allows time.Time and a
// driver-specific NamedValueChecker-negotiated type; both are out of
// scope here (no context-aware NamedValueChecker, see above).
type Value any

// Driver is the interface a database driver must implement.
type Driver interface {
	// Open returns a new connection to the database. name is in a
	// driver-specific format.
	Open(name string) (Conn, error)
}

// Conn is a connection to a database. It is not used concurrently by
// multiple goroutines.
type Conn interface {
	Prepare(query string) (Stmt, error)
	Close() error
	Begin() (Tx, error)
}

// Execer is an optional interface a Conn may implement to skip the
// Prepare+Exec+Close round trip for a query with no rows to return.
type Execer interface {
	Exec(query string, args []Value) (Result, error)
}

// Queryer is an optional interface a Conn may implement to skip the
// Prepare+Query round trip for a query that returns rows.
type Queryer interface {
	Query(query string, args []Value) (Rows, error)
}

// Result is the result of a query execution.
type Result interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

// Stmt is a prepared statement, bound to a Conn, not used concurrently.
type Stmt interface {
	Close() error
	// NumInput returns the number of placeholder parameters, or -1 if
	// the driver doesn't know it (in which case database/sql won't sanity
	// check argument counts before calling Exec/Query).
	NumInput() int
	Exec(args []Value) (Result, error)
	Query(args []Value) (Rows, error)
}

// Rows is an iterator over an executed query's results.
type Rows interface {
	// Columns returns the column names. The result's column count is
	// inferred from the length of this slice.
	Columns() []string
	Close() error
	// Next populates dest (the same length as Columns()) with the next
	// row's values, or returns io.EOF when there are no more rows.
	Next(dest []Value) error
}

// Tx is a transaction.
type Tx interface {
	Commit() error
	Rollback() error
}

var ErrSkip = errors.New("driver: skip fast-path; continue as if unimplemented")

var ErrBadConn = errors.New("driver: bad connection")

// RowsAffected implements Result for a driver whose query only affects
// rows (an INSERT/UPDATE) and has no auto-generated ID to report. A
// struct wrapping the count, not real Go's own `type RowsAffected
// int64` -- this project's codegen only supports methods on struct
// receivers, not a defined non-struct type (see this package's own
// header comment for the same constraint on `Value`, which stays a
// plain `any` for exactly this reason: it never needed a method). Also
// renamed from real Go's own `RowsAffected` to `RowsAffectedResult` --
// a struct with a method of the EXACT SAME NAME as the struct itself
// (real Go's `RowsAffected.RowsAffected()`) is valid Go but unrepresentable
// in C++, where a member function named identically to its enclosing
// class is always parsed as a constructor attempt; same "rename the Go
// type, don't teach the compiler to mangle" precedent as image/color's
// `Rgba` (renamed from `RGBA`) and hash/fnv's `Digest32`/`Digest64`.
type RowsAffectedResult struct {
	N int64
}

func (v RowsAffectedResult) LastInsertId() (int64, error) {
	return 0, errors.New("driver: LastInsertId is not supported by this driver")
}

func (v RowsAffectedResult) RowsAffected() (int64, error) {
	return v.N, nil
}
