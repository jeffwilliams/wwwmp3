// Command scan is used to update an mp3 database with metainformation found under a directory tree.
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"github.com/jeffwilliams/wwwmp3/play"
	"github.com/jeffwilliams/wwwmp3/scan"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var dbflag = flag.String("db", "", "If set, store data in the mentioned database")
var dump = flag.Bool("dump", false, "If set, print out all id3 information contained in the files")

func openOrCreateDb(name string) (mp3db scan.Mp3Db, err error) {
	_, err = os.Stat(name)

	create := false
	if err != nil {
		if os.IsNotExist(err) {
			create = true
		} else {
			return
		}
	}

	db, err := sql.Open("sqlite3", name)
	if err != nil {
		return
	}

	if create {
		fmt.Println("Creating database")
		mp3db, err = scan.CreateMp3Db(db)
		if err != nil {
			return
		}
	} else {
		mp3db, err = scan.OpenMp3Db(db)
		if err != nil {
			return
		}
	}

	return
}

// ttyCols gets the number of columns in the terminal. This is horribly non-portable.
func ttyCols() (int, error) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return -1, err
	}
	parts := strings.Split(string(out), " ")
	if len(parts) <= 1 {
		return -1, errors.New("stty output has too few fields")
	}
	v, err := strconv.Atoi(strings.Trim(parts[1], " \n"))
	if err != nil {
		return -1, err
	}
	return v, nil
}

type ScanResult struct {
	Meta *scan.Metadata
	Err  error
}

func printProg(c chan ScanResult) {
	lastLen := 0
	cols, colerr := ttyCols()
	cols -= 1

	for r := range c {
		fmt.Printf("\r")
		for i := 0; i < lastLen; i++ {
			fmt.Printf(" ")
		}
		fmt.Printf("\r")
		msg := r.Meta.Path
		if r.Err != nil {
			msg = r.Err.Error() + ":" + msg
		}
		runes := []rune(msg)
		lastLen = len(runes)
		if colerr == nil && len(runes) > cols {
			// Take first `cols` characters
			msg = string(runes[:cols])
			lastLen = cols
		}
		fmt.Printf(msg)
		os.Stdout.Sync()
	}
}

func printProgNonblocking() func(m *scan.Metadata, err error) {

	c := make(chan ScanResult)

	go printProg(c)

	return func(m *scan.Metadata, err error) {
		r := ScanResult{m, err}
		if err != nil {
			// Don't drop errors
			c <- r
		} else {
			select {
			case c <- r:
			default:
				// Printing still in progress. Drop this message
			}
		}
	}
}

func main() {
	flag.Parse()

	var db scan.Mp3Db
	usedb := false

	if dbflag != nil && len(*dbflag) != 0 {
		var err error
		usedb = true
		fmt.Println("Will output to database", *dbflag)

		db, err = openOrCreateDb(*dbflag)

		if err != nil {
			fmt.Println("Error opening database:", err)
			os.Exit(1)
		}

		defer db.Close()
	}

	if len(flag.Args()) < 1 {
		fmt.Println("Pass the directory to scan")
		os.Exit(1)
	}

	if !usedb {
		c := make(chan scan.Metadata)
		go scan.ScanMp3s(flag.Arg(0), c)

		for meta := range c {
			if *dump {
				fmt.Println("====", meta.Path)
				play.DebugMetadata(meta.Path)
				fmt.Println("")
			} else {
				fmt.Printf("    {artist: \"%s\", album: \"%s\", title: \"%s\", path: \"%s\"},\n",
					meta.Artist, meta.Album, meta.Title, meta.Path)
			}
		}
	} else {

		printer := printProgNonblocking()

		scan.ScanMp3sToDb(flag.Arg(0), db, printer)
	}
	fmt.Printf("\r")
	fmt.Printf("\n")

}
