# Collections extension — implementation plan

Extends the existing Bookmark Manager with **collections**, **many-to-many membership**, and **per-collection manual ordering**. Existing bookmark routes and behavior stay unchanged.

---

## 1. Overview — how code changes will be organized

### Database (`db.go`)

- Append DDL to the existing `schema` constant (still applied on startup via `CREATE TABLE IF NOT EXISTS`):
  - **`collections`** — one row per collection (`name` required).
  - **`collection_bookmarks`** — junction table: which bookmarks belong to which collection and their **`position`** (0-based, dense integers within a collection).
- Enable **`ON DELETE CASCADE`** from both FKs so deleting a bookmark or collection cleans up memberships automatically.
- Add a **unique index** on `(collection_id, bookmark_id)` so a bookmark cannot appear twice in the same collection (supports idempotent “add”).
- Keep **`SetMaxOpenConns(1)`** — SQLite already serializes access through one connection; no separate mutex in application code.

### Data layer (`models.go` or new `collections.go`)

- Add **`Collection`** CRUD methods on `Store` (mirror bookmark patterns: `(nil, nil)` when missing).
- Add membership methods: **add**, **remove**, **list ordered**, **move**.
- Wrap every membership mutation that touches **position** (add at end, remove with gap-fill, move/reindex) in a **single SQL transaction** (`BEGIN` … `COMMIT`). On failure, `ROLLBACK`.
- **Move algorithm (inside txn):** load current ordered `(bookmark_id, position)` for the collection; validate bookmark is a member; validate new index is in `[0, n-1]`; if new index equals current index, commit no-op; else reorder slice and **rewrite all positions** for that collection in one batch (safe for SQLite at expected scale).

### HTTP layer (`handlers.go` or `collection_handlers.go`)

- Reuse existing helpers: `writeJSON`, `writeError`, `pathID`, plus small parsers (e.g. `parseCollectionInput`, `pathBookmarkID` from `{bookmark_id}`).
- Register new routes in **`main.go`** using the same Go 1.22 `METHOD /path/{param}` style as bookmarks.
- Handlers stay thin: parse → call `Store` → map errors to status codes (same 400 / 404 / 500 conventions).

### Tests (new `*_test.go` files)

- **`Store` tests:** in-memory SQLite (`DB_PATH` or direct `openDB` helper with `:memory:`) for all store logic, including concurrency-style tests (sequential goroutines still valid with one DB conn).
- **`HTTP` tests:** `httptest` + `httptest.NewRequest` for status codes and JSON bodies on happy paths and validation errors.
- No changes required to existing bookmark tests until we add regression checks that bookmark CRUD still passes after schema migration.

### Idempotency & concurrency (cross-cutting)

| Operation | Idempotent behavior |
|-----------|---------------------|
| Add bookmark to collection | Second identical add → **200 OK** (same state; no duplicate row). First add → **201 Created** optional, or **200** if we prefer uniform success; plan uses **201 first / 200 repeat**. |
| Remove bookmark from collection | Second remove → **204 No Content** (already absent). |
| Move to current position | **200 OK**, body unchanged / list order unchanged. |
| Update collection with same `name` | **200 OK**. |
| Delete collection | Second delete → **404** (matches existing bookmark delete) *or* **204** if we align all deletes to idempotent 204; **implementation choice: 404 on missing collection** to match bookmarks, **204 on remove-from-collection** for membership. |

**Concurrency:** With one DB connection, concurrent HTTP requests are handled one at a time by the driver pool. Transaction boundaries on reorder still prevent torn reads/writes if `MaxOpenConns` is ever raised later. Reorder uses **immediate transaction** (`BEGIN IMMEDIATE` via `db.BeginTx` with appropriate options where supported) so position updates are atomic.

---

## 2. Structs and schema

### Go types

```go
// JSON resource returned by collection CRUD
type Collection struct {
    ID        int64  `json:"id"`
    Name      string `json:"name"`
    CreatedAt string `json:"created_at"`
}

// Request body for POST / PUT collections
type collectionInput struct {
    Name string `json:"name"`
}

// Request body for move endpoint
type moveBookmarkInput struct {
    Position int `json:"position"` // 0-based index in the collection
}

// Optional: internal row for junction table (not necessarily exposed as API)
type collectionBookmark struct {
    CollectionID int64
    BookmarkID   int64
    Position     int
}
```

`Bookmark` remains unchanged. **`GET /collections/{id}/bookmarks`** returns **`[]Bookmark`** in user order (order = array order; no extra field required).

### SQL tables

