// Command query is a simple tool for querying data from the mp3 database.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"github.com/jeffwilliams/wwwmp3/scan"
	_ "github.com/mattn/go-sqlite3"
	"os"
)

var artist = flag.String("artist", "", "Artist search criteria")
var album = flag.String("album", "", "Album search criteria")
var title = flag.String("title", "", "Title search criteria")
var dbflag = flag.String("db", "mp3db", "database containing mp3 info")
var field = flag.String("field", "", "Only output the specified field (album, artist, or title)")
var page = flag.Int("page", -1, "The page in results to return")
var pageSize = flag.Int("pageSize", 10, "Size of a page")

func output(db scan.Mp3Db, filt map[string]string) {
	quit := make(chan bool)
	ch := make(chan map[string]string)

	var paging *scan.Paging

	if *page >= 0 {
		paging = &scan.Paging{PageSize: *pageSize, Page: *page}
	}

	fields := []string{"artist", "album", "title", "path"}
	if len(*field) > 0 {
		fields = []string{*field}
	}

	go scan.FindMp3sInDb(
		db,
		fields,
		filt,
		[]string{"artist", "album", "title"},
		ch,
		paging,
		quit)

	for meta := range ch {
		if _, ok := meta["eof"]; ok {
			fmt.Println("<final page>")
			break
		} else {
			if len(*field) > 0 {
				fmt.Println(meta[*field])
			} else {
				fmt.Printf("    {artist: \"%s\", album: \"%s\", title: \"%s\", path: \"%s\"},\n",
					meta["artist"], meta["album"], meta["title"], meta["path"])
			}
		}
	}
}

/*
func outputField(db scan.Mp3Db, c scan.Criteria, field scan.DbField) {
  quit := make(chan bool)
  ch := make(chan string)

  go scan.FindMp3FieldInDb(db, c, field, ch, quit)

  i := 0
  for s := range(ch) {
    fmt.Println(s)
    i++
    if *max >= 0 && i >= *max {
      quit <- true
      // On the next iteration the channel will be closed so the loop will exit
    }
  }
}
*/
func main() {
	flag.Parse()

	/*
	   var dbField scan.DbField = -1
	   if len(*field) > 0 {
	     var err error
	     dbField, err = scan.GetDbField(*field)
	     if err != nil {
	       fmt.Println("Unknown field type",*field)
	       os.Exit(1)
	     }
	   }
	*/

	c := map[string]string{
		"artist": *artist,
		"album":  *album,
		"title":  *title,
	}

	db, err := sql.Open("sqlite3", *dbflag)
	if err != nil {
		return
	}

	mp3db, err := scan.OpenMp3Db(db)
	if err != nil {
		fmt.Println("Error opening database:", err)
		os.Exit(1)
	}
	defer mp3db.Close()

	output(mp3db, c)

	/*
	   quit := make(chan bool)
	   ch := make(chan scan.Metadata)

	   go scan.FindMp3sInDb(db, c, ch, quit)

	   i := 0
	   for meta := range(ch) {
	     fmt.Printf("    {artist: \"%s\", album: \"%s\", title: \"%s\", path: \"%s\"},\n",
	       meta.Artist, meta.Album, meta.Title, meta.Path)
	     i++
	     if *max >= 0 && i >= *max {
	       quit <- true
	       // On the next iteration the channel will be closed so the loop will exit
	     }
	   }
	*/
}
