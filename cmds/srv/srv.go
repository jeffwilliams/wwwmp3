// Command srv implements a webserver that implements an mp3 player and hosts a UI for it.
//
// This command expects an mp3 database to be set up and filled.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/jeffwilliams/go-logging"
	"github.com/jeffwilliams/wwwmp3/play"
	"github.com/jeffwilliams/wwwmp3/scan"
	"github.com/jeffwilliams/wwwmp3/tee"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	scanEventPeriod = 1 * time.Second
)

var (
	helpFlag      = pflag.BoolP("help", "h", false, "Print help then exit.")
	genConfigFlag = pflag.BoolP("gen", "g", false, "Generate a sample configuration file in the current directory then exit.")
	db            scan.Mp3Db

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

	// Current repeat mode of the player
	repeatMode RepeatMode = DontRepeat

	// When the repeatMode is changed, this tee is written to.
	repeatModeTee = tee.New()
)

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

// Given an list of paths to mp3 files, return a list of metadata maps
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

// Give a list of QueueElems, return a list of metadata maps
func queueElemsToMetadatas(list []play.QueueElem) (result []map[string]string) {
	result = make([]map[string]string, 0)

	for _, elem := range list {
		m := findMp3ByPath(elem.Filename)
		if m == nil {
			m = map[string]string{
				"artist": "?",
				"album":  "?",
				"title":  "?",
				"path":   elem.Filename,
			}
		}
		m["queueId"] = strconv.FormatUint(uint64(elem.Id), 10)
		result = append(result, m)
	}
	return
}

