package crawler

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

var links []string

func traverseNodes(node *html.Node, base *url.URL) {
	if node.Type == html.ElementNode && node.Data == "a" {
		for _, attr := range node.Attr {
			if attr.Key == "href" {
				href := attr.Val
				ref, err := url.Parse(href)
				if err != nil {
					continue // ignore the malformed URL -> gotta fix this later
				}
				resolved := base.ResolveReference(ref).String()
				links = append(links, resolved)
				break
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		traverseNodes(child, base)
	}
}

func GetHTMLData(htmlString string, baseUrl string) ([]string, error) {
	doc, err := html.Parse(strings.NewReader(htmlString))
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}
	traverseNodes(doc, base)
	return links, err

}
