from db import get_conn


def list_bookmarks():
    with get_conn() as conn:
        rows = conn.execute("SELECT * FROM bookmarks ORDER BY id").fetchall()
    return [dict(r) for r in rows]


def get_bookmark(bookmark_id):
    with get_conn() as conn:
        row = conn.execute("SELECT * FROM bookmarks WHERE id = ?", (bookmark_id,)).fetchone()
    return dict(row) if row else None


def create_bookmark(url, title, description=""):
    with get_conn() as conn:
        cur = conn.execute(
            "INSERT INTO bookmarks (url, title, description) VALUES (?, ?, ?)",
            (url, title, description),
        )
        bookmark_id = cur.lastrowid
    return get_bookmark(bookmark_id)


def update_bookmark(bookmark_id, url, title, description):
    with get_conn() as conn:
        cur = conn.execute(
            "UPDATE bookmarks SET url = ?, title = ?, description = ? WHERE id = ?",
            (url, title, description, bookmark_id),
        )
        if cur.rowcount == 0:
            return None
    return get_bookmark(bookmark_id)


def delete_bookmark(bookmark_id):
    with get_conn() as conn:
        cur = conn.execute("DELETE FROM bookmarks WHERE id = ?", (bookmark_id,))
    return cur.rowcount > 0
