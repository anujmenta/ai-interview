const db = require('./db');

function listBookmarks() {
  return db.prepare('SELECT * FROM bookmarks ORDER BY id').all();
}

function getBookmark(id) {
  return db.prepare('SELECT * FROM bookmarks WHERE id = ?').get(id) || null;
}

function createBookmark(url, title, description = '') {
  const info = db
    .prepare('INSERT INTO bookmarks (url, title, description) VALUES (?, ?, ?)')
    .run(url, title, description);
  return getBookmark(info.lastInsertRowid);
}

function updateBookmark(id, url, title, description) {
  const info = db
    .prepare('UPDATE bookmarks SET url = ?, title = ?, description = ? WHERE id = ?')
    .run(url, title, description, id);
  return info.changes === 0 ? null : getBookmark(id);
}

function deleteBookmark(id) {
  return db.prepare('DELETE FROM bookmarks WHERE id = ?').run(id).changes > 0;
}

module.exports = { listBookmarks, getBookmark, createBookmark, updateBookmark, deleteBookmark };
