// Command play is a simple mp3 player.
package main

import (
	"fmt"
	"github.com/jeffwilliams/wwwmp3/play"
	"os"
	"time"
)

import "C"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Pass the mp3 file to play")
		os.Exit(1)
	}

	meta := play.GetMetadata(os.Args[1])
	fmt.Printf("Title:  '%s'\n", meta.Title)
	fmt.Printf("Artist: '%s'\n", meta.Artist)
	fmt.Printf("Album:  '%s'\n", meta.Album)
	fmt.Printf("Tracknum:  '%d'\n", meta.Tracknum)

	/*
		play.SetVolume(50)
	*/
	volume, err := play.GetVolume()
	fmt.Println("Volume is", volume)

	fmt.Println("Got passed", len(os.Args), "args")

	_ = time.Second

	// New-style player

	player := play.NewPlayer()

	//size, err := player.Load(os.Args[1], nil)

	size, err := player.Load(os.Args[1])
	if err != nil {
		fmt.Println("Loading mp3 failed:", err)
		return
	}
	player.Play()

	playing := true

	fmt.Println(size, "samples")

	stdin := make(chan string)
	go func() {
		b := make([]byte, 1)
		for {
			_, err := os.Stdin.Read(b)
			if err != nil {
				break
			}
			stdin <- string(b)
		}
		close(stdin)
	}()

	// Simple command loop
	for {
		select {
		case s := <-stdin:
			switch s {
			case " ":
				fmt.Println("Pause")
				if playing {
					player.Pause()
					playing = false
				} else {
					player.Play()
					playing = true
				}
			}
		}

	}
}
