package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

type job struct {
	domain string
	ports  []string
}

func main() {
	// Define command-line arguments
	workerCount := flag.Int("workers", 10, "Number of concurrent workers")
	portsArg := flag.String("ports", "6443,8443,4443,443,4343,80,81,9443,8080,8081,8082,8000,10443,9080,8090", "Comma-separated list of ports to probe")
	timeout := flag.Duration("timeout", 3*time.Second, "Timeout for HTTP requests")
	flag.Parse()

	// Parse the list of ports
	ports := strings.Split(*portsArg, ",")

	// Channels for job distribution and results
	jobs := make(chan job)
	wg := &sync.WaitGroup{}
	client := &fasthttp.Client{
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
		// MaxConnsPerHost:     *workerCount,
	}

	// Start workers
	for i := 0; i < *workerCount; i++ {
		wg.Add(1)
		go worker(jobs, wg, client, i, timeout)
	}

	// Read domains from stdin
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		domain := scanner.Text()
		if domain == "" {
			continue
		}
		// fmt.Printf("New job %s:%s\n", domain, ports)
		jobs <- job{domain: domain, ports: ports}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading from stdin: %v", err)
	}

	// Close the jobs channel and wait for workers to finish
	close(jobs)
	wg.Wait()
}

func worker(jobs <-chan job, wg *sync.WaitGroup, client *fasthttp.Client, i int, timeout *time.Duration) {
	// defer func() {
	// 	fmt.Printf("Stopped worker %d\n", i)
	// }()
	defer wg.Done()
	// fmt.Printf("Started worker %d\n", i)
	status := 0

	for j := range jobs {
		for _, port := range j.ports {
			// fmt.Printf("Job %s:%s\n", j.domain, port)
			// Try HTTPS first
			if strings.HasPrefix(j.domain, "http") {
				httpsURL := fmt.Sprintf("%s:%s", j.domain, port)
				status = isAlive(client, httpsURL, timeout)
				if status != 0 {
					fmt.Printf("%s %d\n", httpsURL, status)
					continue
				}
			} else {
				httpsURL := fmt.Sprintf("https://%s:%s", j.domain, port)
				status = isAlive(client, httpsURL, timeout)
				if status != 0 {
					fmt.Printf("%s %d\n", httpsURL, status)
					continue
				}

				// Fallback to HTTP
				httpURL := fmt.Sprintf("http://%s:%s", j.domain, port)
				status = isAlive(client, httpURL, timeout)
				if status != 0 {
					fmt.Printf("%s %d\n", httpURL, status)
				}
			}
		}
	}
}

func isAlive(client *fasthttp.Client, url string, timeout *time.Duration) int {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// fmt.Printf("Set %s\n", url)
	req.SetRequestURI(url)
	req.Header.SetMethod("GET")

	err := client.DoTimeout(req, resp, *timeout)
	if err != nil {
		// fmt.Printf("Error %s\n", err)
		return 0
	}
	// fmt.Printf("Finished %s\n", url)

	return resp.StatusCode()
}
