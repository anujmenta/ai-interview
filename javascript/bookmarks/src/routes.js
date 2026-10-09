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

module.exports = router;
