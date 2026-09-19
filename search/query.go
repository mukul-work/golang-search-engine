package search

import (
	"context"
	"fmt"

	"github.com/mukul-work/golang-search-engine/db"
	"github.com/mukul-work/golang-search-engine/indexer"
	"github.com/mukul-work/golang-search-engine/models"
)

const query = `SELECT COALESCE(p.title,''), p.url, COALESCE(LEFT(p.content,200),''), SUM(i.freq::float8 / p.total_words) AS score FROM inverted_index i JOIN pages p ON p.id = i.page_id WHERE i.word = ANY($1) AND p.total_words>0 GROUP BY p.id HAVING COUNT(*) = $2 ORDER BY score DESC limit $3`

func Query(ctx context.Context, q string, limit int) ([]models.Result, error) {
	countsOfWords := indexer.Tokenize(q) // q = web-crawler/search-engine  web 1, crawler 1 search 1 engine 1
	if len(countsOfWords) == 0 {
		return nil, nil
	}
	fmt.Printf("countOfWords: %d", len(countsOfWords))
	terms := make([]string, 0, len(countsOfWords))
	for term := range countsOfWords {
		terms = append(terms, term)
	}
	// fmt.Printf("terms=%q count=%d\n", terms, len(terms))
	rows, err := db.Pool.Query(ctx, query, terms, len(terms), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.Result
	for rows.Next() {
		var r models.Result
		if err := rows.Scan(&r.Title, &r.URL, &r.Snippet, &r.Score); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil

}
