package main

//import "fmt"

type Tee struct {
	In   chan int
	Outs []chan int
	Add  chan chan int
	Del  chan chan int
}

func NewTee() *Tee {
	t := Tee{
		In:   make(chan int),
		Outs: make([]chan int, 0),
		Add:  make(chan chan int),
		Del:  make(chan chan int),
	}

	go func() {
	main:
		for {
			select {
			case a, ok := <-t.Add:
				if !ok {
					break
				}
				// User wants to add another channel to the tee
				// Make sure it doesn't already exist.
				for _, c := range t.Outs {
					if c == a {
						continue main
					}
				}
				t.Outs = append(t.Outs, a)
			case d, ok := <-t.Del:
				if !ok {
					break
				}
				if len(t.Outs) == 0 {
					continue
				}
				// User wants to delete a channel from the tee
				outs := make([]chan int, len(t.Outs)-1)
				j := 0
				for _, o := range t.Outs {
					if o != d {
						if j < len(outs) {
							outs[j] = o
						}
						j++
					}
				}
				// If j == len(outs) then the channel was found and deleted. Otherwise this channel was never added to the tee.
				if j <= len(outs) {
					t.Outs = outs
				}
			case s, ok := <-t.In:
				if !ok {
          // Input channel closed. Close all output channels
          for _,c := range t.Outs {
            close(c)
            t.Del <- c
          }
					break
				}
				// Send input over all the output channels
				for _, c := range t.Outs {
					c <- s
				}
			}
		}
	}()

	return &t
}

