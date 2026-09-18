CREATE TABLE pages (
    id SERIAL PRIMARY KEY,
    url TEXT UNIQUE NOT NULL,
    title TEXT,
    content TEXT,
    total_words INT,
    crawled_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE inverted_index (
    word TEXT NOT NULL,
    page_id INT REFERENCES pages(id) ON DELETE CASCADE,
    freq INT NOT NULL,
    PRIMARY KEY (word, page_id)
);

CREATE INDEX idx_inverted_index_word
ON inverted_index(word);