package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Store) IndexPage(ctx context.Context, wordCounts map[string]int, url, title, content string, totalWords int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id int
	err = tx.QueryRow(ctx,
		`INSERT INTO pages (url, title, content, total_words)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (url) DO UPDATE SET
		     content = EXCLUDED.content,
		     title = EXCLUDED.title,
		     total_words = EXCLUDED.total_words,
		     crawled_at = NOW()
		 RETURNING id`,
		url, title, content, totalWords,
	).Scan(&id)
	if err != nil {
		return err
	}

	// A re-crawled page may have lost words. Drop its old entries first so it
	// cannot keep matching words it no longer contains. The page_id index
	// (migration 00002) makes this a lookup, not a table scan.
	if _, err := tx.Exec(ctx, `DELETE FROM inverted_index WHERE page_id = $1`, id); err != nil {
		return err
	}

	if len(wordCounts) > 0 { // a page with no words has nothing to insert
		batch := &pgx.Batch{}
		for word, count := range wordCounts {
			batch.Queue(
				`INSERT INTO inverted_index (word, page_id, freq) VALUES ($1, $2, $3)`,
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
	}
	return tx.Commit(ctx)
}
