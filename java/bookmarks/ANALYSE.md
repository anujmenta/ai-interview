# Bookmarks (Java) — Code Analysis & Collections Feature Workflow

Scope: `java/bookmarks` only. Stack: JDK 17, `com.sun.net.httpserver.HttpServer` (no framework),
raw JDBC against SQLite (`org.xerial:sqlite-jdbc`), Jackson for JSON (de)serialization, no router
library — all request dispatch is hand-written string matching in `Routes.java`.

---

## 1. Existing code, method by method

### `App.java`
| Method | What it does |
|---|---|
| `main(String[])` | Entry point. Calls `Db.init()` to create the schema if missing, reads `PORT` env var (default `8080`), creates a `com.sun.net.httpserver.HttpServer` bound to that port, registers a single `Routes` instance (wrapping a fresh `Store`) as the handler for context `"/"`, starts the server. There is no framework routing — `Routes` gets *every* request regardless of path. |

### `Db.java`
| Method | What it does |
|---|---|
| `PATH` (static field) | DB file path, from `DB_PATH` env var, default `bookmarks.db`. |
| `SCHEMA` (static field) | Raw DDL string: `CREATE TABLE IF NOT EXISTS bookmarks (...)`. |
| `getConn()` | Opens a **new** JDBC connection per call via `DriverManager.getConnection`, with `SQLiteConfig.enforceForeignKeys(true)` so `ON DELETE`/`ON UPDATE` FK actions are honored. No connection pooling — every `Store` method opens/closes its own connection. |
| `init()` | Opens a connection, runs `SCHEMA` via a plain `Statement`, closes. Called once at startup; idempotent (`IF NOT EXISTS`). |

### `Bookmark.java`
A `record Bookmark(long id, String url, String title, String description, String createdAt)`. Pure immutable DTO — Jackson serializes it to JSON by reading the record's accessor methods (`id()`, `url()`, etc.) as fields `id`, `url`, `title`, `description`, `createdAt`.

