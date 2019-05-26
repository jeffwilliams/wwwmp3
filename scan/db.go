package scan

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Mp3Db abstracts a sqlite3 database containing mp3 metainformation.
type Mp3Db struct {
	DB *sql.DB

	stmtAddMp3 *sql.Stmt

	stmtUpdateMp3 *sql.Stmt

	stmtPathExists *sql.Stmt

	stmtGetMp3sOrderAlbum *sql.Stmt

	stmtCleaners []func()
}

func (m *Mp3Db) prepare() (err error) {
	m.stmtAddMp3, err = m.DB.Prepare("insert into mp3(artist, album, title, tracknum, path) values(?,?,?,?,?)")
	if err != nil {
		return
	}
	m.stmtCleaners = append(m.stmtCleaners, func() { m.stmtAddMp3.Close() })

	m.stmtUpdateMp3, err = m.DB.Prepare("update mp3 set artist = ?, album = ?, title = ?, tracknum = ? where path = ?")
	if err != nil {
		return
	}
	m.stmtCleaners = append(m.stmtCleaners, func() { m.stmtUpdateMp3.Close() })

	m.stmtPathExists, err = m.DB.Prepare("select count(*) from mp3 where path = ?")
	if err != nil {
		return
	}
	m.stmtCleaners = append(m.stmtCleaners, func() { m.stmtPathExists.Close() })

	return
}

// Open an existing database and return the open Mp3Db struct. Expects db to be set to a valid, opened sql.DB.
func OpenMp3Db(db *sql.DB) (r Mp3Db, err error) {
	r = Mp3Db{
		DB:           db,
		stmtCleaners: make([]func(), 0),
	}

	err = r.prepare()
	if err != nil {
		return
	}

	return
}

// Close the database.
func (m Mp3Db) Close() {
	for _, f := range m.stmtCleaners {
		f()
	}

	m.DB.Close()
}

// Create the database schema and return the open Mp3Db struct. Expects db to be set to a valid, opened sql.DB.
func CreateMp3Db(db *sql.DB) (r Mp3Db, err error) {
	r = Mp3Db{
		DB:           db,
		stmtCleaners: make([]func(), 0),
	}

	sql := `create table mp3(path text not null primary key, artist text, album text, title text, tracknum int);`
	_, err = r.DB.Exec(sql)
	if err != nil {
		return
	}

	err = r.prepare()
	return
}

// ScanMp3sToDb scans a directory tree for mp3 files and updates `db` with the new mp3 information found.
// If `callback` is not nil, it is called each metadata.
func ScanMp3sToDb(basedir string, db Mp3Db, callback func(m *Metadata, err error)) (succCnt, errCnt int) {
	c := make(chan Metadata)

	go ScanMp3s(basedir, c)

	doCallback := func(m *Metadata, err error) {
		if callback != nil {
			callback(m, err)
		}
	}

	for m := range c {

		var cnt int
		err := db.stmtPathExists.QueryRow(m.Path).Scan(&cnt)

		if err != nil {
			doCallback(&m, fmt.Errorf("ScanMp3sToDb: querying for existence failed: %v", err))
			errCnt++
			continue
		}

		tx, err := db.DB.Begin()
		// Don't call the callback since we are not returning, and we want the callback called
		// once per metadata.
		if err != nil {
			//doCallback(&m, fmt.Errorf("ScanMp3sToDb: creating transaction failed: %v", err))
			errCnt++
			continue
		}

		var stmt *sql.Stmt
		if cnt == 0 {
			// No rows with that path.
			stmt = tx.Stmt(db.stmtAddMp3)
		} else {
			// Row exists.
			stmt = tx.Stmt(db.stmtUpdateMp3)
		}

		_, err = stmt.Exec(m.Artist, m.Album, m.Title, m.Tracknum, m.Path)
		if err != nil {
			doCallback(&m, fmt.Errorf("ScanMp3sToDb: inserting or updating failed: %v\n", err))
			errCnt++
			continue
		}
		err = tx.Commit()
		// Don't call the callback since we are not returning, and we want the callback called
		// once per metadata.
		if err != nil {
			//	doCallback(&m, fmt.Errorf("ScanMp3sToDb: commit failed: %v", err))
			errCnt++
		}

		succCnt++
		if callback != nil {
			doCallback(&m, nil)
		}
	}

	return
}

// Paging describes what page of data to return.
type Paging struct {
	// Number of items in a page
	PageSize int
	// The page requested (0 based)
	Page int
}

