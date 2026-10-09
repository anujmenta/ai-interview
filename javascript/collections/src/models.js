const db = require('./db');
const bookmarkModels = require('../../bookmarks/src/models');

function getCollection(collectionId) {
  const collection = db.prepare('SELECT id, title, bookmarks, created_at FROM collections WHERE id = ?').get(collectionId);
  if (!collection) return null;

  let bookmarkIds = [];
  if (collection.bookmarks) {
    try {
      bookmarkIds = JSON.parse(collection.bookmarks);
    } catch {
      bookmarkIds = [];
    }
  }

  const bookmarkObjects = bookmarkIds
    .map(id => bookmarkModels.getBookmark(id))
    .filter(b => b !== null);

  return {
    id: collection.id,
    title: collection.title,
    bookmarks: bookmarkObjects,
    created_at: collection.created_at,
  };
}

function createCollection(title) {
  try {
    const info = db
      .prepare('INSERT INTO collections (title, bookmarks) VALUES (?, ?)')
      .run(title, JSON.stringify([]));
    return getCollection(info.lastInsertRowid);
  } catch (err) {
    if (err.message.includes('UNIQUE constraint failed')) {
      return null;
    }
    throw err;
  }
}

function updateCollection(collectionId, title, bookmarksArray) {
  const collection = db.prepare('SELECT * FROM collections WHERE id = ?').get(collectionId);
  if (!collection) return null;

  const newTitle = title !== undefined ? title : collection.title;
  const newBookmarks = bookmarksArray !== undefined ? JSON.stringify(bookmarksArray) : collection.bookmarks;

  try {
    const info = db
      .prepare('UPDATE collections SET title = ?, bookmarks = ? WHERE id = ?')
      .run(newTitle, newBookmarks, collectionId);
    return info.changes === 0 ? null : getCollection(collectionId);
  } catch (err) {
    if (err.message.includes('UNIQUE constraint failed')) {
      return null;
    }
    throw err;
  }
}

module.exports = { getCollection, createCollection, updateCollection };
