package crawler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/mukul-work/golang-search-engine/db"
	"github.com/mukul-work/golang-search-engine/indexer"
	"golang.org/x/net/html"
)

func fetchUrl(url string, workerNum int) ([]string, error) {
	htmlBody, contentType, statusCode, err := GetPageData(url)
	if err != nil {
		err = fmt.Errorf("Error while fetching the page data for '%s': %v\n", url, err)
		return nil, err
	}

	if !strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		fmt.Printf("Worker: %d Skipping non-HTML (%s): %s\n", workerNum, contentType, url)
		return nil, nil
	}

	fmt.Printf("Worker: %d Trying to visit: %s\n", workerNum, url)
	fmt.Printf("Worker: %d Content Type: %s\n", workerNum, contentType)
	fmt.Printf("Worker: %d Status Code: %d\n", workerNum, statusCode)

	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return nil, fmt.Errorf("Problem while parsing the HTML before passing it into the GetHTMLData: %w", err)
	}

	links, err := GetHTMLData(doc, url)
	if err != nil {
		return nil, fmt.Errorf("Problem while parsing the HTML: %w", err)
	}

	text, err := GetPageText(doc)
	if err != nil {
		return nil, fmt.Errorf("Problem extracting text: %w", err)
	}

	wordCounts := indexer.Tokenize(text)
	total := 0
	for _, c := range wordCounts {
		total += c
	}
	title, err := GetPageTitle(doc)
	if err != nil {
		return nil, fmt.Errorf("Problem extracting title: %w", err)
	}

	if err := db.IndexPage(context.Background(), wordCounts, url, title, text, total); err != nil {
		return nil, fmt.Errorf("Failed to insert into the database: %w", err)
	}

	// pageID, err := db.InsertPage(context.Background(), url, title, text, total)
	// if err != nil {
	// 	return nil, fmt.Errorf("Failed to insert page: %w", err)
	// }

	// if err := db.InsertWordEntries(context.Background(), pageID, wordCounts); err != nil {
	// 	return nil, fmt.Errorf("Failed to insert word entries: %w", err)
	// }

	return links, nil

}

func Worker(workerNum int, f *Frontier, wg *sync.WaitGroup, v *VisitedSet, rc *RobotCache, hl *HostLimiter) {
	for url := range f.out {
		Allowed, err := rc.Allowed(url)
		if err != nil {
			fmt.Printf("Worker: %d Error while working with robots.txt: %v", workerNum, err)
			wg.Done()
			continue
		}

		if !Allowed {
			fmt.Printf("Worker: %d Skipping (robots.txt disallows): %s\n", workerNum, url)
			wg.Done()
			continue
		}
		if err := hl.Wait(url); err != nil { // <-- rate limit, blocks per host
			fmt.Printf("Worker: %d Rate limiter error: %v\n", workerNum, err)
			wg.Done()
			continue
		}
		links, err := fetchUrl(url, workerNum)
		if err != nil {
			log.Printf("Worker: %d %v\n", workerNum, err)
			wg.Done()
			continue

		}

		fmt.Printf("Worker: %d List of links for '%s':\n\n", workerNum, url)
		for _, link := range links {
			fmt.Printf("Worker: %d Link before checking in the visited Set: %s\n", workerNum, link)
			if !v.IsVisited(link) { // check for already visited URL
				fmt.Printf("Worker: %d Link pushed to visited set\n\n", workerNum)
				wg.Add(1)
				f.Push(link)
			} else {
				fmt.Printf("Worker: %d Link already in visited set\n\n", workerNum)
			}
		}
		fmt.Printf("Worker: %d Visited URL: %s\n\n\n\n", workerNum, url)
		wg.Done() // URL parsed
	}

}
