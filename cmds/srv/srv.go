// Command srv implements a webserver that implements an mp3 player and hosts a UI for it.
//
// This command expects an mp3 database to be set up and filled.
package main

import (
	//"code.google.com/p/go.net/websocket"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/jeffwilliams/go-logging"
	"github.com/jeffwilliams/wwwmp3/play"
	"github.com/jeffwilliams/wwwmp3/scan"
	"github.com/jeffwilliams/wwwmp3/tee"
	_ "github.com/mattn/go-sqlite3"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	scanEventPeriod = 1 * time.Second
)

var (
	helpFlag     = flag.Bool("help", false, "Print help")
	dbflag       = flag.String("db", "mp3.db", "database containing mp3 info")
	allVolFlag   = flag.Bool("allvol", false, "If set to true, changing the volume affects all ALSA cards, not just the default.")
	logfileFlag  = flag.String("log", "", "File to write log messages to. Defaults to stdout if not specified.")
	logLevelFlag = flag.String("loglevel", "DEBUG", "Minimum severity of log messages to write. One of DEBUG, INFO, NOTICE, WARNING, ERROR, or CRITICAL")
	db           scan.Mp3Db

	// The mp3 player
	player play.Player = play.NewPlayer()
	// The queue for the mp3 player.
	queue play.Queue
	// Recently played songs
	recent Recent

	// Metadata for the currently playing mp3
	meta     map[string]string
	metaLock sync.Mutex

	upgrader websocket.Upgrader

	// Player events are written to this Tee
	eventTee = tee.New()

	log *logging.Logger

	// If we are not logging to stdout, this is the file handle we are using.
	logFile *os.File

	// Directories to scan recursively to find mp3s.
	mp3Dirs []string

	// When a scan is performed, the metadata of mp3s scanned are written to this tee.
	// A nil is written to the channel when the current scan completes.
	scanTee = tee.New()

	// scanning: Are we currently scanning for mp3s?
	scanning bool = false
	// scanMutex protects `scanning`.
	scanMutex sync.Mutex
)

// queryVal returns the first value for the GET query variable with name `key`.
func queryVal(r *http.Request, key string) (rs string) {
	v, ok := r.URL.Query()[key]
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
			l = strings.Split(s, ",")
		}
		return
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

		ch := make(chan map[string]string)

		go scan.FindMp3sInDb(
			db,
			fields,
			filt,
			order,
			ch,
			&scan.Paging{PageSize: pageSize, Page: page},
			nil)

		enc := json.NewEncoder(w)
		w.Write([]byte("[\n"))
		j := 0
		for m := range ch {
			if j > 0 {
				w.Write([]byte(",\n"))
			}

			enc.Encode(m)
			j++
		}
		w.Write([]byte("\n]\n"))
	}

	d := time.Now().Sub(timer)
	log.Info("serveMeta completed in %v", d)
}

// findMp3ByPath returns the mp3 information for the mp3 with the specified path.
func findMp3ByPath(path string) map[string]string {
	ch := make(chan map[string]string)

	go scan.FindMp3sInDb(
		db,
		nil,
		map[string]string{"path": path},
		nil,
		ch,
		&scan.Paging{PageSize: 1, Page: 0},
		nil)

	r := <-ch

	// Drain channel
	for _ = range ch {
	}

	return r
}

func setMetainfo() {
	if meta == nil {
		meta = make(map[string]string)
	}

	// Set the current mp3 info into the metadata struct
	info := player.GetInfo()
	if info != nil {
		meta["bitrate"] = strconv.Itoa(info.BitRate)
		meta["rate"] = strconv.Itoa(info.Rate)
		meta["duration"] = strconv.FormatFloat(info.Duration, 'f', -1, 64)
		meta["sec_per_sample"] = strconv.FormatFloat(info.Sps, 'f', -1, 64)
	} else {
		log.Error("Getting loaded mp3 info (like bitrate) failed")
	}
}

// Give an list of paths to mp3 files, return a list of metadata maps
func pathsToMetadatas(list []string) (result []map[string]string) {
	result = make([]map[string]string, 0)

	for _, path := range list {
		m := findMp3ByPath(path)
		if m == nil {
			m = map[string]string{
				"artist": "?",
				"album":  "?",
				"title":  "?",
				"path":   path,
			}
		}
		result = append(result, m)
	}
	return
}

// listQueue returns a slice containing the metainfo of the tracks in the play queue.
// The metainfo entries are typed as maps of names to values.
func listQueue(queue play.Queue) []map[string]string {
	return pathsToMetadatas(queue.List())
}

