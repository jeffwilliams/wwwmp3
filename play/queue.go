package play

import "container/list"

type queueCmdType int

const (
	Move queueCmdType = iota
	Remove
	Clear
)

type queueCmd struct {
	Type  queueCmdType
	Index int
	Delta int
}

// Queue implements a queue of metadata for a player. Items from the queue
// are added to the player when the player is Empty.
type Queue struct {
	player  Player
	files   *list.List
	enqueue chan string
	list    chan chan []string
	modify  chan queueCmd
}

func NewQueue(player Player) Queue {
	return NewQueueWithEvents(player, player.Events)
}

func NewQueueWithEvents(player Player, events chan Event) Queue {
	q := Queue{
		files:   list.New(),
		enqueue: make(chan string),
		list:    make(chan chan []string),
		modify:  make(chan queueCmd),
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

			file := q.files.Remove(q.files.Front()).(string)

			_, err := player.Load(file)
			if err != nil {
				return
			}
			err = player.Play()
			if err != nil {
				return
			}
		}
	}

	move := func(index, delta int) {
		if delta < 0 {
			delta = -1
		} else if delta > 0 {
			delta = 1
		}

		if index < 1 && delta == -1 || index > q.files.Len()-2 && delta == 1 {
			return
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

	remove := func(index int) {
		if index < 0 || index > q.files.Len()-1 {
			return
		}

		e := nth(q.files, index)
		q.files.Remove(e)
	}

	clear := func() {
		for q.files.Len() > 0 {
			q.files.Remove(q.files.Front())
		}
	}

	modify := func(cmd queueCmd) {
		if cmd.Type == Move {
			move(cmd.Index, cmd.Delta)
		} else if cmd.Type == Remove {
			remove(cmd.Index)
		} else if cmd.Type == Clear {
			clear()
		}
	}

	// Dequeue items as required.
	go func() {
		for {
			select {
			case c := <-q.list:
				a := make([]string, q.files.Len())
				i := 0
				for e := q.files.Front(); e != nil; e = e.Next() {
					a[i] = e.Value.(string)
					i++
				}
				c <- a
			case f := <-q.enqueue:
				q.files.PushBack(f)
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
func (q Queue) List() []string {
	c := make(chan []string)
	q.list <- c
	return <-c
}

// Move moves the specified queue element `index` by `delta`. Currently only
// -1 and +1 are supported for delta.
func (q Queue) Move(index int, delta int) {
	q.modify <- queueCmd{Type: Move, Index: index, Delta: delta}
}

// Remove removes the specified queue element from the queue.
func (q Queue) Remove(index int) {
	q.modify <- queueCmd{Type: Remove, Index: index}
}

// Clear the queue
func (q Queue) Clear() {
	q.modify <- queueCmd{Type: Clear}
}