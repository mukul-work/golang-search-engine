package crawler

import (
	"strings"

	"golang.org/x/net/html"
)

var links []string

func traverseNodes(node *html.Node) {
	if node.Type == html.ElementNode && node.Data == "a" {
		for _, attr := range node.Attr {
			if attr.Key == "href" {

				links = append(links, attr.Val)
				break
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		traverseNodes(child)
	}
}

func GetHTMLData(htmlString string) ([]string, error) {
	doc, err := html.Parse(strings.NewReader(htmlString))
	if err != nil {
		return nil, err
	}
	traverseNodes(doc)
	return links, err

}
