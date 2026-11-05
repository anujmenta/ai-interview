# Implementation callouts & open items

---

## Resolved decisions

| Topic | Decision |
|-------|----------|
| Route registration | `Store.registerRoutes` in `routes.go`. |
| Add to collection | `PUT ...` — **201** first add, **200** if already member. |
| All `DELETE` endpoints | **204 No Content** when the path id is valid, including when the resource is already gone (idempotent delete). |
| Invalid path ids | Non-numeric `{id}` / `{bookmark_id}` → **404** with `collection not found` or `bookmark not found`. |
| Duplicate collection names | **Allowed.** Dedup / identity for idempotent ops uses **`collection_id`** (and bookmark ids), not name uniqueness. |
| Move validation (Q1) | See **Move / position errors** below. |
| List order in JSON (Q4) | **Keep `[]Bookmark` only** — order = array index; no `position` field (see Q4 detail). |
| `BEGIN IMMEDIATE` | Ignored for now. |
| HTTP test breadth (Q7) | On hold. |

### Move / position errors (Q1)

Checked in order in `MoveCollectionBookmark`:

| Situation | HTTP | `error` message |
|-----------|------|-----------------|
| Collection id invalid (non-numeric path) | **404** | `collection not found` |
| Collection id valid but no row | **404** | `collection not found` |
| Bookmark id invalid (non-numeric path) | **404** | `bookmark not found` |
| Bookmark id valid but no row | **404** | `bookmark not found` |
| Bookmark exists but not in this collection (includes **empty collection**) | **404** | `bookmark not in collection` |
| Body missing / invalid JSON | **400** | `body must be a JSON object` |
| Body missing `position` | **400** | `position is required` |
| `position` &lt; 0 or `position` ≥ member count | **400** | `position out of range` |
| Valid move (including no-op same index) | **200** | `Bookmark` JSON body |

Membership **add** still returns **404** `bookmark not found` when the bookmark row does not exist; **404** `collection not found` when the collection does not exist.

---

## Q4 — Why not expose `position` on list?

**Current:** `GET /collections/{id}/bookmarks` returns `[{ "id", "url", "title", "description", "created_at" }, ...]`. Order is **only** the JSON array order (matches `ORDER BY position ASC` in SQL).

**Option A — array order only (chosen)**

- Pros: Same `Bookmark` shape as `GET /bookmarks`; simple clients (render list top-to-bottom); no redundant fields.
- Cons: Clients cannot see sparse/gap metadata (we don’t have gaps anyway); reorder APIs must be used to inspect indices explicitly.

**Option B — wrap or extend with `position`**

- Example: `{ "bookmark": { ... }, "position": 0 }` or add `"position"` on each element.
- Pros: Explicit index for UI labels (“move to slot 3”); easier to debug without counting array indices.
- Cons: Breaks symmetry with global bookmark list; two sources of truth if client re-sorts locally; slightly larger payloads.

**Decision:** Stay with **Option A** unless a client needs to display numeric ranks without inferring from array index. Reorder is done via `PUT .../position` with `{"position": N}`.

---

## Callouts (behavior to be aware of)

1. **Schema migration** — `CREATE TABLE IF NOT EXISTS` only; no versioned migrations.

2. **Position updates on move** — Two-phase SQL updates to satisfy `UNIQUE (collection_id, position)`.

3. **Remove membership** — Does not require bookmark row to exist; idempotent **204**. Collection must exist or **404**.

4. **SQLite tests** — In-memory DB per test via `DB_PATH`.

---

## Verification

```bash
cd go/bookmarks
go test ./... -race -count=1
go run .
```

Manual curls: **`setup.md`**.

---

## Public share links (read-only)

No authentication; anyone with the token can read; anyone can revoke.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/bookmarks/{id}/share` | Create link for one bookmark (**201**) |
| `POST` | `/collections/{id}/share` | Create link for collection list in order (**201**) |
| `GET` | `/public/{token}` | Read-only view (**200**) |
| `DELETE` | `/public/{token}` | Revoke (**204**, idempotent) |

Optional JSON body on create: `{"expires_at":"2026-12-31T00:00:00Z"}` and/or `{"ttl_seconds":3600}`. Expired or revoked links → **404** `share not found`. Deleting the bookmark/collection cascades share rows.
