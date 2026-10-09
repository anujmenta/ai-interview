const express = require('express');
const models = require('./models');

const router = express.Router();

function validateBookmark(data) {
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
}

router.get('/bookmarks', (req, res) => {
  res.json(models.listBookmarks());
});

router.get('/bookmarks/:id', (req, res) => {
  const bookmark = models.getBookmark(Number(req.params.id));
  if (!bookmark) return res.status(404).json({ error: 'bookmark not found' });
  res.json(bookmark);
});

router.post('/bookmarks', (req, res) => {
  const error = validateBookmark(req.body);
  if (error) return res.status(400).json({ error });
  const { url, title, description } = req.body;
  const bookmark = models.createBookmark(url.trim(), title.trim(), description);
  res.status(201).json(bookmark);
});

router.put('/bookmarks/:id', (req, res) => {
  const error = validateBookmark(req.body);
  if (error) return res.status(400).json({ error });
  const { url, title, description = '' } = req.body;
  const bookmark = models.updateBookmark(Number(req.params.id), url.trim(), title.trim(), description);
  if (!bookmark) return res.status(404).json({ error: 'bookmark not found' });
  res.json(bookmark);
});

router.delete('/bookmarks/:id', (req, res) => {
  if (!models.deleteBookmark(Number(req.params.id))) {
    return res.status(404).json({ error: 'bookmark not found' });
  }
  res.status(204).end();
});

router.post('/bookmarks/:id/links', (req, res) => {
  const bookmarkId = Number(req.params.id);
  const { expiryHours } = req.body || {};

  if (expiryHours && typeof expiryHours !== 'number') {
    return res.status(400).json({ error: 'expiryHours must be a number' });
  }

  const result = models.createLink(bookmarkId, expiryHours || 24);

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

router.get('/bookmarks/:id/links', (req, res) => {
  const bookmarkId = Number(req.params.id);
  const bookmark = models.getBookmark(bookmarkId);
  if (!bookmark) {
    return res.status(404).json({ error: 'bookmark not found' });
  }

  const link = models.getLinkForBookmark(bookmarkId);
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

router.delete('/links/:id', (req, res) => {
  const linkId = Number(req.params.id);
  if (!models.revokeLink(linkId)) {
    return res.status(404).json({ error: 'link not found' });
  }
  res.status(204).end();
});

router.get('/share/:token', (req, res) => {
  const bookmark = models.getBookmarkByLink(req.params.token);
  if (!bookmark) {
    return res.status(404).json({ error: 'link not found, expired, or revoked' });
  }
  res.json(bookmark);
});

module.exports = router;
