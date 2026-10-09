const express = require('express');
const models = require('./models');
const bookmarkModels = require('../../bookmarks/src/models');

const router = express.Router();

function validateCollection(data) {
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
}

router.post('/collections', (req, res) => {
  if (!req.body?.title) return res.status(400).json({ error: 'title is required' });
  const error = validateCollection(req.body);
  if (error) return res.status(400).json({ error });
  const collection = models.createCollection(req.body.title.trim());
  if (!collection) {
    return res.status(409).json({ error: 'collection title already exists' });
  }
  res.status(201).json(collection);
});

router.get('/collections/:id', (req, res) => {
  const collection = models.getCollection(Number(req.params.id));
  if (!collection) return res.status(404).json({ error: 'collection not found' });
  res.json(collection);
});

router.put('/collections/:id', (req, res) => {
  const error = validateCollection(req.body);
  if (error) return res.status(400).json({ error });
  const { title, bookmarks } = req.body;
  if (title === undefined && bookmarks === undefined) {
    return res.status(400).json({ error: 'title or bookmarks must be provided' });
  }
  const collection = models.updateCollection(Number(req.params.id), title, bookmarks);
  if (!collection) {
    const exist = models.getCollection(Number(req.params.id));
    if (!exist) return res.status(404).json({ error: 'collection not found' });
    return res.status(409).json({ error: 'collection title already exists' });
  }
  res.json(collection);
});

module.exports = router;
