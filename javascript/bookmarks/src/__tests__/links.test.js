const path = require('path');
const fs = require('fs');
const request = require('supertest');
const express = require('express');
const Database = require('better-sqlite3');

describe('Shareable Links Feature', () => {
  let app;
  let db;
  const testDbPath = path.join(__dirname, 'test-links.db');

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

      CREATE TABLE IF NOT EXISTS links (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        bookmark_id INTEGER NOT NULL,
        token TEXT NOT NULL UNIQUE,
        expires_at TEXT NOT NULL,
        revoked INTEGER DEFAULT 0,
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (bookmark_id) REFERENCES bookmarks(id) ON DELETE CASCADE
      );
    `);

    process.env.DB_PATH = testDbPath;

    // Setup app
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
      deleteBookmark: (id) => db.prepare('DELETE FROM bookmarks WHERE id = ?').run(id).changes > 0,
    };

    // Link models
    const generateToken = () => require('crypto').randomBytes(32).toString('hex');

    const linkModels = {
      createLink: (bookmarkId, expiryHours = 24) => {
        if (expiryHours > 24) {
          return { error: 'expiryHours cannot exceed 24 hours' };
        }
        const bookmark = bookmarkModels.getBookmark(bookmarkId);
        if (!bookmark) {
          return { error: 'bookmark not found' };
        }
        const existingLink = db
          .prepare('SELECT * FROM links WHERE bookmark_id = ? AND revoked = 0')
          .get(bookmarkId);
        if (existingLink) {
          return { error: 'a shareable link already exists for this bookmark, revoke it first' };
        }
        const token = generateToken();
        const expiresAt = new Date(Date.now() + expiryHours * 60 * 60 * 1000).toISOString();
        const info = db
          .prepare('INSERT INTO links (bookmark_id, token, expires_at) VALUES (?, ?, ?)')
          .run(bookmarkId, token, expiresAt);
        return linkModels.getLink(info.lastInsertRowid);
      },
      getLink: (linkId) => db.prepare('SELECT * FROM links WHERE id = ?').get(linkId) || null,
      getLinkByToken: (token) => {
        const link = db.prepare('SELECT * FROM links WHERE token = ?').get(token) || null;
        if (!link) return null;
        if (link.revoked) return null;
        const now = new Date();
        const expiresAt = new Date(link.expires_at);
        if (now > expiresAt) return null;
        return link;
      },
      getBookmarkByLink: (token) => {
        const link = linkModels.getLinkByToken(token);
        if (!link) return null;
        return bookmarkModels.getBookmark(link.bookmark_id);
      },
      getLinkForBookmark: (bookmarkId) =>
        db.prepare('SELECT * FROM links WHERE bookmark_id = ? AND revoked = 0').get(bookmarkId) || null,
      revokeLink: (linkId) => {
        const info = db.prepare('UPDATE links SET revoked = 1 WHERE id = ?').run(linkId);
        return info.changes > 0;
      },
    };

    // Routes
    const bookmarkRouter = express.Router();

    bookmarkRouter.post('/bookmarks', (req, res) => {
      const { url, title, description } = req.body;
      if (!url || !title) return res.status(400).json({ error: 'url and title required' });
      const bookmark = bookmarkModels.createBookmark(url, title, description);
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

    // Link routes
    bookmarkRouter.post('/bookmarks/:id/links', (req, res) => {
      const bookmarkId = Number(req.params.id);
      const { expiryHours } = req.body || {};

      if (expiryHours && typeof expiryHours !== 'number') {
        return res.status(400).json({ error: 'expiryHours must be a number' });
      }

      const result = linkModels.createLink(bookmarkId, expiryHours || 24);

      if (result.error) {
        if (result.error === 'bookmark not found') {
          return res.status(404).json({ error: result.error });
        }
        return res.status(400).json({ error: result.error });
      }

      const link = result;
      res.status(201).json({
        id: link.id,
        token: link.token,
        bookmark_id: link.bookmark_id,
        expires_at: link.expires_at,
        created_at: link.created_at,
        share_url: `/share/${link.token}`,
      });
    });

    bookmarkRouter.get('/bookmarks/:id/links', (req, res) => {
      const bookmarkId = Number(req.params.id);
      const bookmark = bookmarkModels.getBookmark(bookmarkId);
      if (!bookmark) {
        return res.status(404).json({ error: 'bookmark not found' });
      }

      const link = linkModels.getLinkForBookmark(bookmarkId);
      if (!link) {
        return res.json({ activeLink: null });
      }

      res.json({
        activeLink: {
          id: link.id,
          token: link.token,
          expires_at: link.expires_at,
          created_at: link.created_at,
          share_url: `/share/${link.token}`,
        },
      });
    });

    bookmarkRouter.delete('/links/:id', (req, res) => {
      const linkId = Number(req.params.id);
      if (!linkModels.revokeLink(linkId)) {
        return res.status(404).json({ error: 'link not found' });
      }
      res.status(204).end();
    });

    bookmarkRouter.get('/share/:token', (req, res) => {
      const bookmark = linkModels.getBookmarkByLink(req.params.token);
      if (!bookmark) {
        return res.status(404).json({ error: 'link not found, expired, or revoked' });
      }
      res.json(bookmark);
    });

    app.use(bookmarkRouter);
  });

  beforeEach(() => {
    db.exec('DELETE FROM links; DELETE FROM bookmarks;');
  });

  afterAll(() => {
    db.close();
    if (fs.existsSync(testDbPath)) {
      fs.unlinkSync(testDbPath);
    }
  });

  describe('Link Creation', () => {
    it('should create a shareable link with default 24h expiry', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const link = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      expect(link.body.token).toBeDefined();
      expect(link.body.share_url).toContain('/share/');
      expect(link.body.expires_at).toBeDefined();
    });

    it('should create a link with custom expiry hours', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const link = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({ expiryHours: 12 })
        .expect(201);

      expect(link.body.token).toBeDefined();
      const expiresIn = new Date(link.body.expires_at) - new Date();
      expect(expiresIn).toBeLessThan(13 * 60 * 60 * 1000); // Less than 13 hours
      expect(expiresIn).toBeGreaterThan(11 * 60 * 60 * 1000); // More than 11 hours
    });

    it('should reject expiry > 24 hours', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const res = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({ expiryHours: 48 })
        .expect(400);

      expect(res.body.error).toContain('cannot exceed 24 hours');
    });

    it('should reject non-existent bookmark', async () => {
      const res = await request(app)
        .post('/bookmarks/999/links')
        .send({})
        .expect(404);

      expect(res.body.error).toBe('bookmark not found');
    });

    it('should reject invalid expiryHours type', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const res = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({ expiryHours: 'twelve' })
        .expect(400);

      expect(res.body.error).toContain('must be a number');
    });
  });

  describe('One Active Link Per Bookmark', () => {
    it('should error when trying to create second link', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      const res = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(400);

      expect(res.body.error).toContain('already exists');
    });

    it('should allow creating new link after revoking old one', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const link1 = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      await request(app).delete(`/links/${link1.body.id}`).expect(204);

      const link2 = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      expect(link2.body.id).not.toBe(link1.body.id);
      expect(link2.body.token).not.toBe(link1.body.token);
    });

    it('should maintain only one active link across multiple creations', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      // Create, revoke, create cycle
      const link1 = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      await request(app).delete(`/links/${link1.body.id}`).expect(204);

      const link2 = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      await request(app).delete(`/links/${link2.body.id}`).expect(204);

      const link3 = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      const activeLink = await request(app)
        .get(`/bookmarks/${book.body.id}/links`)
        .expect(200);

      expect(activeLink.body.activeLink.id).toBe(link3.body.id);
    });
  });

  describe('Link Access', () => {
    it('should access bookmark via public link', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://secret.com', title: 'Secret' })
        .expect(201);

      const link = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      const res = await request(app)
        .get(`/share/${link.body.token}`)
        .expect(200);

      expect(res.body.title).toBe('Secret');
      expect(res.body.url).toBe('https://secret.com');
    });

    it('should reject invalid token', async () => {
      const res = await request(app)
        .get('/share/invalid-token-xyz')
        .expect(404);

      expect(res.body.error).toContain('not found');
    });

    it('should reject revoked link', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const link = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      await request(app).delete(`/links/${link.body.id}`).expect(204);

      const res = await request(app)
        .get(`/share/${link.body.token}`)
        .expect(404);

      expect(res.body.error).toContain('revoked');
    });
  });

  describe('Link Management', () => {
    it('should get active link for bookmark', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const link = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      const res = await request(app)
        .get(`/bookmarks/${book.body.id}/links`)
        .expect(200);

      expect(res.body.activeLink).toBeDefined();
      expect(res.body.activeLink.id).toBe(link.body.id);
      expect(res.body.activeLink.token).toBe(link.body.token);
    });

    it('should return null when no active link exists', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const res = await request(app)
        .get(`/bookmarks/${book.body.id}/links`)
        .expect(200);

      expect(res.body.activeLink).toBeNull();
    });

    it('should revoke a link', async () => {
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://example.com', title: 'Test' })
        .expect(201);

      const link = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(201);

      await request(app).delete(`/links/${link.body.id}`).expect(204);

      const res = await request(app)
        .get(`/bookmarks/${book.body.id}/links`)
        .expect(200);

      expect(res.body.activeLink).toBeNull();
    });

    it('should error when revoking non-existent link', async () => {
      const res = await request(app).delete('/links/999').expect(404);

      expect(res.body.error).toBe('link not found');
    });

    it('should error getting links for non-existent bookmark', async () => {
      const res = await request(app).get('/bookmarks/999/links').expect(404);

      expect(res.body.error).toBe('bookmark not found');
    });
  });

  describe('Integration: Full Workflow', () => {
    it('should complete full shareable link lifecycle', async () => {
      // 1. Create bookmark
      const book = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://private.com', title: 'Private Doc' })
        .expect(201);

      // 2. Create link
      const link1 = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({ expiryHours: 6 })
        .expect(201);

      // 3. Share link - public access works
      await request(app)
        .get(`/share/${link1.body.token}`)
        .expect(200);

      // 4. Try to create another - fails
      await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({})
        .expect(400);

      // 5. Revoke link
      await request(app).delete(`/links/${link1.body.id}`).expect(204);

      // 6. Old link no longer works
      await request(app)
        .get(`/share/${link1.body.token}`)
        .expect(404);

      // 7. Create new link
      const link2 = await request(app)
        .post(`/bookmarks/${book.body.id}/links`)
        .send({ expiryHours: 24 })
        .expect(201);

      // 8. New link works
      const shared = await request(app)
        .get(`/share/${link2.body.token}`)
        .expect(200);

      expect(shared.body.title).toBe('Private Doc');
    });

    it('should handle multiple bookmarks with independent links', async () => {
      const book1 = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://a.com', title: 'A' })
        .expect(201);

      const book2 = await request(app)
        .post('/bookmarks')
        .send({ url: 'https://b.com', title: 'B' })
        .expect(201);

      const link1 = await request(app)
        .post(`/bookmarks/${book1.body.id}/links`)
        .send({})
        .expect(201);

      const link2 = await request(app)
        .post(`/bookmarks/${book2.body.id}/links`)
        .send({})
        .expect(201);

      // Both links work
      const res1 = await request(app).get(`/share/${link1.body.token}`).expect(200);
      const res2 = await request(app).get(`/share/${link2.body.token}`).expect(200);

      expect(res1.body.title).toBe('A');
      expect(res2.body.title).toBe('B');

      // Revoke link1
      await request(app).delete(`/links/${link1.body.id}`).expect(204);

      // link1 dead, link2 still works
      await request(app).get(`/share/${link1.body.token}`).expect(404);
      await request(app).get(`/share/${link2.body.token}`).expect(200);
    });
  });
});
