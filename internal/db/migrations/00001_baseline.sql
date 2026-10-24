-- +goose Up
CREATE TABLE IF NOT EXISTS pages (
    id SERIAL PRIMARY KEY,
    url TEXT UNIQUE NOT NULL,
    title TEXT,
    content TEXT,
    total_words INT,
    crawled_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inverted_index (
    word TEXT NOT NULL,
    page_id INT REFERENCES pages(id) ON DELETE CASCADE,
    freq INT NOT NULL,
    PRIMARY KEY (word, page_id)
);

CREATE INDEX IF NOT EXISTS idx_inverted_index_word
ON inverted_index(word);

-- +goose Down
DROP TABLE IF EXISTS inverted_index;
DROP TABLE IF EXISTS pages;