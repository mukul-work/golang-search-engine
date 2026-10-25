-- +goose Up
CREATE INDEX IF NOT EXISTS idx_inverted_index_page_id ON inverted_index(page_id);
DROP INDEX IF EXISTS idx_inverted_index_word;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_inverted_index_word ON inverted_index(word);
DROP INDEX IF EXISTS idx_inverted_index_page_id;