```sql
CREATE TABLE IF NOT EXISTS collections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collection_bookmarks (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    PRIMARY KEY (collection_id, bookmark_id),
    UNIQUE (collection_id, position)
);
```

**Invariants (enforced in store logic + constraints):**

- Positions for a given `collection_id` are **0 .. n-1** with no gaps after any successful mutating operation.
- A bookmark may appear in **many** collections with **independent** position values per collection.

---

## 3. API specification (new routes)

Base URL: `http://localhost:8080` (same as today). Invalid path ids (non-numeric) → **404** with `{"error":"collection not found"}` or `{"error":"bookmark not found"}` consistent with existing handlers.

### Collection CRUD

| Method | Path | Body | Success | Notes |
|--------|------|------|---------|--------|
| `GET` | `/collections` | — | **200** `[Collection]` | Ordered by `id`. |
| `POST` | `/collections` | `{"name":"Reading list"}` | **201** `Collection` | `name` required (trimmed, non-empty) → else **400**. |
| `GET` | `/collections/{id}` | — | **200** `Collection` | Missing → **404** `collection not found`. |
| `PUT` | `/collections/{id}` | `{"name":"..."}` | **200** `Collection` | Same validation as create; missing → **404**. |
| `DELETE` | `/collections/{id}` | — | **204** | Missing → **404**; cascades junction rows. |

### Membership

| Method | Path | Body | Success | Notes |
|--------|------|------|---------|--------|
| `PUT` | `/collections/{collection_id}/bookmarks/{bookmark_id}` | — | **201** first add, **200** if already member | Collection or bookmark missing → **404**. Idempotent via `UNIQUE (collection_id, bookmark_id)` + “insert or noop”. New members appended at **position = count** (end). |
| `DELETE` | `/collections/{collection_id}/bookmarks/{bookmark_id}` | — | **204** | Idempotent: not a member → still **204**. Collection missing → **404**. Removes row and **decrements positions** of items after the removed slot. |

### Ordered listing

| Method | Path | Body | Success | Notes |
|--------|------|------|---------|--------|
| `GET` | `/collections/{collection_id}/bookmarks` | — | **200** `[Bookmark]` | Order = user order (SQL `ORDER BY position ASC`). Empty collection → **200** `[]`. Missing collection → **404**. |

### Reorder

| Method | Path | Body | Success | Notes |
|--------|------|------|---------|--------|
| `PUT` | `/collections/{collection_id}/bookmarks/{bookmark_id}/position` | `{"position": 0}` | **200** `Bookmark` or **200** ordered snippet | Bookmark not in collection → **404**. `position` required, must be **0 ≤ position < n** → else **400**. Same position → **200** no-op (idempotent). Reindexes all positions in one transaction. |

*(Alternative naming: `PATCH .../position` — `PUT` is fine if body is full intended index.)*

### Error messages (align with existing style)

- `"name is required"`
- `"body must be a JSON object"`
- `"position is required"` / `"position out of range"`
- `"collection not found"`
- `"bookmark not found"` (reuse for unknown bookmark id on membership routes)

---

## 4. Testing plan

### 4.1 Unit tests — store layer (`collections_test.go` / `store_test.go`)

Use `:memory:` SQLite and a shared helper to create `Store` with schema applied.

**Collection CRUD — happy paths**

- Create collection → non-zero id, name persisted.
- List collections → includes created row.
- Get by id → round-trip fields.
- Update name → visible on get.
- Delete collection → get returns nil; list empty.

**Collection CRUD — edge cases**

- Get / update / delete **non-existent id** → `(nil, nil)` / `false`.
- Create with empty name → error at handler; store may assume pre-validated name.

**Membership — happy paths**

- Add two bookmarks to one collection → list returns `[b1, b2]` in add order.
- Same bookmark in **two collections** → independent orders.
- Remove middle bookmark → list order compacts (positions 0..n-1).

**Membership — edge cases & idempotency**

- Add same bookmark twice → one row; second add returns “already member” without changing order.
- Remove bookmark not in collection → success (no error); positions unchanged.
- Remove last bookmark → list `[]`.
- Add to missing collection / missing bookmark → appropriate error or nil parent.
- Delete bookmark (global) → membership rows gone (CASCADE).
- Delete collection → memberships gone; bookmarks table unchanged.

**Move — happy paths**

- `[A, B, C]` move C to `0` → `[C, A, B]`.
- Move B to `1` when already at `1` → unchanged (idempotent).

**Move — edge cases**

- Move bookmark **not in collection** → error / nil.
- `position < 0` or `position >= n` → error.
- `n == 1`, position `0` → no-op ok.

**Concurrency (store-level)**

