package config

const Schema = `
CREATE TABLE IF NOT EXISTS articles(
    id SERIAL PRIMARY KEY,
    author TEXT,
    title TEXT,
    description TEXT,
    content TEXT,
    url TEXT,
    image_url TEXT,
    category TEXT,
    publishedAt DATE
);
`
