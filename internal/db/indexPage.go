package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func IndexPage(ctx context.Context, wordCounts map[string]int, url, title, content string, totalWords int) error {
	tx, err := Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id int

	err = tx.QueryRow(ctx,
		`INSERT INTO pages (url, title, content, total_words)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (url) DO UPDATE SET content = EXCLUDED.content, title = EXCLUDED.title, total_words = EXCLUDED.total_words
		 RETURNING id`,
		url, title, content, totalWords,
	).Scan(&id)

	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for word, count := range wordCounts {
		batch.Queue(
			`INSERT INTO inverted_index (word, page_id, freq)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (word, page_id) DO UPDATE SET freq = EXCLUDED.freq`,
			word, id, count,
		)
	}
	br := tx.SendBatch(ctx, batch)
	for range wordCounts {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return err
		}
	}
	if err := br.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
