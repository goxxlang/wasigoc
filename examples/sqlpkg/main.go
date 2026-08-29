package main

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
)

type fakeTable struct {
	rows [][]driver.Value
}

type fakeDriver struct{ table *fakeTable }

func (d *fakeDriver) Open(name string) (driver.Conn, error) {
	return &fakeConn{table: d.table}, nil
}

type fakeConn struct{ table *fakeTable }

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{table: c.table, query: query}, nil
}
func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) {
	return &fakeTx{}, nil
}

// fakeFastConn implements driver.Execer/driver.Queryer directly, so
// sql.DB routes Exec/Query straight to it instead of falling back to
// Prepare+Exec/Query+Close -- exercises the OTHER dispatch branch
// fakeConn (above) doesn't.
type fakeFastConn struct{ table *fakeTable }

func (c *fakeFastConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{table: c.table, query: query}, nil
}
func (c *fakeFastConn) Close() error { return nil }
func (c *fakeFastConn) Begin() (driver.Tx, error) {
	return &fakeTx{}, nil
}
func (c *fakeFastConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	s := &fakeStmt{table: c.table, query: query}
	return s.Exec(args)
}
func (c *fakeFastConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	s := &fakeStmt{table: c.table, query: query}
	return s.Query(args)
}

type fakeFastDriver struct{ table *fakeTable }

func (d *fakeFastDriver) Open(name string) (driver.Conn, error) {
	return &fakeFastConn{table: d.table}, nil
}

type fakeStmt struct {
	table *fakeTable
	query string
}

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }
func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.query == "INSERT" {
		id, _ := args[0].(int64)
		name, _ := args[1].(string)
		s.table.rows = append(s.table.rows, []driver.Value{id, name})
		return driver.RowsAffectedResult{N: 1}, nil
	}
	if s.query == "DELETE" {
		s.table.rows = nil
		return driver.RowsAffectedResult{N: 0}, nil
	}
	return nil, errors.New("fakedb: unsupported exec query")
}
func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.query == "SELECT" {
		return &fakeRows{table: s.table, pos: 0}, nil
	}
	return nil, errors.New("fakedb: unsupported query")
}

type fakeRows struct {
	table *fakeTable
	pos   int
}

func (r *fakeRows) Columns() []string { return []string{"id", "name"} }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.table.rows) {
		return io.EOF
	}
	row := r.table.rows[r.pos]
	dest[0] = row[0]
	dest[1] = row[1]
	r.pos = r.pos + 1
	return nil
}

type fakeTx struct{}

func (t *fakeTx) Commit() error   { return nil }
func (t *fakeTx) Rollback() error { return nil }

func main() {
	sql.Register("fakedb", &fakeDriver{table: &fakeTable{}})

	db, err := sql.Open("fakedb", "test")
	fmt.Println(err == nil)

	res, err := db.Exec("INSERT", int64(1), "alice")
	fmt.Println(err == nil)
	n, _ := res.RowsAffected()
	fmt.Println(n)

	_, err = db.Exec("INSERT", int64(2), "bob")
	fmt.Println(err == nil)

	rows, err := db.Query("SELECT")
	fmt.Println(err == nil)
	count := 0
	var lastID int64
	var lastName string
	for rows.Next() {
		var id int64
		var name string
		serr := rows.Scan(&id, &name)
		fmt.Println(serr == nil)
		count = count + 1
		lastID = id
		lastName = name
	}
	fmt.Println(count, lastID, lastName)
	fmt.Println(rows.Err() == nil)

	row := db.QueryRow("SELECT")
	var fid int64
	var fname string
	fmt.Println(row.Scan(&fid, &fname) == nil)
	fmt.Println(fid, fname)

	stmt, err := db.Prepare("INSERT")
	fmt.Println(err == nil)
	_, err = stmt.Exec(int64(3), "carol")
	fmt.Println(err == nil)
	stmt.Close()

	rows2, _ := db.Query("SELECT")
	count2 := 0
	for rows2.Next() {
		count2 = count2 + 1
	}
	fmt.Println(count2)

	tx, err := db.Begin()
	fmt.Println(err == nil)
	_, err = tx.Exec("INSERT", int64(4), "dave")
	fmt.Println(err == nil)
	fmt.Println(tx.Commit() == nil)
	fmt.Println(tx.Commit() == sql.ErrTxDone)

	rows3, _ := db.Query("SELECT")
	count3 := 0
	for rows3.Next() {
		count3 = count3 + 1
	}
	fmt.Println(count3)

	_, err = db.Exec("BOGUS")
	fmt.Println(err != nil)

	db.Exec("DELETE")
	emptyRow := db.QueryRow("SELECT")
	var eid int64
	fmt.Println(emptyRow.Scan(&eid) == sql.ErrNoRows)

	_, err = sql.Open("nonexistent-driver", "x")
	fmt.Println(err != nil)

	// Execer/Queryer fast-path Conn (bypasses Prepare+Exec/Query+Close).
	sql.Register("fakedb-fast", &fakeFastDriver{table: &fakeTable{}})
	fdb, ferr := sql.Open("fakedb-fast", "test")
	fmt.Println(ferr == nil)
	_, ferr = fdb.Exec("INSERT", int64(10), "eve")
	fmt.Println(ferr == nil)
	_, ferr = fdb.Exec("INSERT", int64(11), "frank")
	fmt.Println(ferr == nil)
	frows, ferr := fdb.Query("SELECT")
	fmt.Println(ferr == nil)
	fcount := 0
	for frows.Next() {
		var id int64
		var name string
		frows.Scan(&id, &name)
		fcount = fcount + 1
	}
	fmt.Println(fcount)
}