// listQueue returns a slice containing the metainfo of the tracks in the play queue.
// The metainfo entries are typed as maps of names to values.
func listQueue(queue play.Queue) []map[string]string {
	return queueElemsToMetadatas(queue.List())
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
		s := player.GetStatus()
		if event.Data.(play.PlayerState) == play.Paused || event.Data.(play.PlayerState) == play.Empty {
			// If we just changed to Paused then we may have loaded a new song, and if we changed to
			// Empty we have no song. In these cases send _all_ the information (including song metainfo).
			d, err = jsonFullStatus(s, meta, listQueue(queue), pathsToMetadatas(recent.Slice()), repeatMode)
		} else {
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

	if len(viper.GetString("log")) != 0 {
		err := setFileLogBackend(viper.GetString("log"))
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

	level, levelErr := logging.LogLevel(viper.GetString("loglevel"))
	if levelErr == nil {
		logging.SetLevel(level, "")
	} else {
		log.Error("Setting logging level to %v failed: %v", viper.GetString("loglevel"), levelErr)
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
// If the player events affect our internal state, change state based on the events
// (If repeat mode is set to repeat all, re-enqueue completed songs)
func handlePlayerEvents() {
	for e := range player.Events {
		if e.Type == play.StateChange {
			if e.Data.(play.PlayerState) == play.Empty {
				if repeatMode == RepeatAll {
					// Re-enqueue this sucker
					queue.Enqueue(recent.Held)
				}
				// Commit the mp3 that just finished to the recent list
				recent.Commit()

			} else if e.Data.(play.PlayerState) == play.Paused {
				s := player.GetStatus()
				// If we just changed to Paused then we may have loaded a new song.
				if len(s.Path) != 0 {
					meta = findMp3ByPath(s.Path)
					if meta == nil {
						log.Error("Loaded mp3, but can't find metainformation for it...")
					}
				}
				setMetainfo()
			} else if e.Data.(play.PlayerState) == play.Empty {
				// If we changed to Empty we have no song.
				meta = nil
			} else if e.Data.(play.PlayerState) == play.Playing {
				s := player.GetStatus()
				// Hold the current mp3, and add it to the list when it's stopped.
				recent.Hold(s.Path)
			}
		}

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
	db, err := openDb(viper.GetString("db"))
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
	pflag.PrintDefaults()
}

func handleSignals() {
	c := make(chan os.Signal, 1)

	signal.Notify(c, syscall.SIGHUP)

	for _ = range c {
		// Reopen the logfile. I am assuming that the go-logging package is safe to be called from multiple goroutines.
		if len(viper.GetString("log")) != 0 {
			setFileLogBackend(viper.GetString("log"))
			log.Info("Reopened logfile on HUP")
		}
	}
}

func findWwwDir() string {

	locs := []string{"www", "src/github.com/jeffwilliams/wwwmp3/www", "/usr/share/wwwmp3/www"}

	for _, f := range locs {

		fi, err := os.Stat(f)
		if err != nil {
			log.Info("The www root directory was not found at %v.", f)
			continue
		}

		if !fi.Mode().IsDir() {
			log.Info("The www root %v is not a directory.", f)
			continue
		}

		log.Info("Using www root %v", f)
		return f
	}

	log.Fatalf("The www root could not be located!")

	return ""
}

func initViper() {
	pflag.IntP("port", "p", 2001, "TCP Port to listen on")
	pflag.StringP("db", "d", "", "Database containing mp3 info")
	pflag.StringP("log", "l", "", "File to write log messages to. Defaults to stdout if not specified.")
	pflag.StringP("loglevel", "e", "", "Minimum severity of log messages to write. One of DEBUG, INFO, NOTICE, WARNING, ERROR, or CRITICAL")
	pflag.IntP("max_recent", "r", 100, "Maximum number of songs in the Recently Played list.")

	viper.BindPFlags(pflag.CommandLine)

	viper.SetDefault("db", "mp3.db")
	viper.SetDefault("port", 2001)
	viper.SetDefault("log", "")
	viper.SetDefault("loglevel", "DEBUG")
	viper.SetDefault("max_recent", 100)

	// Config file basename. Actual config file is config.yaml, .toml, etc.
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/wwwmp3/")

}

func genConfig() {
	const name = "config.yaml"

	if _, err := os.Stat(name); err == nil {
		fmt.Printf("A file named %v already exists in the current directory\n", name)
		return
	}

	file, err := os.Create(name)
	if err != nil {
		fmt.Println("Generating config file failed:", err)
		return
	}

	fmt.Fprintln(file, "## TCP Port for the server to listen on")
	fmt.Fprintln(file, "port: 2001")
	fmt.Fprintln(file, "")
	fmt.Fprintln(file, "## Path to the sqlite3 database that contains the mp3 information.")
	fmt.Fprintln(file, "db: 'mp3.db'")
	fmt.Fprintln(file, "")
	fmt.Fprintln(file, "## File to write logs to. If this is not specified, stdout is used.")
	fmt.Fprintln(file, "# log: '/var/log/wwwmp3/wwwmp3.log'")
	fmt.Fprintln(file, "")
	fmt.Fprintln(file, "## Minimum logging level to print. Must be one of DEBUG, INFO, NOTICE, WARNING, ERROR, or CRITICAL")
	fmt.Fprintln(file, "loglevel: 'DEBUG'")
	fmt.Fprintln(file, "")
	fmt.Fprintln(file, "## Maximum number of songs in the Recently Played list.")
	fmt.Fprintln(file, "max_recent: 100")
	fmt.Fprintln(file, "")

	file.Close()

	fmt.Println("Generated config file", name)
}

func main() {
	initViper()
	pflag.Parse()
	viper.ReadInConfig()

	if *helpFlag {
		showHelp()
		os.Exit(0)
	} else if *genConfigFlag {
		genConfig()
		os.Exit(0)
	}

	mp3Dirs = flag.Args()

	initLogging()
	log = logging.MustGetLogger("server")
	log.Info("Used config file %v", viper.ConfigFileUsed())

	var err error

	// Open database
	db, err = openDb(viper.GetString("db"))
	if err != nil {
		log.Fatalf("Error opening database %v: %v", viper.GetString("db"), err)
		os.Exit(1)
	}
	defer db.Close()

	// Set up the play queue
	queue = play.NewQueueWithEvents(player, adaptor())

	go handlePlayerEvents()

	go handleSignals()

	recent.Max = viper.GetInt("max_recent")

	// Setup http server
	http.HandleFunc("/songmeta", serveMeta)
	http.HandleFunc("/player/", servePlayer)
	http.HandleFunc("/scan/", serveScan)
	http.HandleFunc("/playerEvents", serveWebsock)

	if wwwDir := findWwwDir(); len(wwwDir) > 0 {
		http.Handle("/", http.FileServer(http.Dir(wwwDir)))
	} else {
		os.Exit(1)
	}

	addr := ":" + viper.GetString("port")
	log.Notice("Listening on %v", addr)

	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe failed: %v", err)
		os.Exit(1)
	}
}
