const path = require('path');
const fs = require('fs');
const request = require('supertest');
const express = require('express');
const Database = require('better-sqlite3');

describe('Collections & Bookmarks Integration Tests', () => {
  let app;
  let db;
  const testDbPath = path.join(__dirname, 'test-integration.db');

  beforeAll(() => {
    if (fs.existsSync(testDbPath)) {
      fs.unlinkSync(testDbPath);
    }
    db = new Database(testDbPath);
    db.pragma('foreign_keys = ON');
    db.exec(`
      CREATE TABLE IF NOT EXISTS bookmarks (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        url TEXT NOT NULL,
        title TEXT NOT NULL,
        description TEXT NOT NULL DEFAULT '',
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
      );

      CREATE TABLE IF NOT EXISTS collections (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL UNIQUE,
        bookmarks TEXT,
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
      );
    `);

    process.env.DB_PATH = testDbPath;

    // Setup app with routes
    app = express();
    app.use(express.json());

    // Bookmark models
    const bookmarkModels = {
      listBookmarks: () => db.prepare('SELECT * FROM bookmarks ORDER BY id').all(),
      getBookmark: (id) => db.prepare('SELECT * FROM bookmarks WHERE id = ?').get(id) || null,
      createBookmark: (url, title, description = '') => {
        const info = db
          .prepare('INSERT INTO bookmarks (url, title, description) VALUES (?, ?, ?)')
          .run(url, title, description);
        return bookmarkModels.getBookmark(info.lastInsertRowid);
      },
      updateBookmark: (id, url, title, description) => {
        const info = db
          .prepare('UPDATE bookmarks SET url = ?, title = ?, description = ? WHERE id = ?')
          .run(url, title, description, id);
        return info.changes === 0 ? null : bookmarkModels.getBookmark(id);
      },
      deleteBookmark: (id) => db.prepare('DELETE FROM bookmarks WHERE id = ?').run(id).changes > 0,
    };

    // Bookmark routes
    const bookmarkRouter = express.Router();
    const validateBookmark = (data) => {
      if (data === null || typeof data !== 'object' || Array.isArray(data)) {
        return 'body must be a JSON object';
      }
      for (const field of ['url', 'title']) {
        if (typeof data[field] !== 'string' || !data[field].trim()) {
          return `${field} is required`;
        }
      }
      if (data.description !== undefined && typeof data.description !== 'string') {
        return 'description must be a string';
      }
      return null;
    };

    bookmarkRouter.post('/bookmarks', (req, res) => {
      const error = validateBookmark(req.body);
      if (error) return res.status(400).json({ error });
      const { url, title, description } = req.body;
      const bookmark = bookmarkModels.createBookmark(url.trim(), title.trim(), description);
      res.status(201).json(bookmark);
    });

    bookmarkRouter.get('/bookmarks/:id', (req, res) => {
      const bookmark = bookmarkModels.getBookmark(Number(req.params.id));
      if (!bookmark) return res.status(404).json({ error: 'bookmark not found' });
      res.json(bookmark);
    });

    bookmarkRouter.delete('/bookmarks/:id', (req, res) => {
      if (!bookmarkModels.deleteBookmark(Number(req.params.id))) {
        return res.status(404).json({ error: 'bookmark not found' });
      }
      res.status(204).end();
    });

    // Collection models
    const getCollection = (collectionId) => {
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
    };

    const collectionModels = {
      getCollection,
      createCollection: (title) => {
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
      },
      updateCollection: (collectionId, title, bookmarksArray) => {
        const collection = db.prepare('SELECT * FROM collections WHERE id = ?').get(collectionId);
        if (!collection) return null;

        const newTitle = title !== undefined ? title : collection.title;
        const newBookmarks =
          bookmarksArray !== undefined ? JSON.stringify(bookmarksArray) : collection.bookmarks;

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
      },
    };

    // Collection routes
    const collectionRouter = express.Router();
    const validateCollection = (data) => {
      if (data === null || typeof data !== 'object' || Array.isArray(data)) {
        return 'body must be a JSON object';
      }
      if (data.title !== undefined) {
        if (typeof data.title !== 'string' || !data.title.trim()) {
          return 'title is required';
        }
      }
      if (data.bookmarks !== undefined) {
        if (!Array.isArray(data.bookmarks)) {
          return 'bookmarks must be an array';
        }
        const seen = new Set();
        for (const id of data.bookmarks) {
          if (!Number.isInteger(id)) {
            return 'bookmarks must contain only integers';
          }
          if (seen.has(id)) {
            return 'bookmarks must not contain duplicates';
          }
          seen.add(id);
          if (!bookmarkModels.getBookmark(id)) {
            return `bookmark ${id} not found`;
          }
        }
      }
      return null;
    };

    collectionRouter.post('/collections', (req, res) => {
      if (!req.body?.title) return res.status(400).json({ error: 'title is required' });
      const error = validateCollection(req.body);
      if (error) return res.status(400).json({ error });
      const collection = collectionModels.createCollection(req.body.title.trim());
      if (!collection) {
        return res.status(409).json({ error: 'collection title already exists' });
      }
      res.status(201).json(collection);
    });

    collectionRouter.get('/collections/:id', (req, res) => {
      const collection = collectionModels.getCollection(Number(req.params.id));
      if (!collection) return res.status(404).json({ error: 'collection not found' });
      res.json(collection);
    });

    collectionRouter.put('/collections/:id', (req, res) => {
      const error = validateCollection(req.body);
      if (error) return res.status(400).json({ error });
      const { title, bookmarks } = req.body;
      if (title === undefined && bookmarks === undefined) {
        return res.status(400).json({ error: 'title or bookmarks must be provided' });
      }
      const collection = collectionModels.updateCollection(Number(req.params.id), title, bookmarks);
      if (!collection) {
        const exist = collectionModels.getCollection(Number(req.params.id));
        if (!exist) return res.status(404).json({ error: 'collection not found' });
        return res.status(409).json({ error: 'collection title already exists' });
      }
      res.json(collection);
    });

    app.use(bookmarkRouter);
    app.use(collectionRouter);
  });

  beforeEach(() => {
    // Clear tables but keep schema
    db.exec('DELETE FROM collections; DELETE FROM bookmarks;');
  });

  afterAll(() => {
    db.close();
    if (fs.existsSync(testDbPath)) {
      fs.unlinkSync(testDbPath);
    }
  });

  describe('End-to-End: Bookmarks → Collections → Ordering', () => {
    it('should create bookmarks, create collection, and add bookmarks in order', async () => {
      // Create 5 bookmarks
      const bookmarks = [];
      for (let i = 1; i <= 5; i++) {
        const res = await request(app)
          .post('/bookmarks')
          .send({ url: `https://example.com/${i}`, title: `Bookmark ${i}` })
          .expect(201);
        bookmarks.push(res.body.id);
      }
      expect(bookmarks).toEqual([1, 2, 3, 4, 5]);

      // Create collection
      const collRes = await request(app)
        .post('/collections')
        .send({ title: 'My Collection' })
        .expect(201);
      expect(collRes.body.title).toBe('My Collection');
      expect(collRes.body.bookmarks).toEqual([]);

      // Add bookmarks in custom order
      const orderRes = await request(app)
        .put('/collections/1')
        .send({ bookmarks: [5, 3, 1, 4, 2] })
        .expect(200);
      expect(orderRes.body.bookmarks.map(b => b.id)).toEqual([5, 3, 1, 4, 2]);
    });
  });

  describe('End-to-End: Error Scenarios', () => {
    it('should reject duplicate collection titles', async () => {
      await request(app).post('/collections').send({ title: 'Unique' }).expect(201);
      const res = await request(app).post('/collections').send({ title: 'Unique' }).expect(409);
      expect(res.body.error).toBe('collection title already exists');
    });
  });

});
