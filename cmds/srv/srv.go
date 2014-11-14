package main

import (
  "os"
  "io"
  "strings"
  //"sort"
  "database/sql"
  _ "github.com/mattn/go-sqlite3"
  "net/http"
  "flag"
  "strconv"
  "fmt"
  "time"
  "github.com/jeffwilliams/wwwmp3/scan"
  "github.com/jeffwilliams/wwwmp3/play"
)

var dbflag = flag.String("db", "mp3.db", "database containing mp3 info")
var db scan.Mp3Db
var player play.Player = play.NewPlayer()

func queryVal(r *http.Request, key string) (rs string) {
  v,ok := r.URL.Query()[key]
  if ok {
    rs = v[0]
  }
  return
}


// Respond to requests for mp3 metadata: lists of artists, titles, song paths, etc.
func serveMeta(w http.ResponseWriter, r *http.Request) {
  timer := time.Now()

  // Parse a query value which is a comma separated list. If the value doesn't appear, return the empty list
  queryList := func(key string) (l []string) {
    if s := queryVal(r, key); len(s) > 0 {
      l = strings.Split(s,",")
    }
    return
  }

  writeMapAsJson := func(w io.Writer, m map[string]string) {
    w.Write([]byte("  {"))
    i := 0
    for k,v := range m {
      if i > 0 {
        w.Write([]byte(", "))
      }
      w.Write([]byte(`"` + k + `": "` + strings.Replace(v,`"`,`\"`,-1) + `"`))
      i++
    }
    w.Write([]byte("}"))
  }

  if r.Method == "GET" {
    // Query string Format:
    //  filter:
    //    artist=blah&album=blah&title=blah
    //  paging:
    //    page=0&pagesize=10
    //  fields (not present or the following:)
    //    fields=artist,album,...
    //  order (not present or the following:)
    //    order=artist,album,...

    // Calculate the filter
    filt := make(map[string]string)
    if v := queryVal(r, "artist"); len(v) > 0 {
      filt["artist"] = v
    }
    if v := queryVal(r, "album"); len(v) > 0 {
      filt["album"] = v
    }
    if v := queryVal(r, "title"); len(v) > 0 {
      filt["title"] = v
    }

    page, err := strconv.Atoi(queryVal(r, "page"))
    if err != nil {
      w.WriteHeader(400)
      w.Write([]byte("400 Bad Request: The 'page' parameter is missing or invalid."))
    }

    pageSize, err := strconv.Atoi(queryVal(r, "pagesize"))
    if err != nil {
      w.WriteHeader(400)
      w.Write([]byte("400 Bad Request: The 'pagesize' parameter is missing or invalid."))
    }

    fields := queryList("fields")

    order := queryList("order")

    quit := make(chan bool)
    ch := make(chan map[string]string)

    go scan.FindMp3sInDb(
      db,
      fields,
      filt,
      order,
      ch,
      &scan.Paging{PageSize: pageSize, Page: page},
      quit)

    w.Write([]byte("[\n"))
    j := 0
    for m := range ch {
      if j > 0 {
        w.Write([]byte(",\n"))
      }
      writeMapAsJson(w, m)
      j++
    }
    w.Write([]byte("\n]\n"))
  }

  d := time.Now().Sub(timer)
  fmt.Println("serveMeta completed in",d)
}

// Perform functions on the mp3 player like play and pause.
func servePlayer(w http.ResponseWriter, r *http.Request) {
  if r.Method == "GET" {
    // Query string Format:
    // Load an mp3:   
    //  load: <path>
    // Play:
    //  play: play
    if v := queryVal(r, "load"); len(v) > 0 {
      _, err := player.Load(v, nil)
      if err != nil {
        w.WriteHeader(500)
        w.Write([]byte(err.Error()))
      }
    } else if _,ok := r.URL.Query()["play"]; ok {
      player.Play()
    } else if _,ok := r.URL.Query()["pause"]; ok {
      player.Pause()
    } else if _,ok := r.URL.Query()["stop"]; ok {
      player.Stop()
    }
  }
}

func openDb(path string) (mp3db scan.Mp3Db, err error) {
  db, err := sql.Open("sqlite3", path)
  if err != nil {
    return
  }

  mp3db, err = scan.OpenMp3Db(db)
  if err != nil {
    return
  }

  return
}

func main(){
  var err error

  // Open database
  db, err = openDb(*dbflag)
  if err != nil {
    fmt.Println("Error opening database:", err)
    os.Exit(1)
  }
  defer db.Close()

  // Setup http server
  http.HandleFunc("/songmeta", serveMeta)
  http.HandleFunc("/player", servePlayer)
	http.Handle("/", http.FileServer(http.Dir("src/github.com/jeffwilliams/wwwmp3/www")))

  fmt.Println("Listening on http://localhost:2001/")

  err = http.ListenAndServe(":2001", nil)
  if err != nil {
    fmt.Println("ListenAndServe failed: ", err)
    os.Exit(1)
  }
}


