package db

import "context"

func InsertPage(ctx context.Context, url, title, content string, totalWords int) (int, error) {
	var id int
	err := Pool.QueryRow(ctx,
		`INSERT INTO pages (url, title, content, total_words)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (url) DO UPDATE SET content = EXCLUDED.content, title = EXCLUDED.title, total_words = EXCLUDED.total_words
		 RETURNING id`,
		url, title, content, totalWords,
	).Scan(&id)
	return id, err
}
