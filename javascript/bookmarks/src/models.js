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

function generateToken() {
  return require('crypto').randomBytes(32).toString('hex');
}

function createLink(bookmarkId, expiryHours = 24) {
  if (expiryHours > 24) {
    return { error: 'expiryHours cannot exceed 24 hours' };
  }

  const bookmark = getBookmark(bookmarkId);
  if (!bookmark) {
    return { error: 'bookmark not found' };
  }

  // Check if an active link already exists
  const existingLink = getLinkForBookmark(bookmarkId);
  if (existingLink) {
    return { error: 'a shareable link already exists for this bookmark, revoke it first' };
  }

  const token = generateToken();
  const expiresAt = new Date(Date.now() + expiryHours * 60 * 60 * 1000).toISOString();

  const info = db
    .prepare(
      'INSERT INTO links (bookmark_id, token, expires_at) VALUES (?, ?, ?)'
    )
    .run(bookmarkId, token, expiresAt);

  return getLink(info.lastInsertRowid);
}

function getLink(linkId) {
  return db.prepare('SELECT * FROM links WHERE id = ?').get(linkId) || null;
}

function getLinkByToken(token) {
  const link = db.prepare('SELECT * FROM links WHERE token = ?').get(token) || null;
  if (!link) return null;

  if (link.revoked) return null;

  const now = new Date();
  const expiresAt = new Date(link.expires_at);
  if (now > expiresAt) return null;

  return link;
}

function getBookmarkByLink(token) {
  const link = getLinkByToken(token);
  if (!link) return null;
  return getBookmark(link.bookmark_id);
}

function getLinkForBookmark(bookmarkId) {
  return db.prepare('SELECT * FROM links WHERE bookmark_id = ? AND revoked = 0').get(bookmarkId) || null;
}

function revokeLink(linkId) {
  const info = db.prepare('UPDATE links SET revoked = 1 WHERE id = ?').run(linkId);
  return info.changes > 0;
}

module.exports = {
  listBookmarks,
  getBookmark,
  createBookmark,
  updateBookmark,
  deleteBookmark,
  createLink,
  getLink,
  getLinkByToken,
  getBookmarkByLink,
  getLinkForBookmark,
  revokeLink,
};
