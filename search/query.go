package search

import (
	"context"

	"github.com/mukul-work/golang-search-engine/db"
	"github.com/mukul-work/golang-search-engine/indexer"
	"github.com/mukul-work/golang-search-engine/models"
)

const query = `WITH n AS (
    SELECT COUNT(*)::float8 AS total FROM pages          -- N
),
df AS (
    SELECT word, COUNT(*)::float8 AS df                  -- df per query term
    FROM inverted_index
    WHERE word = ANY($1)
    GROUP BY word
)
SELECT COALESCE(p.title, ''),
       p.url,
       COALESCE(LEFT(p.content, 200), ''),
       SUM( (i.freq::float8 / p.total_words)             -- TF
            * LN(1 + n.total / df.df) ) AS score         -- × IDF
FROM inverted_index i
JOIN pages p  ON p.id = i.page_id
JOIN df       ON df.word = i.word
CROSS JOIN n
WHERE i.word = ANY($1)
  AND p.total_words > 0
GROUP BY p.id
HAVING COUNT(*) = $2                                     -- page must contain ALL terms
ORDER BY score DESC
LIMIT $3`

func Query(ctx context.Context, q string, limit int) ([]models.Result, error) {
	countsOfWords := indexer.Tokenize(q) // q = web-crawler/search-engine  web 1, crawler 1 search 1 engine 1
	if len(countsOfWords) == 0 {
		return nil, nil
	}
	// fmt.Printf("countOfWords: %d", len(countsOfWords))
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil

}