- Sequential goroutines: two adds to same collection → two distinct positions, no duplicate `(collection_id, bookmark_id)`.
- Sequential goroutines: interleaved move + add → final positions remain dense (run with `-race` in CI).

### 4.2 HTTP tests — handlers (`handlers_collections_test.go`)

Use `httptest.NewRecorder` and a mux wired like `main.go`.

- Each endpoint: **happy path status + JSON shape**.
- **400** invalid JSON, missing `name`, bad `position`.
- **404** bad ids, missing collection/bookmark on nested routes.
- **Idempotent PUT add** twice → 201 then 200 (or both 200 if implemented uniformly).
- **Idempotent DELETE** membership twice → both 204.
- Regression: existing **`GET /bookmarks`** still 200 after migration.

### 4.3 Manual / integration curls

Assume server running (`go run .`) and:

```bash
export BASE=http://localhost:8080
```

**Setup bookmarks**

```bash
curl -s -X POST "$BASE/bookmarks" -H "Content-Type: application/json" \
  -d '{"url":"https://a.example","title":"A","description":""}'
curl -s -X POST "$BASE/bookmarks" -H "Content-Type: application/json" \
  -d '{"url":"https://b.example","title":"B","description":""}'
curl -s -X POST "$BASE/bookmarks" -H "Content-Type: application/json" \
  -d '{"url":"https://c.example","title":"C","description":""}'
# Note ids from responses (example uses 1,2,3)
```

**Collection CRUD**

```bash
# Create
curl -s -X POST "$BASE/collections" -H "Content-Type: application/json" \
  -d '{"name":"Reading list"}'

# List
curl -s "$BASE/collections"

# Get
curl -s "$BASE/collections/1"

# Update
curl -s -X PUT "$BASE/collections/1" -H "Content-Type: application/json" \
  -d '{"name":"Reading list (2026)"}'

# Delete (after other tests, or use id 2)
# curl -s -o /dev/null -w "%{http_code}\n" -X DELETE "$BASE/collections/1"
```

**Add / remove / list (idempotent add & remove)**

```bash
# Add bookmarks 1,2,3 to collection 1 (201 first time)
curl -s -o /dev/null -w "add 1: %{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/1"
curl -s -o /dev/null -w "add 2: %{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/2"
curl -s -o /dev/null -w "add 3: %{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/3"

# Idempotent re-add bookmark 1 (expect 200)
curl -s -o /dev/null -w "re-add 1: %{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/1"

# Ordered list (expect A, B, C)
curl -s "$BASE/collections/1/bookmarks"

# Remove middle bookmark 2 (204)
curl -s -o /dev/null -w "remove 2: %{http_code}\n" -X DELETE "$BASE/collections/1/bookmarks/2"

# Idempotent remove 2 again (204)
curl -s -o /dev/null -w "remove 2 again: %{http_code}\n" -X DELETE "$BASE/collections/1/bookmarks/2"

curl -s "$BASE/collections/1/bookmarks"
```

**Move / reorder**

```bash
# Re-add B as bookmark 2 if needed, then list order 1,2,3
# Move bookmark 3 to front (position 0)
curl -s -X PUT "$BASE/collections/1/bookmarks/3/position" \
  -H "Content-Type: application/json" \
  -d '{"position":0}'

curl -s "$BASE/collections/1/bookmarks"

# Idempotent: same position again
curl -s -o /dev/null -w "move noop: %{http_code}\n" \
  -X PUT "$BASE/collections/1/bookmarks/3/position" \
  -H "Content-Type: application/json" \
  -d '{"position":0}'

# Bad position (400)
curl -s -X PUT "$BASE/collections/1/bookmarks/3/position" \
  -H "Content-Type: application/json" \
  -d '{"position":99}'
```

**404 / validation samples**

```bash
curl -s -X POST "$BASE/collections" -H "Content-Type: application/json" -d '{"name":""}'
curl -s "$BASE/collections/9999"
curl -s -o /dev/null -w "%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/9999"
curl -s -X PUT "$BASE/collections/1/bookmarks/1/position" \
  -H "Content-Type: application/json" -d '{}'
```

### 4.4 Running automated tests

```bash
cd go/bookmarks
go test ./... -race -count=1
```

---

## Implementation checklist (for the coding phase)

1. Extend `schema` in `db.go`.
2. Implement `Store` collection + membership + move methods with transactions.
3. Add handlers + route registration in `main.go`.
4. Add store and handler tests per §4.1–4.2.
5. Extend `setup.md` with new curls (optional, after implementation).
6. Run `go test ./... -race` and manual curl script from §4.3.
