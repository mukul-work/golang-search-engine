package crawler

import (
	"fmt"
	"log"
	"sync"
)

func fetchUrl(url string, workerNum int) ([]string, error) {
	htmlBody, contentType, statusCode, err := GetPageData(url)
	if err != nil {
		err = fmt.Errorf("Error while fetching the page data for '%s': %v\n", url, err)
		return nil, err
	}
	fmt.Printf("Worker: %d Trying to visit: %s\n", workerNum, url)
	fmt.Printf("Worker: %d Content Type: %s\n", workerNum, contentType)
	fmt.Printf("Worker: %d Status Code: %d\n", workerNum, statusCode)

	links, err := GetHTMLData(htmlBody, url)
	if err != nil {
		return nil, fmt.Errorf("Problem while parsing the HTML: %w", err)
	}
	return links, err

}

func Worker(workerNum int, f *Frontier, wg *sync.WaitGroup, v *VisitedSet, rc *RobotCache) {
	for url := range f.out {
		Allowed, err := rc.Allowed(url)
		if err != nil {
			fmt.Printf("Worker: %d Error while wroking with robots.txt: %v", workerNum, err)
			wg.Done()
			continue
		}

		if !Allowed {
			fmt.Printf("Worker: %d Skipping (robots.txt disallows): %s\n", workerNum, url)
			wg.Done()
			continue
		}

		links, err := fetchUrl(url, workerNum)
		if err != nil {
			log.Printf("Worker: %d Problem with fetching or parsing the URL: %v\n", workerNum, err)
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
