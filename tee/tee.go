package tee

// Tee implements a channel tee: items written on In are written to all registered out channels.
type Tee struct {
	In   chan interface{}
	outs []chan interface{}
	add  chan chan interface{}
	del  chan chan interface{}
}

func (t *Tee) delet(c chan interface{}) {
  if len(t.outs) == 0 {
	  return
	}
  outs := make([]chan interface{}, len(t.outs)-1)
  j := 0
  for _, o := range t.outs {
    if o != c {
      if j < len(outs) {
        outs[j] = o
      }
      j++
    }
  }
  // If j == len(outs) then the channel was found and deleted. Otherwise this channel was never added to the tee.
  if j <= len(outs) {
    t.outs = outs
  }

}

// NewFrom is the same as New, but instead of making the In chan, this function uses the passed channel as the In chan.
func NewFrom(in chan interface{}) *Tee {
  if in == nil {
    in = make(chan interface{})
  }

	t := Tee{
		In:   in,
		outs: make([]chan interface{}, 0),
		add:  make(chan chan interface{}),
		del:  make(chan chan interface{}),
	}

	go func() {
	main:
		for {
			select {
			case a, ok := <-t.add:
				if !ok {
					break
				}
				// User wants to add another channel to the tee
				// Make sure it doesn't already exist.
				for _, c := range t.outs {
					if c == a {
						continue main
					}
				}
				t.outs = append(t.outs, a)
			case d, ok := <-t.del:
				if !ok {
					break
				}
        t.delet(d)
			case s, ok := <-t.In:
				if !ok {
					// Input channel closed. Close all output channels
					for _, c := range t.outs {
						close(c)
            t.delet(c)
					}
					break
				}
				// Send input over all the output channels.
				for _, c := range t.outs {
					c <- s
				}
			}
		}
	}()

	return &t
}

// NewTee creates a new Tee.
func New() *Tee {
  return NewFrom(nil)
}

// Del deletes the specified channel from the Tee.
func (t Tee) Del(c chan interface{}) {
  // Since the Tee is writing to the channel, and we are writing
  // to the Tee to delete the channel we can cause a deadlock where
  // the two goroutines are both blocked on write.
  // We prevent this by dropping all messages the Tee is writing to
  // the channel while we delete the channel.
  for{
    select {
    case _ = <-c:
    case t.del <- c:
      return
    }
  }
}

// Add adds the specified channel to the Tee.
func (t Tee) Add(c chan interface{}) {
  t.add <- c
}
