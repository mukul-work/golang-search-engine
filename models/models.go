package models

import "time"

type Urls struct {
	URL string `json:"url"`
}

type Page struct {
	ID         int
	Url        string
	Title      string
	Content    string
	TotalWords int
	CrawledAt  time.Time
}

type WordEntry struct {
	Word      string
	PageID    int
	Frequency int
}

type Result struct {
	Title   string
	URL     string
	Snippet string // first 200 characters of the page text
	Score   float64
}
