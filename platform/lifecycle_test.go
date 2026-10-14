package platform

import "testing"

func TestAppQueueWakeOrderingAndQuit(t *testing.T) {
	var q AppQueue
	var sequence []int
	q.Post(func() {
		sequence = append(sequence, 1)
		q.Post(func() { sequence = append(sequence, 3) })
	})
	wakes := 0
	q.SetWake(func() { wakes++ })
	if wakes != 1 {
		t.Fatalf("queued work did not wake loop: %d", wakes)
	}
	q.Post(func() { sequence = append(sequence, 2) })
	if q.Drain() || len(sequence) != 2 || sequence[0] != 1 || sequence[1] != 2 {
		t.Fatalf("first drain: %v", sequence)
	}
	if q.Drain() || len(sequence) != 3 || sequence[2] != 3 {
		t.Fatalf("second drain: %v", sequence)
	}
	q.Post(func() { t.Error("ran after quit") })
	q.Quit()
	q.Post(func() { t.Error("accepted after quit") })
	if !q.Drain() || wakes != 5 {
		t.Fatalf("quit was not delivered: wakes=%d", wakes)
	}
}

func TestAppQueueQuitDuringCallbackSkipsRemainingWork(t *testing.T) {
	var q AppQueue
	q.Post(q.Quit)
	q.Post(func() { t.Fatal("ran after Quit") })
	if !q.Drain() {
		t.Fatal("Quit did not stop the loop")
	}
}
