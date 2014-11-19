package scan

import (
	"bytes"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

type Mp3Db struct {
	DB *sql.DB

	stmtAddMp3 *sql.Stmt

	stmtGetMp3sOrderAlbum *sql.Stmt

	stmtCleaners []func()
}

func (m *Mp3Db) prepare() (err error) {
	m.stmtAddMp3, err = m.DB.Prepare("insert into mp3(artist, album, title, path) values(?,?,?,?)")
	if err != nil {
		return
	}
	m.stmtCleaners = append(m.stmtCleaners, func() { m.stmtAddMp3.Close() })

	return
}

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

func (m Mp3Db) Close() {
	for _, f := range m.stmtCleaners {
		f()
	}

	m.DB.Close()
}

/* Create all the tables in the database */
func CreateMp3Db(db *sql.DB) (r Mp3Db, err error) {
	r = Mp3Db{
		DB:           db,
		stmtCleaners: make([]func(), 0),
	}

	sql := `create table mp3(path text not null primary key, artist text, album text, title text);`
	_, err = r.DB.Exec(sql)
	if err != nil {
		return
	}

	err = r.prepare()
	return
}

/*
 */
func ScanMp3sToDb(basedir string, db Mp3Db, prog *chan int) {
	if prog != nil {
		defer close((*prog))
	}

	c := make(chan Metadata)

	go ScanMp3s(basedir, c)

	i := 0
	for m := range c {

		//fmt.Println("Adding", m);

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Println("ScanMp3sToDb: creating transaction failed:", err)
		}
		stmt := tx.Stmt(db.stmtAddMp3)
		_, err = stmt.Exec(m.Artist, m.Album, m.Title, m.Path)
		if err != nil {
			fmt.Println("ScanMp3sToDb: inserting failed:", err)
		}
		err = tx.Commit()
		if err != nil {
			fmt.Println("ScanMp3sToDb: commit failed:", err)
		}

		i++
		if prog != nil && i%100 == 0 {
			(*prog) <- i
		}
	}
}

type Criteria struct {
	Title  string
	Artist string
	Album  string
}

func (c Criteria) Empty() bool {
	return len(c.Title) == 0 && len(c.Artist) == 0 && len(c.Album) == 0
}

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

func FindMp3sInDb(db Mp3Db, fields []string, filt map[string]string, order []string, ch chan map[string]string, p *Paging, quit chan bool) {
	defer close(ch)

	var sql bytes.Buffer

	if len(fields) == 0 {
		fields = []string{"artist", "album", "title", "path"}
	}

	sql.WriteString("select distinct ")
	sql.WriteString(makelist(fields, ", "))
	sql.WriteString(" from mp3 ")

	clauses := make([]string, 0)
	var where bytes.Buffer
	for k, v := range filt {
		if len(v) > 0 {
			where.WriteString("instr(lower(")
			where.WriteString(k)
			where.WriteString("),'")
			where.WriteString(strings.ToLower(v))
			where.WriteString("') = 1")
			clauses = append(clauses, where.String())
			where.Reset()
		}
	}

	if len(clauses) != 0 {
		sql.WriteString("where ")
		sql.WriteString(makelist(clauses, " and "))
	}

	if len(order) == 0 {
		order = []string{"artist", "album", "title", "path"}
	}

	sql.WriteString(" order by ")
	for i, _ := range order {
		var buf bytes.Buffer
		buf.WriteString("lower(")
		buf.WriteString(order[i])
		buf.WriteString(")")
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

	fmt.Println("scan.FindMp3sInDb: executing SQL " + sql.String())

	rows, err := db.DB.Query(sql.String())
	if err != nil {
		fmt.Println("scan.FindMp3sInDb: query failed:", err)
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

		if err := rows.Scan(fieldValPtrs...); err != nil {
			fmt.Println("scan.FindMp3sInDb: db read failed:", err)
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

	if err := rows.Err(); err != nil {
		fmt.Println("scan.FindMp3sInDb: query error:", err)
	}

	return
}
