-- Add up migration script here
CREATE TABLE IF NOT EXISTS usd_uah_rate (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME,
    value REAL
);