func makelist(a []string, sep string) string {
	var buf bytes.Buffer
	for i, s := range a {
		if i > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(s)
	}
	return buf.String()
}

// Escape the ticks (') in the string
func escape(s string) string {
	return strings.Replace(s, "'", "''", -1)
}

// Database fields that are textual and not numeric.
var stringFields map[string]bool = map[string]bool{"artist": true, "album": true, "title": true, "path": true}

// FindMp3sInDb passes mp3 metainformation to channel `ch` for all mp3s matching the specified criteria.
// `fields` should be a list of field names to return; allowed fields are "artist", "album", "title", "tracknum", "path". If fields is nil, all fields are returned.
// `filt` should be a simple filter whos keys are fieldnames, and values are substrings of that field to match against. If filt is nil, no filter is applied.
// `order` should be a list of field names to order ascending by, or nil for no ordering.
// `p` describes what page of data to return; PageSize rows are returned, starting at row Page*PageSize.
// Results are written to `ch` as maps where the keys are fieldnames and values are field values.
func FindMp3sInDb(db Mp3Db, fields []string, filt map[string]string, order []string, ch chan map[string]string, p *Paging, errWriter io.Writer) {
	defer close(ch)

	var sql bytes.Buffer

	if fields == nil || len(fields) == 0 {
		fields = []string{"artist", "album", "title", "tracknum", "path"}
	}

	sql.WriteString("select distinct ")
	sql.WriteString(makelist(fields, ", "))
	sql.WriteString(" from mp3 ")

	clauses := make([]string, 0)
	var where bytes.Buffer
	if filt != nil {
		for k, v := range filt {
			if len(v) > 0 {
				where.WriteString("instr(lower(")
				where.WriteString(k)
				where.WriteString("),'")
				where.WriteString(strings.ToLower(escape(v)))
				// Initial substring match:
				//where.WriteString("') = 1")
				// Anywhere match:
				where.WriteString("') > 0")
				clauses = append(clauses, where.String())
				where.Reset()
			}
		}
	}

	if len(clauses) != 0 {
		sql.WriteString("where ")
		sql.WriteString(makelist(clauses, " and "))
	}

	if order == nil || len(order) == 0 {
		order = []string{"artist", "album", "tracknum", "title", "path"}
	}

	sql.WriteString(" order by ")
	for i, _ := range order {
		var buf bytes.Buffer

		// Only use the lower() function on string fields
		if _, ok := stringFields[order[i]]; ok {
			buf.WriteString("lower(")
		}
		buf.WriteString(order[i])
		if _, ok := stringFields[order[i]]; ok {
			buf.WriteString(")")
		}
		order[i] = buf.String()
	}
	sql.WriteString(makelist(order, ", "))

	if p != nil {
		sql.WriteString(" limit ")
		// Read one more row than the requested size.
		// If we end up reading an actual PageSize+1 rows of data, then
		// we know we are not at EOF, otherwise we are.
		sql.WriteString(strconv.Itoa(p.PageSize + 1))
		sql.WriteString(" offset ")
		sql.WriteString(strconv.Itoa(p.Page * p.PageSize))
	}

	fmt.Println("Query: ", sql.String())

	rows, err := db.DB.Query(sql.String())
	if err != nil && errWriter != nil {
		fmt.Fprintln(errWriter, "scan.FindMp3sInDb: query failed:", err)
		return
	}
	defer rows.Close()

	fieldVals := make([]string, len(fields))
	fieldValPtrs := make([]interface{}, len(fields))
	for i, _ := range fieldVals {
		fieldValPtrs[i] = &fieldVals[i]
	}

	i := 0
	eof := true
	for rows.Next() {
		i++
		if i > p.PageSize {
			// There is still more data after this page. We are not at eof.
			eof = false
			break
		}

		if err := rows.Scan(fieldValPtrs...); err != nil && errWriter != nil {
			fmt.Fprintln(errWriter, "scan.FindMp3sInDb: db read failed:", err)
		} else {
			m := make(map[string]string)

			for i, v := range fields {
				m[v] = fieldVals[i]
			}

			ch <- m
		}
	}

	if eof {
		ch <- map[string]string{"eof": "eof"}
	}

	if err := rows.Err(); err != nil && errWriter != nil {
		fmt.Fprintln(errWriter, "scan.FindMp3sInDb: query error:", err)
	}

	return
}
