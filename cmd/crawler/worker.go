package crawler

import (
	"fmt"
	"log"
	"sync"
)

func fetchUrl(url string) ([]string, error) {
	htmlBody, contentType, statusCode, err := GetPageData(url)
	if err != nil {
		fmt.Printf("Error while fetching the page data for %s: %v\n", url, err)
		return nil, err
	}
	fmt.Printf("Content Type: %s\n", contentType)
	fmt.Printf("Status Code: %d\n", statusCode)

	links, err := GetHTMLData(htmlBody, url)
	if err != nil {
		return nil, fmt.Errorf("Problem while parsing the HTML: %w", err)
	}
	return links, err

}

func Worker(f *Frontier, wg *sync.WaitGroup, v *VisitedSet) {
	for url := range f.out {
		links, err := fetchUrl(url)
		if err != nil {
			log.Fatalf("Problem with fetching or parsing the URL: %v", err)
			wg.Done()
			continue

		}

		for _, link := range links {
			if !v.IsVisited(link) { // check for already visited URL
				wg.Add(1)
				f.Push(link)
			}
		}
		fmt.Printf("Visited URL: %s", url)
		wg.Done() // URL parsed
	}

}
