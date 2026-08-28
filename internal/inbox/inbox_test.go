package inbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestQueuePersistsOrdersAndDeletesSettledContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sqlite")
	queue, err := Open(path, Limits{Records: 10, Bytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	records := []Record{testRecord("one"), testRecord("two")}
	consumers := []Consumer{{Tailapp: "guard", Revision: "r1"}, {Tailapp: "cost", Revision: "r2"}}
	positions, err := queue.Enqueue(context.Background(), records, consumers)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(positions) != "[1 2]" {
		t.Fatalf("positions = %v", positions)
	}
	if err := queue.Complete(context.Background(), "guard", 1); err != nil {
		t.Fatal(err)
	}
	if stats, _ := queue.Stats(context.Background()); stats.Records != 2 {
		t.Fatalf("record deleted before all consumers settled: %#v", stats)
	}
	if err := queue.Close(); err != nil {
		t.Fatal(err)
	}

	queue, err = Open(path, Limits{Records: 10, Bytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	pending, err := queue.Pending(context.Background(), "cost", 10)
	if err != nil || len(pending) != 2 || pending[0].EventID != "local:1" {
		t.Fatalf("recovered pending = %#v, %v", pending, err)
	}
	if err := queue.Complete(context.Background(), "cost", 1); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(context.Background(), "cost", 1); err != nil {
		t.Fatalf("replayed completion after cleanup: %v", err)
	}
	if stats, _ := queue.Stats(context.Background()); stats.Records != 1 {
		t.Fatalf("settled content retained: %#v", stats)
	}
	if err := queue.DetachAll(context.Background(), "guard", "test_gap"); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(context.Background(), "cost", 2); err != nil {
		t.Fatal(err)
	}
	if stats, _ := queue.Stats(context.Background()); stats.Records != 0 || stats.PendingObligations != 0 {
		t.Fatalf("queue did not drain: %#v", stats)
	}
	next, err := queue.Enqueue(context.Background(), []Record{testRecord("three")}, consumers)
	if err != nil || fmt.Sprint(next) != "[3]" {
		t.Fatalf("durable delivery head was reused: %v, %v", next, err)
	}
}

func TestQueueBackpressureIsAtomic(t *testing.T) {
	queue, err := Open(filepath.Join(t.TempDir(), "control.sqlite"), Limits{Records: 2, Bytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	consumer := []Consumer{{Tailapp: "guard", Revision: "r1"}}
	if _, err := queue.Enqueue(context.Background(), []Record{testRecord("one")}, consumer); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Enqueue(context.Background(), []Record{testRecord("two"), testRecord("three")}, consumer); !errors.Is(err, ErrFull) {
		t.Fatalf("over-capacity batch = %v", err)
	}
	stats, _ := queue.Stats(context.Background())
	if stats.Records != 1 {
		t.Fatalf("partial batch committed: %#v", stats)
	}
}

func TestConcurrentRequestsReceiveUniqueContiguousPositions(t *testing.T) {
	queue, err := Open(filepath.Join(t.TempDir(), "control.sqlite"), Limits{Records: 100, Bytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	consumer := []Consumer{{Tailapp: "guard", Revision: "r1"}}
	positions := make(chan int64, 20)
	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			accepted, err := queue.Enqueue(context.Background(), []Record{testRecord(fmt.Sprint(index))}, consumer)
			if err != nil {
				t.Errorf("enqueue: %v", err)
				return
			}
			positions <- accepted[0]
		}(index)
	}
	wait.Wait()
	close(positions)
	seen := make(map[int64]bool)
	for position := range positions {
		seen[position] = true
	}
	for position := int64(1); position <= 20; position++ {
		if !seen[position] {
			t.Fatalf("missing position %d in %v", position, seen)
		}
	}
}

func testRecord(name string) Record {
	return Record{Signal: "log", Name: name, Source: "test", ContentDigest: "sha256:test", JSON: []byte(`{"name":"` + name + `"}`)}
}
