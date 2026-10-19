package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func InsertWordEntries(ctx context.Context, pageID int, wordCounts map[string]int) error {
	batch := &pgx.Batch{}
	for word, count := range wordCounts {
		batch.Queue(
			`INSERT INTO inverted_index (word, page_id, freq)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (word, page_id) DO UPDATE SET freq = EXCLUDED.freq`,
			word, pageID, count,
		)
	}
	br := Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range wordCounts {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
