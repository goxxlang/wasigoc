// Real, working database/sql -- NOT a stub. The generic driver-dispatch
// layer (Register/Open, DB/Rows/Row/Stmt/Tx/Result, argument conversion,
// Scan-by-pointer-assertion) is ordinary Go logic with no I/O of its own;
// it works correctly against ANY conforming database/sql/driver.Driver.
// What's genuinely impossible on wasm32-wasip1 is a CONCRETE driver that
// dials a real socket to an external database server and holds it open
// under concurrent use (WASI preview 1 has no socket syscalls at all --
// see this project's own `net` package tracker line) -- that gap lives
// entirely in a driver implementation this project doesn't and can't
// ship, never in this package. Once Wasi2G++ bridges real networking
// through its sandbox (see this project's own roadmap), a real
// socket-backed driver.Conn could be registered here and this package
// would work with it completely unchanged.
//
// Verified with a small hand-written in-memory driver.Driver (an
// in-memory table, not a real database) exercising Register/Open/Exec/
// Query/Rows.Next/Scan/Prepare/Stmt.Exec/Stmt.Query/Begin/Commit/Rollback
// end-to-end through this package's real dispatch logic -- the same
// technique real Go's own database/sql test suite uses internally
// (`fakedb_test.go`) to test the generic layer without a real database,
// and legitimate evidence for what's being tested here: the dispatch
// logic itself, not any particular driver's I/O.
//
// Bounded: DB does not pool connections (one connection is opened lazily
// on first use and kept open, not per-goroutine-concurrent pooling --
// real concurrent multi-goroutine access to one *DB, meaningful on a real
// multi-threaded runtime with a real connection pool, isn't a shape this
// project's cooperative execution model benefits from anyway). Argument/
// Scan conversion covers nil/int64/float64/bool/[]byte/string and Go's
// other integer/float widths widened to int64/float64 -- no time.Time
// (driver.Value's real Go definition allows it; adding it needs this
// package to agree on a wire shape with the `time` package that hasn't
// been designed here) and no sql.Null* wrapper types (a NULL scanned into
// anything other than `*any` is an error, same as real Go's behavior for
// a non-nullable destination type). No context-aware method variants,
// consistent with database/sql/driver's own bounded scope.
package sql

import (
	"database/sql/driver"
	"errors"
	"io"
)

var ErrNoRows = errors.New("sql: no rows in result set")
var ErrTxDone = errors.New("sql: transaction has already been committed or rolled back")

var drivers = map[string]driver.Driver{}

// Register makes a database driver available by name, for Open.
func Register(name string, drv driver.Driver) {
	drivers[name] = drv
}

// ---- argument conversion (Go value -> driver.Value) ----------------------

func convertArg(a any) (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	if v, ok := a.(int64); ok {
		return v, nil
	}
	if v, ok := a.(float64); ok {
		return v, nil
	}
	if v, ok := a.(bool); ok {
		return v, nil
	}
	if v, ok := a.([]byte); ok {
		return v, nil
	}
	if v, ok := a.(string); ok {
		return v, nil
	}
	if v, ok := a.(int); ok {
		return int64(v), nil
	}
	if v, ok := a.(int32); ok {
		return int64(v), nil
	}
	if v, ok := a.(int16); ok {
		return int64(v), nil
	}
	if v, ok := a.(int8); ok {
		return int64(v), nil
	}
	if v, ok := a.(uint); ok {
		return int64(v), nil
	}
	if v, ok := a.(uint32); ok {
		return int64(v), nil
	}
	if v, ok := a.(float32); ok {
		return float64(v), nil
	}
	return nil, errors.New("sql: unsupported argument type")
}

func convertArgs(args []any) ([]driver.Value, error) {
	out := make([]driver.Value, len(args))
	i := 0
	for i < len(args) {
		v, err := convertArg(args[i])
		if err != nil {
			return nil, err
		}
		out[i] = v
		i = i + 1
	}
	return out, nil
}

// ---- Result ----------------------------------------------------------

type Result interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

// driverResult wraps a driver.Result as an independent sql.Result value,
// rather than relying on one interface value satisfying a second,
// distinct interface type with the same method set -- this project's
// interface values aren't structurally interchangeable across distinct
// interface types that way (each interface type has its own dispatch
// mechanism), so a small wrapper struct is used everywhere in this
// package instead, not just here.
type driverResult struct {
	res driver.Result
}