// Perform functions on the mp3 player like play and pause.
func servePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if v := queryVal(r, "enqueue"); len(v) > 0 {
			log.Notice("servePlayer: enqueue")
			queue.Enqueue(v)
		} else if _, ok := r.URL.Query()["play"]; ok {
			log.Notice("servePlayer: play")
			err := player.Play()
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
			}
		} else if _, ok := r.URL.Query()["pause"]; ok {
			log.Notice("servePlayer: pause")
			player.Pause()
		} else if _, ok := r.URL.Query()["stop"]; ok {
			log.Notice("servePlayer: stop")
			player.Stop()
		} else if _, ok := r.URL.Query()["getvolume"]; ok {
			v, err := play.GetVolume()
			if err != nil {
				log.Error("%v", err)
			}
			w.Write([]byte("{\"volume\": "))
			w.Write([]byte(strconv.Itoa(int(v))))
			w.Write([]byte("}"))
			log.Notice("servePlayer: getvolume: returning %d", int(v))
		} else if s := queryVal(r, "setvolume"); len(s) > 0 {
			log.Notice("servePlayer: setvolume %s", s)
			v, err := strconv.Atoi(s)
			if err != nil {
				w.WriteHeader(400)
				w.Write([]byte("400 Bad Request: invalid numeric volume"))
			} else {
				if *allVolFlag {
					player.SetVolumeAll(byte(v))
				} else {
					player.SetVolume(byte(v))
				}
			}
		} else if s := queryVal(r, "seek"); len(s) > 0 {
			log.Notice("servePlayer: seek %s", s)
			v, err := strconv.Atoi(s)
			if err != nil {
				w.WriteHeader(400)
				w.Write([]byte("400 Bad Request: invalid numeric offset"))
			} else {
				player.Seek(v)
			}

		} else if _, ok := r.URL.Query()["queue.list"]; ok {
			enc := json.NewEncoder(w)
			enc.Encode(listQueue(queue))
		} else if _, ok := r.URL.Query()["queue.move"]; ok {
			log.Notice("servePlayer: queue.move started")
			s := queryVal(r, "index")
			if len(s) == 0 {
				log.Warning("servePlayer: queue.move: request contained no 'index'")
				return
			}
			i, err := strconv.Atoi(s)
			if err != nil {
				log.Warning("servePlayer: queue.move: index was not an integer: %v", err)
				return
			}

			s = queryVal(r, "delta")
			if len(s) == 0 {
				log.Warning("servePlayer: queue.move: request contained no 'delta'")
				return
			}
			d, err := strconv.Atoi(s)
			if err != nil {
				log.Warning("servePlayer: queue.move: delta was not an integer: %v", err)
				return
			}

			queue.Move(i, d)
			log.Notice("servePlayer: queue.move completed")
		} else if _, ok := r.URL.Query()["queue.move_to_top"]; ok {
			log.Notice("servePlayer: queue.move_to_top started")
			s := queryVal(r, "index")
			if len(s) == 0 {
				log.Warning("servePlayer: queue.move: request contained no 'index'")
				return
			}
			i, err := strconv.Atoi(s)
			if err != nil {
				log.Warning("servePlayer: queue.move: index was not an integer: %v", err)
				return
			}

			queue.MoveToTop(i)
			log.Notice("servePlayer: queue.move_to_top completed")
		} else if _, ok := r.URL.Query()["queue.remove"]; ok {
			s := queryVal(r, "index")
			if len(s) == 0 {
				log.Warning("servePlayer: queue.remove: request contained no 'index'")
				return
			}
			i, err := strconv.Atoi(s)
			if err != nil {
				log.Warning("servePlayer: queue.remove: index was not an integer: %v", err)
				return
			}

			queue.Remove(i)
		} else if _, ok := r.URL.Query()["queue.clear"]; ok {
			queue.Clear()
		} else if _, ok := r.URL.Query()["recent.list"]; ok {
			enc := json.NewEncoder(w)
			enc.Encode(pathsToMetadatas(recent.Slice()))
		}

	}
}

// Perform functions related to mp3 scanning
func serveScan(w http.ResponseWriter, r *http.Request) {
	log.Notice("serveScan: called")
	if r.Method == "GET" {
		if r.URL.Path == "/scan/start" {
			if mp3Dirs != nil {
				scanMutex.Lock()
				defer scanMutex.Unlock()
				if !scanning {
					scanning = true
					go scanDirs(mp3Dirs)
				}
			} else {
				log.Notice("serveScan: scan requested but no directories to scan")
				w.WriteHeader(404)
				w.Write([]byte("404 Not Found: no directories to scan"))
			}
		}
	}
}

