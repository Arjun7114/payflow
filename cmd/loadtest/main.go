// Command loadtest fires concurrent requests at a running payflow server and
// reports latency percentiles (p50/p95/p99) plus throughput.
//
// It measures round-trip latency on the machine it runs on — client, server,
// and database are typically all local — so the numbers are a RELATIVE measure
// (in-memory vs Postgres, regressions) and a check that the handler path is
// fast. They are not a claim about production latency under real network load.
//
// Usage:
//   go run ./cmd/loadtest                 # defaults: 2000 requests, 50 concurrent
//   go run ./cmd/loadtest -n 5000 -c 100
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	base := flag.String("base", "http://localhost:8080", "server base URL")
	n := flag.Int("n", 2000, "total number of requests")
	c := flag.Int("c", 50, "concurrent workers")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}

	// Warm up: make sure the server is reachable before timing anything.
	if _, err := client.Get(*base + "/healthz"); err != nil {
		log.Fatalf("server not reachable at %s: %v", *base, err)
	}

	latencies := make([]time.Duration, *n)
	var errCount int64

	jobs := make(chan int, *n)
	var wg sync.WaitGroup

	start := time.Now()
	for w := 0; w < *c; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				latencies[i] = timeOne(client, *base, i, &errCount)
			}
		}()
	}
	for i := 0; i < *n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	report(latencies, errCount, total, *n, *c)
}

// timeOne runs one full payment lifecycle request set and returns the latency
// of the capture call (the most representative state-changing operation). Each
// iteration uses a unique payment ID so requests are independent.
func timeOne(client *http.Client, base string, i int, errCount *int64) time.Duration {
	id := fmt.Sprintf("load-%d", i)

	post := func(path, key, body string) bool {
		req, _ := http.NewRequest("POST", base+path, bytes.NewReader([]byte(body)))
		req.Header.Set("Idempotency-Key", key)
		resp, err := client.Do(req)
		if err != nil {
			atomic.AddInt64(errCount, 1)
			return false
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode < 400
	}

	// Set up to the authorized state (not timed).
	post("/payments", id+"-c", `{"payment_id":"`+id+`"}`)
	post("/payments/"+id+"/authorize", id+"-a", "")

	// Time the capture — one representative state-changing request.
	t0 := time.Now()
	ok := post("/payments/"+id+"/capture", id+"-cap", "")
	d := time.Since(t0)
	if !ok {
		atomic.AddInt64(errCount, 1)
	}
	return d
}

func report(lat []time.Duration, errCount int64, total time.Duration, n, c int) {
	sorted := make([]time.Duration, len(lat))
	copy(sorted, lat)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	pct := func(p float64) time.Duration {
		if len(sorted) == 0 {
			return 0
		}
		idx := int(p/100*float64(len(sorted))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	mean := time.Duration(0)
	if len(sorted) > 0 {
		mean = sum / time.Duration(len(sorted))
	}
	rps := float64(n) / total.Seconds()

	fmt.Printf("\npayflow load test — capture latency\n")
	fmt.Printf("  requests:    %d  (concurrency %d)\n", n, c)
	fmt.Printf("  errors:      %d\n", errCount)
	fmt.Printf("  wall time:   %s\n", total.Round(time.Millisecond))
	fmt.Printf("  throughput:  %.0f req/s\n", rps)
	fmt.Printf("  latency min: %s\n", pct(0).Round(time.Microsecond))
	fmt.Printf("  latency p50: %s\n", pct(50).Round(time.Microsecond))
	fmt.Printf("  latency p95: %s\n", pct(95).Round(time.Microsecond))
	fmt.Printf("  latency p99: %s\n", pct(99).Round(time.Microsecond))
	fmt.Printf("  latency max: %s\n", pct(100).Round(time.Microsecond))
	fmt.Printf("  latency avg: %s\n", mean.Round(time.Microsecond))
}
