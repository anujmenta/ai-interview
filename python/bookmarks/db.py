import os
import sqlite3

DB_PATH = os.environ.get("DB_PATH", "bookmarks.db")


def get_conn():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys = ON")
    return conn


def init_db():
    with open(os.path.join(os.path.dirname(__file__), "schema.sql")) as f:
        schema = f.read()
    with get_conn() as conn:
        conn.executescript(schema)
