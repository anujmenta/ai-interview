# Bookmark Manager API

A small REST API for saving bookmarks. It supports creating, listing, viewing, updating and deleting bookmarks.

## Task: Collections with Manual Ordering

Users want to group bookmarks into **collections** (e.g. "Reading list", "Recipes") and arrange the bookmarks inside a collection in an order they choose.

Add:

1. A **Collection** resource with basic CRUD (a name is required).
2. A way to **add a bookmark to a collection** and **remove it**. A bookmark can be in more than one collection.
3. An endpoint to **list the bookmarks in a collection**, returned in the user's chosen order.
4. An endpoint to **move a bookmark to a new position** within a collection.

Everything else (status codes, route shapes, validation, edge cases) is up to you — follow the conventions already in the codebase.

Existing bookmarks must keep working.