/*
serveWebsock implements a websocket connection with the browser, over which events
generated by the player are sent.

The messages sent to the browser are JSON and have the form:

{
  Offset: 0,
  Size: 0,
  State: 0,
  Volume: 0,
  Meta: {
    artist: "s",
    album: "s",
    title: "s",
    path: "s"
    }
}

State is an enumeration with the values:
  0 = empty
  1 = playing
  2 = paused

Meta may be null if there is no loaded mp3.
*/
func serveWebsock(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	userAgent := r.Header.Get("User-Agent")

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("Error upgrading connection to websock: %v", err)
		return
	}

	log.Notice("Websocket connection from %s %v", ws.RemoteAddr(), userAgent)

	defer log.Notice("Websocket handler for %s exiting", ws.RemoteAddr())

	// Send the full status to the browser
	d, err := jsonFullStatus(player.GetStatus(), meta, listQueue(queue), pathsToMetadatas(recent.Slice()))
	if err != nil {
		log.Error("Error encoding Player event as JSON: %v", err)
		return
	}
	log.Notice("Writing complete status to websocket %s", ws.RemoteAddr())
	err = websockWrite(ws, d)
	if err != nil {
		log.Error("Error writing to websocket %s: %v. Handler for socket is terminating.", ws.RemoteAddr(), err)
		// Client is probably gone. Close our channel and exit.
		return
	}

	// Add a new channel to the tee that we will use to detect changes
	c := make(chan interface{})
	eventTee.Add(c)
	defer eventTee.Del(c)

	scanEvents := make(chan interface{})
	scanTee.Add(scanEvents)
	defer scanTee.Del(scanEvents)

	//ws.SetReadDeadline(time.Now().Add(10 * time.Millisecond))

loop:
	for {
		select {
		case e, ok := <-c:
			if !ok {
				// Player died...
				log.Error("Player closed event channel. Closing websocket %v", ws.RemoteAddr())
				ws.Close()
				break loop
			}

			if !websockHandlePlayerEvent(ws, e.(play.Event)) {
				break loop
			}

		case e, ok := <-scanEvents:
			if !ok {
				// Something closed the scanner channel.
				log.Error("Scanner channel was closed. Closing websocket %v", ws.RemoteAddr())
				ws.Close()
				break loop
			}

			d, err = jsonScan(e.(*scan.Metadata))
			if err != nil {
				log.Error("Error encoding metadata as JSON: %v", err)
				// Hopefully the next one works...
				continue loop
			}
			err = websockWrite(ws, d)
			if err != nil {
				log.Error("Error writing to websocket %v: %v", ws.RemoteAddr(), err)
				// Client is probably gone. Close our channel and exit.
				break loop
			}
		}
	}
}

// websockWrite writes bytes to a websocket with a timeout.
func websockWrite(ws *websocket.Conn, payload []byte) error {
	ws.SetWriteDeadline(time.Now().Add(10 * time.Millisecond))
	return ws.WriteMessage(websocket.TextMessage, payload)
}