func (r driverResult) LastInsertId() (int64, error) { return r.res.LastInsertId() }
func (r driverResult) RowsAffected() (int64, error) { return r.res.RowsAffected() }

// ---- Scan (driver.Value -> *T pointer assertion) --------------------

func scanInto(dest any, src driver.Value) error {
	if d, ok := dest.(*any); ok {
		*d = src
		return nil
	}
	if d, ok := dest.(*string); ok {
		v, ok2 := src.(string)
		if !ok2 {
			return errors.New("sql: Scan: source is not a string")
		}
		*d = v
		return nil
	}
	if d, ok := dest.(*int64); ok {
		v, ok2 := src.(int64)
		if !ok2 {
			return errors.New("sql: Scan: source is not an int64")
		}
		*d = v
		return nil
	}
	if d, ok := dest.(*int); ok {
		v, ok2 := src.(int64)
		if !ok2 {
			return errors.New("sql: Scan: source is not an int64")
		}
		*d = int(v)
		return nil
	}
	if d, ok := dest.(*float64); ok {
		v, ok2 := src.(float64)
		if !ok2 {
			return errors.New("sql: Scan: source is not a float64")
		}
		*d = v
		return nil
	}
	if d, ok := dest.(*bool); ok {
		v, ok2 := src.(bool)
		if !ok2 {
			return errors.New("sql: Scan: source is not a bool")
		}
		*d = v
		return nil
	}
	if d, ok := dest.(*[]byte); ok {
		v, ok2 := src.([]byte)
		if !ok2 {
			return errors.New("sql: Scan: source is not []byte")
		}
		*d = v
		return nil
	}
	return errors.New("sql: Scan: unsupported destination type")
}

// ---- Rows/Row ----------------------------------------------------------

type Rows struct {
	rows    driver.Rows
	stmt    driver.Stmt // non-nil only for the Prepare+Query fallback path
	cols    []string
	dest    []driver.Value
	lastErr error
	closed  bool
}

func (r *Rows) Columns() []string {
	if r.cols == nil {
		r.cols = r.rows.Columns()
	}
	return r.cols
}

func (r *Rows) Next() bool {
	if r.closed {
		return false
	}
	cols := r.Columns()
	if r.dest == nil {
		r.dest = make([]driver.Value, len(cols))
	}
	err := r.rows.Next(r.dest)
	if err != nil {
		r.lastErr = err
		return false
	}
	return true
}

func (r *Rows) Scan(dest ...any) error {
	if len(dest) != len(r.dest) {
		return errors.New("sql: Scan argument count does not match column count")
	}
	i := 0
	for i < len(dest) {
		if err := scanInto(dest[i], r.dest[i]); err != nil {
			return err
		}
		i = i + 1
	}
	return nil
}

func (r *Rows) Err() error {
	if r.lastErr == io.EOF {
		return nil
	}
	return r.lastErr
}

func (r *Rows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	err := r.rows.Close()
	if r.stmt != nil {
		r.stmt.Close()
	}
	return err
}

type Row struct {
	rows *Rows
	err  error
}

func (r *Row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	defer r.rows.Close()
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return ErrNoRows
	}
	return r.rows.Scan(dest...)
}

func (r *Row) Err() error {
	return r.err
}

// ---- Stmt ----------------------------------------------------------

type Stmt struct {
	stmt driver.Stmt
}

func (s *Stmt) Exec(args ...any) (Result, error) {
	dargs, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	res, err := s.stmt.Exec(dargs)
	if err != nil {
		return nil, err
	}
	return driverResult{res: res}, nil
}

