package play

import (
	"container/list"
	"fmt"
	"sort"
)

type queueCmdType int

const (
	Move queueCmdType = iota
	MoveToTop
	Remove
	Clear
)

// Sort ints
type Asc []int

func (a Asc) Len() int           { return len(a) }
func (a Asc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Asc) Less(i, j int) bool { return a[i] < a[j] }

type Desc []int

func (a Desc) Len() int           { return len(a) }
func (a Desc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Desc) Less(i, j int) bool { return a[i] > a[j] }

type queueCmd struct {
	Type    queueCmdType
	Indexes []int
	Delta   int
}

// QueueElem is an element in the play queue.
type QueueElem struct {
	Filename string
	// A unique id for the element in the queue. This can be used to identify elements
	// after the queue has been modified (i.e. if items have been moved).
	Id uint32
}

// Queue implements a queue of metadata for a player. Items from the queue
// are added to the player when the player is Empty.
type Queue struct {
	player  Player
	files   *list.List
	enqueue chan string
	list    chan chan []QueueElem
	modify  chan queueCmd
	nextId  uint32
}

func NewQueue(player Player) Queue {
	return NewQueueWithEvents(player, player.Events)
}

func NewQueueWithEvents(player Player, events chan Event) Queue {
	q := Queue{
		files:   list.New(),
		enqueue: make(chan string),
		list:    make(chan chan []QueueElem),
		modify:  make(chan queueCmd),
		nextId:  0,
	}

	nth := func(list *list.List, n int) *list.Element {
		e := list.Front()
		for i := 0; e != nil && i < n; i, e = i+1, e.Next() {
		}
		return e
	}

	sendEvent := func(e Event) {
		// Do not block when sending events. We buffer some and then drop the rest.
		// This is to prevent deadlocks: users of the client need to read events, but
		// they also send commands to the player. If the player is sending an event
		// while the event reader is sending a command to the player we have a deadlock.
		select {
		case player.Events <- e:
		default:
		}
	}

	addToPlayer := func() {
		if q.files.Len() == 0 {
			return
		}

		s := player.GetStatus()
		if s.State == Empty {

			elem := q.files.Remove(q.files.Front()).(QueueElem)

			_, err := player.Load(elem.Filename)
			if err != nil {
				// TODO: If we get an error here, we'll stop trying to play stuff until
				// something is added to the queue. Should we start a timer to retry here? It could be
				// a temporary condition like something (youtube) stole the audio device.
				sendEvent(Event{Type: Error, Data: fmt.Errorf("Queue failed to load file %s: %t", elem.Filename, err)})
				return
			}
			err = player.Play()
			if err != nil {
				sendEvent(Event{Type: Error, Data: fmt.Errorf("Queue failed to play file %s: %t", elem.Filename, err)})
				return
			}
		}
	}

	move := func(indexes []int, delta int) {
		if delta < 0 {
			delta = -1
			sort.Sort(Asc(indexes))
		} else if delta > 0 {
			sort.Sort(Desc(indexes))
		}

		for _, index := range indexes {

			if index < 1 && delta == -1 || index > q.files.Len()-2 && delta == 1 {
				continue
			}

			e := nth(q.files, index)

			if delta < 0 {
				p := e.Prev()
				v := q.files.Remove(e)
				q.files.InsertBefore(v, p)
			} else {
				n := e.Next()
				v := q.files.Remove(e)
				q.files.InsertAfter(v, n)
			}
		}
	}

	moveToTop := func(indexes []int) {
		sort.Sort(Desc(indexes))

		mod := 0
		for _, index := range indexes {
			i := index + mod
			e := nth(q.files, i)
			v := q.files.Remove(e)
			q.files.PushFront(v)
			mod += 1
		}
	}

	remove := func(indexes []int) {
		sort.Sort(Asc(indexes))

		mod := 0
		for _, index := range indexes {

			i := index + mod

			if i < 0 || i > q.files.Len()-1 {
				return
			}

			e := nth(q.files, i)
			q.files.Remove(e)
			mod -= 1
		}
	}

	clear := func() {
		for q.files.Len() > 0 {
			q.files.Remove(q.files.Front())
		}
	}

	modify := func(cmd queueCmd) {
		if cmd.Type == Move {
			move(cmd.Indexes, cmd.Delta)
		} else if cmd.Type == MoveToTop {
			moveToTop(cmd.Indexes)
		} else if cmd.Type == Remove {
			remove(cmd.Indexes)
		} else if cmd.Type == Clear {
			clear()
		}
	}

	// Dequeue items as required.
	go func() {
		for {
			select {
			case c := <-q.list:
				a := make([]QueueElem, q.files.Len())
				i := 0
				for e := q.files.Front(); e != nil; e = e.Next() {
					a[i] = e.Value.(QueueElem)
					i++
				}
				c <- a
			case f := <-q.enqueue:
				q.files.PushBack(QueueElem{f, q.nextId})
				q.nextId += 1
				addToPlayer()
				sendEvent(Event{Type: QueueChange})
			case e := <-events:
				if e.Type == StateChange {
					addToPlayer()
					sendEvent(Event{Type: QueueChange})
				}
			case e := <-q.modify:
				modify(e)
				sendEvent(Event{Type: QueueChange})
			}

		}
	}()

	return q
}

func (q Queue) Enqueue(filename string) {
	q.enqueue <- filename
}

// List returns the contents of the queue.
func (q Queue) List() []QueueElem {
	c := make(chan []QueueElem)
	q.list <- c
	return <-c
}

// Move moves the specified queue element `index` by `delta`. Currently only
// -1 and +1 are supported for delta.
func (q Queue) Move(indexes []int, delta int) {
	q.modify <- queueCmd{Type: Move, Indexes: indexes, Delta: delta}
}

// Move the specified queue element to the top of the queue.
func (q Queue) MoveToTop(indexes []int) {
	q.modify <- queueCmd{Type: MoveToTop, Indexes: indexes}
}

// Remove removes the specified queue element from the queue.
func (q Queue) Remove(indexes []int) {
	q.modify <- queueCmd{Type: Remove, Indexes: indexes}
}

// Clear the queue
func (q Queue) Clear() {
	q.modify <- queueCmd{Type: Clear}
}
