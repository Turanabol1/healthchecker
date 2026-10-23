package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Result — результат опитування однієї адреси.
type Result struct {
	URL        string
	StatusCode int
	Status     string
	Duration   time.Duration
	Err        error
}

// Checker — контракт перевірки ресурсу (реалізується неявно).
type Checker interface {
	Check(url string) Result
}

// HTTPChecker — HTTP-клієнт із налаштованим таймаутом.
type HTTPChecker struct {
	client *http.Client
}

func NewHTTPChecker(timeout time.Duration) *HTTPChecker {
	return &HTTPChecker{client: &http.Client{Timeout: timeout}}
}

// Check виконує GET-запит і вимірює його тривалість.
func (c *HTTPChecker) Check(url string) Result {
	start := time.Now()
	resp, err := c.client.Get(url)
	elapsed := time.Since(start)
	if err != nil {
		return Result{URL: url, Duration: elapsed, Err: err}
	}
	defer resp.Body.Close()
	return Result{URL: url, StatusCode: resp.StatusCode, Status: resp.Status, Duration: elapsed}
}

// Stats — спільна статистика, захищена м'ютексом.
type Stats struct {
	mu      sync.Mutex
	Total   int
	Success int // статус 200
	NonOK   int // відповідь отримано, але статус != 200
	Errors  int // помилки з'єднання
}

func (s *Stats) Record(r Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Total++
	switch {
	case r.Err != nil:
		s.Errors++
	case r.StatusCode == http.StatusOK:
		s.Success++
	default:
		s.NonOK++
	}
}

func printResult(r Result) {
	if r.Err != nil {
		fmt.Printf("-> %s: Помилка (%v) (час: %v)\n", r.URL, r.Err, r.Duration.Round(time.Millisecond))
		return
	}
	fmt.Printf("-> %s: Status %s (час: %v)\n", r.URL, r.Status, r.Duration.Round(time.Millisecond))
}

// runSequential — Рівень 1: послідовне опитування.
func runSequential(c Checker, urls []string) time.Duration {
	start := time.Now()
	for _, u := range urls {
		printResult(c.Check(u))
	}
	return time.Since(start)
}

// runConcurrent — Рівень 2: горутини + WaitGroup + Mutex.
func runConcurrent(c Checker, urls []string, stats *Stats) time.Duration {
	start := time.Now()
	var wg sync.WaitGroup
	var printMu sync.Mutex // щоб рядки виводу не перемішувалися

	for _, u := range urls {
		wg.Add(1) // до запуску горутини
		go func(url string) {
			defer wg.Done()
			res := c.Check(url)
			stats.Record(res)
			printMu.Lock()
			printResult(res)
			printMu.Unlock()
		}(u) // передаємо URL як аргумент
	}

	wg.Wait()
	return time.Since(start)
}

func main() {
	urls := []string{
		"https://go.dev",
		"https://github.com",
		"https://httpbin.org/status/404",
		"https://invalid-fake-host-test.org",
	}

	var checker Checker = NewHTTPChecker(5 * time.Second)

	fmt.Println("=== HTTP URL Health Checker ===")

	fmt.Println("\n[Послідовна перевірка адрес...]")
	seqTime := runSequential(checker, urls)

	fmt.Println("\n[Конкурентна перевірка адрес...]")
	stats := &Stats{}
	conTime := runConcurrent(checker, urls, stats)

	fmt.Println("\n==========================================")
	fmt.Println("ПІДСУМКОВИЙ ЗВІТ МОНІТОРИНГУ:")
	fmt.Printf("- Усього перевірено адрес: %d\n", stats.Total)
	fmt.Printf("- Успішних відповідей (200): %d\n", stats.Success)
	fmt.Printf("- Інших HTTP-статусів: %d\n", stats.NonOK)
	fmt.Printf("- Мережевих помилок: %d\n", stats.Errors)
	fmt.Printf("- Час послідовно: %v\n", seqTime.Round(time.Millisecond))
	fmt.Printf("- Час конкурентно: %v", conTime.Round(time.Millisecond))
	if conTime > 0 {
		fmt.Printf(" (прискорення ≈ %.1fx)", float64(seqTime)/float64(conTime))
	}
	fmt.Println("\n==========================================")
}
