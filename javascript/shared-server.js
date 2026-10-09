const express = require('express');
const bookmarkRoutes = require('./bookmarks/src/routes');
const collectionRoutes = require('./collections/src/routes');

const app = express();
app.use(express.json());
app.use(bookmarkRoutes);
app.use(collectionRoutes);

// malformed JSON etc.
app.use((err, req, res, next) => {
  res.status(err.status || 500).json({ error: err.message });
});

const port = process.env.PORT || 3000;
app.listen(port, () => console.log(`listening on http://localhost:${port}`));