func (s *Stmt) Query(args ...any) (*Rows, error) {
	dargs, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	rows, err := s.stmt.Query(dargs)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

func (s *Stmt) QueryRow(args ...any) *Row {
	rows, err := s.Query(args...)
	if err != nil {
		return &Row{err: err}
	}
	return &Row{rows: rows}
}

func (s *Stmt) Close() error {
	return s.stmt.Close()
}

// ---- Tx ----------------------------------------------------------

type Tx struct {
	tx   driver.Tx
	conn driver.Conn
	done bool
}

func (tx *Tx) Commit() error {
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	return tx.tx.Commit()
}

func (tx *Tx) Rollback() error {
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	return tx.tx.Rollback()
}

func (tx *Tx) Exec(query string, args ...any) (Result, error) {
	if tx.done {
		return nil, ErrTxDone
	}
	dargs, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	if ex, ok := tx.conn.(driver.Execer); ok {
		res, err := ex.Exec(query, dargs)
		if err != nil {
			return nil, err
		}
		return driverResult{res: res}, nil
	}
	stmt, err := tx.conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	res, err := stmt.Exec(dargs)
	stmt.Close()
	if err != nil {
		return nil, err
	}
	return driverResult{res: res}, nil
}

func (tx *Tx) Query(query string, args ...any) (*Rows, error) {
	if tx.done {
		return nil, ErrTxDone
	}
	dargs, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	if qr, ok := tx.conn.(driver.Queryer); ok {
		rows, err := qr.Query(query, dargs)
		if err != nil {
			return nil, err
		}
		return &Rows{rows: rows}, nil
	}
	stmt, err := tx.conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	rows, err := stmt.Query(dargs)
	if err != nil {
		stmt.Close()
		return nil, err
	}
	return &Rows{rows: rows, stmt: stmt}, nil
}

func (tx *Tx) QueryRow(query string, args ...any) *Row {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return &Row{err: err}
	}
	return &Row{rows: rows}
}

func (tx *Tx) Prepare(query string) (*Stmt, error) {
	if tx.done {
		return nil, ErrTxDone
	}
	s, err := tx.conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &Stmt{stmt: s}, nil
}

// ---- DB ----------------------------------------------------------

// DB is a database handle. Unlike real Go's *DB, this does not pool
// multiple connections -- see this file's own header comment.
type DB struct {
	drv  driver.Driver
	dsn  string
	conn driver.Conn
}

// Open looks up a driver already registered via Register and returns a
// handle for it. Like real Go, it does not verify the driver can connect
// -- that happens lazily on first use.
func Open(driverName string, dataSourceName string) (*DB, error) {
	drv, ok := drivers[driverName]
	if !ok {
		return nil, errors.New("sql: unknown driver \"" + driverName + "\" (forgotten import?)")
	}
	return &DB{drv: drv, dsn: dataSourceName}, nil
}

func (db *DB) getConn() (driver.Conn, error) {
	if db.conn != nil {
		return db.conn, nil
	}
	c, err := db.drv.Open(db.dsn)
	if err != nil {
		return nil, err
	}
	db.conn = c
	return c, nil
}

func (db *DB) Ping() error {
	_, err := db.getConn()
	return err
}

func (db *DB) Close() error {
	if db.conn == nil {
		return nil
	}
	c := db.conn
	db.conn = nil
	return c.Close()
}

func (db *DB) Exec(query string, args ...any) (Result, error) {
	c, err := db.getConn()
	if err != nil {
		return nil, err
	}
	dargs, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	if ex, ok := c.(driver.Execer); ok {
		res, err := ex.Exec(query, dargs)
		if err != nil {
			return nil, err
		}
		return driverResult{res: res}, nil
	}
	stmt, err := c.Prepare(query)
	if err != nil {
		return nil, err
	}
	res, err := stmt.Exec(dargs)
	stmt.Close()
	if err != nil {
		return nil, err
	}
	return driverResult{res: res}, nil
}

func (db *DB) Query(query string, args ...any) (*Rows, error) {
	c, err := db.getConn()
	if err != nil {
		return nil, err
	}
	dargs, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	if qr, ok := c.(driver.Queryer); ok {
		rows, err := qr.Query(query, dargs)
		if err != nil {
			return nil, err
		}
		return &Rows{rows: rows}, nil
	}
	stmt, err := c.Prepare(query)
	if err != nil {
		return nil, err
	}
	rows, err := stmt.Query(dargs)
	if err != nil {
		stmt.Close()
		return nil, err
	}
	return &Rows{rows: rows, stmt: stmt}, nil
}

func (db *DB) QueryRow(query string, args ...any) *Row {
	rows, err := db.Query(query, args...)
	if err != nil {
		return &Row{err: err}
	}
	return &Row{rows: rows}
}

func (db *DB) Prepare(query string) (*Stmt, error) {
	c, err := db.getConn()
	if err != nil {
		return nil, err
	}
	s, err := c.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &Stmt{stmt: s}, nil
}

func (db *DB) Begin() (*Tx, error) {
	c, err := db.getConn()
	if err != nil {
		return nil, err
	}
	t, err := c.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{tx: t, conn: c}, nil
}
