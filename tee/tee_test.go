package tee

import "testing"

func TestEmpty(*testing.T) {
  // Tests two things: writing to an empty Tee doesn't block, and deleting from an empty Tee doesn't block.
  // This test passes if this function returns.
	tee := New()

	tee.In <- "test"

	c1 := make(chan interface{})
	tee.Del(c1)
}

func TestAddOne(test *testing.T) {
	tee := New()

	c1 := make(chan interface{})

	tee.Add(c1)

	tee.In <- "test"

	v := <-c1

	if v != "test" {
		test.Fatal("Got ", v, " instead of test")
	}
}

func TestAddTwo(test *testing.T) {
	tee := New()

	c1 := make(chan interface{})
	c2 := make(chan interface{})

	tee.Add(c1)
	tee.Add(c2)

	tee.In <- "test"

	v1 := <-c1
	v2 := <-c2

	if v1 != "test" {
		test.Fatal("Got ", v1, " instead of test")
	}
	if v2 != "test" {
		test.Fatal("Got ", v2, " instead of test")
	}
}

func TestInvalDelete(test *testing.T) {
	// Delete something not in the tee

	tee := New()

	c1 := make(chan interface{})
	c2 := make(chan interface{})

	tee.Add(c1)
	tee.Del(c2)
}

func TestDoubleAdd(test *testing.T) {
	tee := New()

	c1 := make(chan interface{})

	tee.Add(c1)
	tee.Add(c1)

	if len(tee.outs) != 1 {
		test.Fatal("Adding the same channel twice is possible.", len(tee.outs))
	}
}

func TestDelete(test *testing.T) {
	tee := New()

	c1 := make(chan interface{})
	c2 := make(chan interface{}, 1)

	tee.Add(c1)
	tee.Add(c2)

	tee.Del(c2)

	tee.In <- "test"
	c2 <- "blort"

	v1 := <-c1
	if v1 != "test" {
		test.Fatal("Got ", v1, " instead of test")
	}

	v2 := <-c2
	if v2 == "test" {
		test.Fatal("Got test when channel was removed from tee")
	}
	if v2 != "blort" {
		test.Fatal("Got ", v2, " instead of blort")
	}

	tee.Del(c1)

	tee.In <- "dead end"
}

func TestDeleteDeadlock(*testing.T) {
  // Validate there is no deadlock on delete. 
  // The possible deadlock is the following. If the Tee goroutine is trying to send 
  // to an output channel, but the goroutine that would read that channel is trying to 
  // delete the channel (and so is sending to the Tee goroutine) then the goroutines would 
  // deadlock since each is waiting for the other to read.
  //
  // If this test function returns, then the test passes.

  tee := New()

  c := make(chan interface{})

  tee.Add(c)

  go func(){ tee.In <- "blort" }()

  tee.Del(c)

}
