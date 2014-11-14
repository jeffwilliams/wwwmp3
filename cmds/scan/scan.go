package main

import (
  "database/sql"
  _ "github.com/mattn/go-sqlite3"
	"flag"
	"fmt"
	"os"
	"github.com/jeffwilliams/wwwmp3/scan"
)

var dbflag = flag.String("db", "", "If set, store data in the mentioned database")

func openOrCreateDb(name string) (mp3db scan.Mp3Db, err error){
  _, err = os.Stat(name)

  create := false
  if err != nil {
    if os.IsNotExist(err) {
      create  = true
    } else {
      return
    }
  }

  db, err := sql.Open("sqlite3",name)
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

func main() {
	flag.Parse()

	//var db *leveldb.DB = nil
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
    /*
		opts := leveldb.NewOptions()
		opts.SetCache(leveldb.NewLRUCache(2 << 10))
		opts.SetCreateIfMissing(true)
		db, err = leveldb.Open(*dbflag, opts)
		if err != nil {
			fmt.Println("Error opening database:", err)
			os.Exit(1)
		}
		defer db.Close()
    */
	}

	if len(flag.Args()) < 1 {
		fmt.Println("Pass the directory to scan")
		os.Exit(1)
	}

	/*
	  play.DebugMetadata(os.Args[1]);
	  os.Exit(1)
	*/

	if ! usedb {
	  c := make(chan scan.Metadata)
		go scan.ScanMp3s(flag.Arg(0), c)

		for meta := range c {
			fmt.Printf("    {artist: \"%s\", album: \"%s\", title: \"%s\", path: \"%s\"},\n",
        meta.Artist, meta.Album, meta.Title, meta.Path)
		}
	} else {
    c := make(chan int)
    //go scan.ScanMp3sToDb(flag.Arg(0), db, &c)
    go scan.ScanMp3sToDb(flag.Arg(0), db, &c)
    for p := range c {
      fmt.Println(p)
    }
  }

}