func websockHandlePlayerEvent(ws *websocket.Conn, event play.Event) (wsValid bool) {
	wsValid = true

	log.Info("Got player event %v", event.Type.String())

	var (
		err error
		d   []byte
	)

	if event.Type == play.StateChange {
		if event.Data.(play.PlayerState) == play.Empty {
			// Commit the mp3 that just finished to the recent list
			recent.Commit()
		}

		s := player.GetStatus()
		if event.Data.(play.PlayerState) == play.Paused || event.Data.(play.PlayerState) == play.Empty {
			// If we just changed to Paused then we may have loaded a new song, and if we changed to
			// Empty we have no song. In these cases send _all_ the information (including song metainfo).
			if event.Data.(play.PlayerState) == play.Paused {
				if len(s.Path) != 0 {
					meta = findMp3ByPath(s.Path)
					if meta == nil {
						log.Error("Loaded mp3, but can't find metainformation for it...")
					}
				}
				setMetainfo()
			} else {
				// Empty.
				meta = nil
			}
			d, err = jsonFullStatus(s, meta, listQueue(queue), pathsToMetadatas(recent.Slice()))
		} else {
			if event.Data.(play.PlayerState) == play.Playing {
				s := player.GetStatus()
				// Hold the current mp3, and add it to the list when it's stopped.
				recent.Hold(s.Path)
			}
			d, err = jsonPlayerEvent(event, nil)
		}
	} else if event.Type == play.QueueChange {
		d, err = jsonPlayerEvent(event, listQueue(queue))
	} else if event.Type == play.Error {
		log.Error("%v", event.Data.(error))
	} else {
		d, err = jsonPlayerEvent(event, nil)
	}

	if err != nil {
		log.Error("Error encoding Player event as JSON: %v", err)
		// Hopefully the next one works...
		return
	}
	log.Notice("Writing status to websocket %v", ws.RemoteAddr())
	err = websockWrite(ws, d)
	if err != nil {
		log.Error("Error writing to websocket %v: %v", ws.RemoteAddr(), err)
		// Client is probably gone. Close our channel and exit
		wsValid = false
	}
	return
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

func initLogging() {
	wasSetup := false

	if len(*logfileFlag) != 0 {
		err := setFileLogBackend(*logfileFlag)
		if err == nil {
			var format = logging.MustStringFormatter(
				"%{time:2006-01-02 15:04:05.000000} %{module}: %{level:.4s} %{color:reset} %{message}",
			)
			logging.SetFormatter(format)
			wasSetup = true
		}
	}

	if !wasSetup {
		// Use stdout
		var format = logging.MustStringFormatter(
			"%{color}%{time:2006-01-02 15:04:05.000000} %{module}: %{level:.4s} %{color:reset} %{message}",
		)
		logging.SetFormatter(format)
	}

	level, levelErr := logging.LogLevel(*logLevelFlag)
	if levelErr == nil {
		logging.SetLevel(level, "")
	} else {
		log.Error("Setting logging level to %v failed: %v", *logLevelFlag, levelErr)
	}
}

// Set the logfile that the logging module will use to the specified logfile.
func setFileLogBackend(logfilePath string) error {
	file, err := os.OpenFile(logfilePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err == nil {
		logging.SetBackend(logging.NewLogBackend(file, "", 0))
		if logFile != nil {
			logFile.Close()
		}
		logFile = file
		return nil
	} else {
		return err
	}
}

// Read events from the player and pass them to the tee.
func pumpPlayerEvents() {
	for e := range player.Events {
		eventTee.In <- e
	}
}

// Read events from the player event tee of type interface{}, convert them to
// events of type play.Event, and write then on the output channel.
func adaptor() chan play.Event {
	c := make(chan interface{})
	out := make(chan play.Event)
	eventTee.Add(c)

	go func() {
		for {
			e, ok := <-c
			if !ok {
				break
			}
			out <- e.(play.Event)
		}
	}()

	return out
}

// Scan the specified directories for mp3s.
func scanDirs(dirs []string) {
	// Since we are running in a separate goroutine we need our own connection to the database:
	// "Multi-thread. In this mode, SQLite can be safely used by multiple threads provided that
	// no single database connection is used simultaneously in two or more threads."
	db, err := openDb(*dbflag)
	if err != nil {
		log.Fatalf("Error opening scanner connection to database: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	var lastSend time.Time

	callback := func(m *scan.Metadata) {
		log.Debug("Scanned %v", m)
		now := time.Now()
		if lastSend.IsZero() || now.Sub(lastSend) > scanEventPeriod {
			scanTee.In <- m
			lastSend = now
		}
	}

	for _, d := range dirs {
		log.Info("Scanning directory %v", d)
		scan.ScanMp3sToDb(d, db, callback)
		log.Info("Done scanning directory %v", d)
	}
	scanTee.In <- (*scan.Metadata)(nil)
	scanning = false
}

func showHelp() {
	fmt.Println("Usage: srv [options] [dir] [dir] ...")
	fmt.Println("")
	fmt.Println("The `dir` arguments are directories to scan for mp3s when a scan request is made.")
	fmt.Println("")
	fmt.Println("Options: ")
	flag.PrintDefaults()
}

func handleSignals() {
	c := make(chan os.Signal, 1)

	signal.Notify(c, syscall.SIGHUP)

	for _ = range c {
		// Reopen the logfile. I am assuming that the go-logging package is safe to be called from multiple goroutines.
		if len(*logfileFlag) != 0 {
			setFileLogBackend(*logfileFlag)
			log.Info("Reopened logfile on HUP")
		}
	}
}

func main() {
	flag.Parse()

	if *helpFlag {
		showHelp()
		os.Exit(0)
	}

	mp3Dirs = flag.Args()

	initLogging()
	log = logging.MustGetLogger("server")

	var err error

	// Open database
	db, err = openDb(*dbflag)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	// Set up the play queue
	queue = play.NewQueueWithEvents(player, adaptor())

	go pumpPlayerEvents()

	go handleSignals()

	// Setup http server
	http.HandleFunc("/songmeta", serveMeta)
	http.HandleFunc("/player", servePlayer)
	http.HandleFunc("/scan/", serveScan)
	//http.Handle("/playerEvents", websocket.Handler(playerEvents))
	http.HandleFunc("/playerEvents", serveWebsock)
	http.Handle("/", http.FileServer(http.Dir("src/github.com/jeffwilliams/wwwmp3/www")))

	log.Notice("Listening on http://localhost:2001/")

	err = http.ListenAndServe(":2001", nil)
	if err != nil {
		log.Fatal("ListenAndServe failed: %v", err)
		os.Exit(1)
	}
}
