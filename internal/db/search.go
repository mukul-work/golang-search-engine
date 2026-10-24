package db

import (
	"context"

	"github.com/mukul-work/golang-search-engine/internal/models"
)

const searchQuery = `
WITH stats AS (
    SELECT COUNT(*)::float8 AS n, AVG(total_words)::float8 AS avgdl
    FROM pages
    WHERE total_words > 0
),
df AS (
    SELECT word, COUNT(*)::float8 AS doc_freq
    FROM inverted_index
    WHERE word = ANY($1)
    GROUP BY word
)
SELECT COALESCE(p.title, ''),
       p.url,
       COALESCE(LEFT(p.content, 200), ''),
       SUM(
         LN(1 + (stats.n - df.doc_freq + 0.5) / (df.doc_freq + 0.5))            -- IDF
         * (i.freq::float8 * (1.2 + 1))                                          -- freq * (k1+1)
         / (i.freq + 1.2 * (1 - 0.75 + 0.75 * p.total_words / stats.avgdl))      -- freq + k1*(1-b+b*|d|/avgdl)
       ) AS score,
       (COUNT(*) OVER ())::int AS total                                          -- every match, before LIMIT/OFFSET
FROM inverted_index i
JOIN pages p ON p.id = i.page_id
JOIN df ON df.word = i.word
CROSS JOIN stats
WHERE i.word = ANY($1)
  AND p.total_words > 0
GROUP BY p.id
HAVING COUNT(*) = $2
ORDER BY score DESC, p.id                                                        -- p.id breaks ties: a total order
LIMIT $3 OFFSET $4`

// SearchPages returns one page of results plus the total number of matches.
// Past the last match no rows come back, so total is 0 in that case.
func (s *Store) SearchPages(ctx context.Context, terms []string, limit, offset int) ([]models.Result, int, error) {
	rows, err := s.pool.Query(ctx, searchQuery, terms, len(terms), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []models.Result
	total := 0
	for rows.Next() {
		var r models.Result
		if err := rows.Scan(&r.Title, &r.URL, &r.Snippet, &r.Score, &total); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}
