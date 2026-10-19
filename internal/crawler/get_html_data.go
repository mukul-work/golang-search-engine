package crawler

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// var links []string --> this should not be global

func traverseNodes(node *html.Node, base *url.URL, links *[]string) {
	if node.Type == html.ElementNode && node.Data == "a" {
		for _, attr := range node.Attr {
			if attr.Key == "href" {
				href := attr.Val
				ref, err := url.Parse(href)
				if err != nil {
					continue // ignore the malformed URL -> gotta fix this later
				}
				resolved := base.ResolveReference(ref).String()
				*links = append(*links, resolved)
				break
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		traverseNodes(child, base, links)
	}
}

func GetHTMLData(node *html.Node, baseUrl string) ([]string, error) {
	var links []string
	// doc, err := html.Parse(strings.NewReader(htmlString))
	// if err != nil {
	// 	return nil, err
	// }
	base, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}
	traverseNodes(node, base, &links)
	return links, err
}

func extractText(sb *strings.Builder, node *html.Node) {
	if node.Type == html.ElementNode {
		switch node.Data {
		case "script", "style", "noscript":
			return
		}
	}
	if node.Type == html.TextNode {
		text := strings.TrimSpace(node.Data)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString("")
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		extractText(sb, child)
	}
}

func GetPageText(node *html.Node) (string, error) {
	// doc, err := html.Parse(strings.NewReader(htmlString))
	// if err != nil {
	// 	return "", err
	// }
	var sb strings.Builder
	extractText(&sb, node)
	return sb.String(), nil
}

func extractTitle(node *html.Node, title *string, found *bool) {
	if *found {
		return // stop traversing entirely once found
	}
	if node.Type == html.ElementNode && node.Data == "title" {
		if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
			*title = strings.TrimSpace(node.FirstChild.Data)
			*found = true
		}
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		extractTitle(child, title, found)
	}
}

func GetPageTitle(node *html.Node) (string, error) {
	// doc, err := html.Parse(strings.NewReader(htmlString))
	// if err != nil {
	// 	return "", err
	// }
	var title string
	found := false
	extractTitle(node, &title, &found)
	return title, nil
}
