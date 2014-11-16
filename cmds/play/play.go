// Simple mp3 player
package main

import (
  "github.com/jeffwilliams/wwwmp3/play"
  "os"
  "fmt"
  "time"
)

import "C"

func main(){
  if len(os.Args) < 2 {
    fmt.Println("Pass the mp3 file to play")
    os.Exit(1)
  }

  meta := play.GetMetadata(os.Args[1])
  fmt.Printf("Title:  '%s'\n", meta.Title)
  fmt.Printf("Artist: '%s'\n", meta.Artist)
  fmt.Printf("Album:  '%s'\n", meta.Album)

  play.SetVolume(50)
  fmt.Println("Volume is", play.GetVolume())

  fmt.Println("Got passed",len(os.Args),"args")

_ = time.Second
/*
  go play.Play(os.Args[1])

  time.Sleep(2*time.Second)
  play.SetVolume(40)
  time.Sleep(2*time.Second)
  play.SetVolume(30)
  time.Sleep(2*time.Second)
  play.SetVolume(50)
  time.Sleep(2*time.Second)
*/

  // New-style player

  player := play.NewPlayer()

  //size, err := player.Load(os.Args[1], nil)

  offchan := make(chan int)
  size, err := player.Load(os.Args[1], offchan)
  if err != nil {
    fmt.Println("Loading mp3 failed:",err)
    return
  }
  player.Play()

  playing := true

  fmt.Println(size, "samples")

  stdin := make(chan string)
  go func(){
    b := make([]byte,1)
    for{
      _, err := os.Stdin.Read(b)
      if err != nil{
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
    case i := <-offchan:
      fmt.Printf("offset %d/%d\n",i,size)
    }


  }
}
