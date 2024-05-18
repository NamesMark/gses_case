-- Add migration script here
CREATE TABLE IF NOT EXISTS subscription (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME,
    email TEXT
);