### `Store.java` — data access layer
| Method | What it does |
|---|---|
| `scan(ResultSet)` (private, static) | Maps the current row of a `ResultSet` into a `Bookmark`. Single place that knows the column→field mapping. |
| `listBookmarks()` | `SELECT ... FROM bookmarks ORDER BY id`, collects every row into a `List<Bookmark>`. |
| `getBookmark(long id)` | Public overload: opens a connection, delegates to the private 2-arg version, closes the connection. Returns `Optional<Bookmark>` — **empty, not null**, when no row matches. |
| `getBookmark(Connection, long id)` (private) | Runs `SELECT ... WHERE id = ?` on an **already-open** connection (so callers that already have a connection, e.g. inside `createBookmark`/`updateBookmark`, don't open a second one). This overload is the reusable "fetch after write" building block. |
| `createBookmark(url, title, description)` | `INSERT ... VALUES (?, ?, ?)` with `Statement.RETURN_GENERATED_KEYS`, reads back the new `id` via `getGeneratedKeys()`, then calls the connection-scoped `getBookmark` to return the full persisted row (so `created_at`'s DB-generated default is included in the response). |
| `updateBookmark(id, url, title, description)` | `UPDATE ... WHERE id = ?`. If `executeUpdate()` reports 0 rows affected, returns `Optional.empty()` (caller turns this into 404). Otherwise re-fetches and returns the updated row. |
| `deleteBookmark(id)` | `DELETE ... WHERE id = ?`, returns `true` iff a row was actually removed (`executeUpdate() > 0`). |

Convention baked into every method: **one try-with-resources per call that opens its own `Connection`/`PreparedStatement`/`ResultSet`** (except the private connection-scoped overload), no shared/pooled state, `SQLException` is allowed to propagate up (not caught here).

### `Routes.java` — HTTP layer (implements `HttpHandler`)
| Method | What it does |
|---|---|
| `Input` (private record) | Parsed/validated request body shape: `(url, title, description)`. |
| `HttpError` (private static exception) | Carries an HTTP status + message; thrown anywhere in routing/validation to short-circuit to an error response. |
| `handle(HttpExchange)` | The single entry point `HttpServer` calls for *every* request. Delegates to `route(ex)`; catches `HttpError` → writes `{status, {"error": message}}`; catches `SQLException` → writes 500 with the message; `finally` always closes the exchange. |
| `route(HttpExchange)` (private) | The actual dispatcher. Normalizes the path (strips one trailing `/`), then does manual `if`/`switch` matching: exact `/bookmarks` → GET (list) / POST (create); prefix `/bookmarks/` → parses the id suffix, then GET (fetch) / PUT (update) / DELETE (delete). Anything else → 404; wrong method on a matched path → 405. Uses `Optional.orElseThrow(() -> new HttpError(404, ...))` to turn a missing `Store` result straight into the 404 response. |
| `parseId(String)` (private, static) | `Long.parseLong`; a non-numeric id becomes `HttpError(404, "bookmark not found")` — i.e. bad ids are reported identically to missing ids, not as 400s. |
| `parseInput(HttpExchange)` (private, static) | Reads the request body as JSON via Jackson's `ObjectMapper.readTree`; rejects non-object bodies and I/O failures as 400 `"body must be a JSON object"`; validates `url`/`title`/`description` field-by-field via `requiredString`; returns an `Input`. |
| `requiredString(JsonNode, field)` (private, static) | Shared validation for both `url` and `title`: the field must exist, be textual, and be non-blank after `.strip()` — otherwise `HttpError(400, "<field> is required")`. |
| `writeJson(HttpExchange, status, value)` (private, static) | Serializes `value` with Jackson, sets `Content-Type: application/json`, writes the status + body. Single choke point for every successful (and error) response body. |

### Request flow (existing `/bookmarks*` endpoints)
```
HttpServer → Routes.handle(ex)
               → route(ex)
                   → parseInput / parseId      (validation, may throw HttpError)
                   → store.<method>(...)       (DB op, may throw SQLException)
                   → writeJson(ex, status, v)  (success)
               ↳ HttpError  → writeJson(ex, e.status, {"error": ...})
               ↳ SQLException → writeJson(ex, 500, {"error": ...})
```

### Conventions to preserve for the new feature
1. **Records as DTOs**, `createdAt` style naming matching DB `created_at` via Jackson's default snake→camel is *not* configured — actually Jackson here just serializes the record accessor name verbatim (`createdAt`), while the DB column is `created_at`; the mapping is manual in `Store.scan`. New DTOs should follow the same manual-mapping pattern.
2. **`Optional<T>` for "may not exist"** return values from `Store`, turned into 404 via `orElseThrow(() -> new HttpError(404, ...))` in `Routes`.
3. **One connection per `Store` call**, try-with-resources, no pooling.
4. **`HttpError` for all client-facing failures**; `SQLException` bubbles up to a generic 500.
5. **Manual path parsing** in `route()` — no path-variable framework. New nested routes (`/collections/{id}/bookmarks/{bookmarkId}`) need either more `startsWith`/`substring` chaining or (cleaner, still dependency-free) splitting the path into segments — see §6.
6. **Trailing-slash stripping, 404 on unmatched path, 405 on unmatched method** must keep applying to new routes too.
7. Field validation is centralized (`requiredString`) — reuse the same helper for the collection `name` field.

---

## 2. Feature: Collections with Manual Ordering

Requirements from `PROBLEM.md`:
1. `Collection` resource — CRUD, `name` required.
2. Add / remove a bookmark from a collection (many-to-many: a bookmark can be in several collections).
3. List a collection's bookmarks in the user's chosen order.
4. Move a bookmark to a new position within a collection.
5. Existing `/bookmarks` endpoints must keep working untouched.

---

## 3. Data model

Two new tables, added to `Db.SCHEMA` (same `CREATE TABLE IF NOT EXISTS` style):

```sql
CREATE TABLE IF NOT EXISTS collections (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collection_bookmarks (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    bookmark_id   INTEGER NOT NULL REFERENCES bookmarks(id)   ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    PRIMARY KEY (collection_id, bookmark_id)
);
```

- `enforceForeignKeys(true)` is already set in `Db.getConn()`, so the `ON DELETE CASCADE` means
  deleting a collection or a bookmark automatically cleans up `collection_bookmarks` rows — no
  manual cleanup code needed in `Store`.
- `position` is a **dense, 0-based integer per collection** (0, 1, 2, …), not a sparse/fractional
  key. Simpler to reason about and matches the low-tech style of the rest of the app. The
  trade-off is that add/remove/move must shift neighboring rows — see §5.4–5.6. This is O(n) per
  mutation on a collection, which is fine at bookmark-list scale.
- `PRIMARY KEY (collection_id, bookmark_id)` enforces "no duplicate membership" for free and gives
  an efficient existence check.

---

## 4. API design

| Method | Path | Purpose | Success | Failure |
|---|---|---|---|---|
| `POST` | `/collections` | create collection | 201 + `Collection` | 400 missing `name` |
| `GET` | `/collections` | list collections | 200 + `[Collection]` | — |
| `GET` | `/collections/{id}` | fetch one | 200 + `Collection` | 404 |
| `PUT` | `/collections/{id}` | rename | 200 + `Collection` | 400 / 404 |
| `DELETE` | `/collections/{id}` | delete (cascades memberships) | 204 | 404 |
| `POST` | `/collections/{id}/bookmarks` | add a bookmark (body: `{"bookmark_id": N}`), appended at the end | 201 + membership entry | 404 collection/bookmark not found, 409 already a member |
| `GET` | `/collections/{id}/bookmarks` | list member bookmarks, ordered by `position` | 200 + `[Bookmark]` | 404 collection not found |
| `DELETE` | `/collections/{id}/bookmarks/{bookmarkId}` | remove, then compact positions | 204 | 404 (collection, or bookmark not a member) |
| `PUT` | `/collections/{id}/bookmarks/{bookmarkId}/position` | move to new position (body: `{"position": N}`), shifts neighbors | 200 + `[Bookmark]` (new order) | 400 invalid position, 404 |

Notes:
- Reusing `{"error": "..."}` shape and the existing status-code conventions (400 validation, 404
  missing resource, 405 wrong method) exactly as today's `/bookmarks` routes do.
- `position` in the move request is the **target 0-based index**, clamped-checked against
  `[0, memberCount-1]`; out of range → 400, not silently clamped, to stay consistent with how
  the app treats bad input elsewhere (explicit error over silent correction).

---

## 5. Complete workflow

### 5.1 New/changed files
```
Db.java         (edit)   — extend SCHEMA with the two new tables
Collection.java (new)    — record Collection(long id, String name, String createdAt)
Store.java      (edit)   — add collection + membership methods (same class, same conventions
                            as the existing bookmark methods — the codebase uses one Store for
                            everything, so collections follow suit rather than introducing a
                            second DAO abstraction)
Routes.java     (edit)   — extend route() dispatch for /collections and its sub-resources
```

### 5.2 `Store` — new methods (mirroring existing method shapes exactly)

| Method | SQL | Returns |
|---|---|---|
| `createCollection(name)` | `INSERT INTO collections(name) VALUES (?)` + re-fetch by generated key | `Collection` |
| `listCollections()` | `SELECT ... FROM collections ORDER BY id` | `List<Collection>` |
| `getCollection(id)` / `getCollection(conn, id)` | `SELECT ... WHERE id = ?` | `Optional<Collection>` (public opens its own connection; private connection-scoped overload reused internally — same split as `getBookmark`) |
| `updateCollection(id, name)` | `UPDATE collections SET name = ? WHERE id = ?`, 0 rows → empty | `Optional<Collection>` |
| `deleteCollection(id)` | `DELETE FROM collections WHERE id = ?` (FK cascade removes memberships) | `boolean` |
| `addBookmarkToCollection(collectionId, bookmarkId)` | Checks collection exists and bookmark exists (else `Optional.empty()`/distinguishable result), checks not already a member (else a sentinel for 409), `SELECT COALESCE(MAX(position), -1) + 1 ... WHERE collection_id = ?` for next position, then `INSERT INTO collection_bookmarks VALUES (?, ?, ?)` | tri-state result the route layer maps to 201/404/409 (e.g. a small enum or three dedicated boolean/Optional checks — kept simple, not over-engineered) |
| `removeBookmarkFromCollection(collectionId, bookmarkId)` | `DELETE ... WHERE collection_id = ? AND bookmark_id = ?`; if a row was removed, `UPDATE collection_bookmarks SET position = position - 1 WHERE collection_id = ? AND position > ?` to compact the gap | `boolean` |
| `listCollectionBookmarks(collectionId)` | `SELECT b.* FROM bookmarks b JOIN collection_bookmarks cb ON cb.bookmark_id = b.id WHERE cb.collection_id = ? ORDER BY cb.position` | `List<Bookmark>` (empty list if collection has no members; caller checks collection existence separately for the 404 case) |
| `moveBookmark(collectionId, bookmarkId, newPosition)` | Read current `position` for the pair (404/empty if not a member); validate `newPosition` against `COUNT(*) WHERE collection_id = ?` (400 if out of range, handled in `Routes`); shift the affected range by ±1 (`UPDATE ... SET position = position ± 1 WHERE collection_id = ? AND position BETWEEN ? AND ?`, excluding the moved row); then `UPDATE collection_bookmarks SET position = ? WHERE collection_id = ? AND bookmark_id = ?`. All four statements run on **one shared `Connection`** with `setAutoCommit(false)` / `commit()` / rollback in a `catch`, since this is the one place multiple writes must be atomic (new pattern vs. existing code, but consistent with "use a single Connection for a multi-statement operation" seen in `createBookmark`'s read-after-write) | `Optional<List<Bookmark>>` (new order) or `Optional.empty()` if not a member |

### 5.3 `Routes` — dispatch changes
`route()` currently branches on exact-match `/bookmarks` and prefix `/bookmarks/`. Extend with
analogous branches, ideally by first splitting the normalized path into segments
(`path.split("/")`) so `/collections/{id}/bookmarks/{bookmarkId}/position` doesn't need four
levels of nested `startsWith`/`substring`:

```
segments = ["collections", "{id}", "bookmarks", "{bookmarkId}", "position"?]
```

Dispatch table (method + segment shape → handler), each handler following the exact same
try/throw `HttpError` / `writeJson` pattern as today's bookmark handlers:
- `["collections"]` → GET list / POST create
- `["collections", id]` → GET / PUT / DELETE
- `["collections", id, "bookmarks"]` → GET list-ordered / POST add
- `["collections", id, "bookmarks", bookmarkId]` → DELETE remove
- `["collections", id, "bookmarks", bookmarkId, "position"]` → PUT move

`parseInput`-equivalents needed:
- `CollectionInput(String name)` record + `requiredString(data, "name")` (reuses the existing helper).
- `AddBookmarkInput(long bookmarkId)` — body `{"bookmark_id": N}`, validated as a required integer.
- `MoveInput(int position)` — body `{"position": N}`, validated as a required non-negative integer.

### 5.4 Sequence: add a bookmark to a collection
```
POST /collections/5/bookmarks {"bookmark_id": 42}
  Routes.route → parse ids + body
    Store.addBookmarkToCollection(5, 42)
      getCollection(conn, 5)        → empty? → 404 "collection not found"
      getBookmark(conn, 42)         → empty? → 404 "bookmark not found"
      existing membership check     → present? → 409 "bookmark already in collection"
      SELECT MAX(position)+1 → INSERT
  → 201 + membership/collection-bookmark representation
```

### 5.5 Sequence: list a collection's bookmarks in order
```
GET /collections/5/bookmarks
  Routes.route
    Store.getCollection(5)              → empty? → 404
    Store.listCollectionBookmarks(5)    → JOIN ... ORDER BY position
  → 200 + [Bookmark] (already correctly ordered — no app-side sorting needed)
```

### 5.6 Sequence: remove a bookmark from a collection
```
DELETE /collections/5/bookmarks/42
  Routes.route
    Store.removeBookmarkFromCollection(5, 42)
      DELETE WHERE collection_id=5 AND bookmark_id=42
      rowsAffected == 0 → 404
      else: UPDATE positions > removedPosition  SET position = position - 1   (compact gap)
  → 204
```

### 5.7 Sequence: move a bookmark to a new position
```
PUT /collections/5/bookmarks/42/position {"position": 1}
  Routes.route → validate position is a non-negative integer (else 400)
    Store.moveBookmark(5, 42, 1)
      BEGIN
        currentPosition = SELECT position WHERE collection_id=5 AND bookmark_id=42
                           → absent → ROLLBACK, return empty → 404
        count = SELECT COUNT(*) WHERE collection_id=5
                           → position >= count → ROLLBACK, caller returns 400 "position out of range"
        if newPosition < currentPosition:
            UPDATE position = position + 1 WHERE collection_id=5
                   AND position BETWEEN newPosition AND currentPosition-1
        else if newPosition > currentPosition:
            UPDATE position = position - 1 WHERE collection_id=5
                   AND position BETWEEN currentPosition+1 AND newPosition
        UPDATE position = newPosition WHERE collection_id=5 AND bookmark_id=42
      COMMIT
  → 200 + listCollectionBookmarks(5)   (full new order, so the client can confirm the result)
```

### 5.8 End-to-end component diagram (new pieces in **bold**)
```
HttpServer
  └─ Routes (HttpHandler)
       ├─ /bookmarks, /bookmarks/{id}              (unchanged)
       ├─ **/collections, /collections/{id}**
       └─ **/collections/{id}/bookmarks[...]**
            └─ Store
                 ├─ bookmark methods            (unchanged)
                 ├─ **collection CRUD methods**
                 └─ **collection_bookmarks membership + ordering methods**
                      └─ Db.getConn()  →  SQLite (collections, collection_bookmarks tables, FK cascade)
```

---

## 6. Implementation order (recommended)

1. **Schema**: extend `Db.SCHEMA` with `collections` + `collection_bookmarks`. No migration
   tooling exists in this project (`CREATE TABLE IF NOT EXISTS` is the only mechanism) — adding
   the two statements is additive and leaves existing `bookmarks` rows untouched, satisfying
   "existing bookmarks must keep working."
2. **`Collection.java`**: new record, mirrors `Bookmark.java`.
3. **`Store.java`**: add the methods from §5.2, in the same file, same style (try-with-resources,
   `Optional` for "not found", booleans for delete-style operations). Add the one transactional
   method (`moveBookmark`) carefully — it's the only multi-statement write in the codebase.
4. **`Routes.java`**: extend `route()` dispatch, add the small input records + reuse
   `requiredString`/`writeJson`/`HttpError`.
5. **Manual smoke test** (no test framework present in this project, same as today): run
   `mvn compile exec:java`, then exercise with `curl`:
   - create 2 bookmarks, create a collection, add both, GET the ordered list, move one, GET
     again to confirm order changed, remove one, confirm 404 on second removal, delete the
     collection, confirm its bookmarks still exist via `GET /bookmarks`.
6. Re-verify existing `/bookmarks` endpoints are untouched (same handlers, same SQL, same status
   codes) — the diff should be purely additive in `Db`, `Routes`, `Store`, plus one new file.
