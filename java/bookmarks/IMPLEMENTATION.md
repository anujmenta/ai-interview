# Implementation Notes — Collections with Manual Ordering (Java)

Implements the 4 requirements from `PROBLEM.md` on top of the existing Java bookmarks app
(`java/bookmarks`), following the design in `ANALYSE.md`. This file records **what changed, in
which file, and why** — reasoning first, diff shape second.

| # | Requirement | Where it's implemented |
|---|---|---|
| 1 | `Collection` resource, CRUD, `name` required | `Collection.java` (new), `Store.java` (new methods), `Routes.java` (`/collections`, `/collections/{id}`) |
| 2 | Add/remove a bookmark from a collection (many-to-many) | `Db.java` (`collection_bookmarks` table), `Store.java`, `Routes.java` (`/collections/{id}/bookmarks`, `/collections/{id}/bookmarks/{bookmarkId}`) |
| 3 | List a collection's bookmarks in chosen order | `Store.listCollectionBookmarks`, `Routes.java` (`GET /collections/{id}/bookmarks`) |
| 4 | Move a bookmark to a new position | `Store.moveBookmark`, `Routes.java` (`PUT /collections/{id}/bookmarks/{bookmarkId}/position`) |

"Existing bookmarks must keep working" — verified explicitly: no existing method signature,
SQL statement, route, or status code for `/bookmarks*` was touched. Every change is additive.
This was re-checked by running the original `/bookmarks` CRUD flow end-to-end after the new code
was in place (see §5).

---

## 1. `Db.java` — schema

**Change:** added two tables to the schema, and changed `init()` from one `stmt.execute(SCHEMA)`
call on a single `;`-joined string to a loop over an array of individual `CREATE TABLE`
statements.

```sql
CREATE TABLE IF NOT EXISTS collections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collection_bookmarks (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    bookmark_id   INTEGER NOT NULL REFERENCES bookmarks(id)   ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    PRIMARY KEY (collection_id, bookmark_id)
);
```

**Reasoning:**
- `collections` mirrors `bookmarks`' own shape (`id`, required text field, `created_at`
  default) — same convention, nothing new introduced.
- `collection_bookmarks` is the join table that makes a bookmark belong to *many* collections
  (requirement 2). `PRIMARY KEY (collection_id, bookmark_id)` gets "no duplicate membership" for
  free from the DB, so the app only needs to turn a constraint violation into a 409, not
  re-implement uniqueness itself.
- `position` is a dense, 0-based integer scoped per `collection_id`. Chosen over a
  fractional/sparse key (e.g. floats or string-based ordering) because it's the simplest option
  that satisfies "list in chosen order" and "move to a new position" at the scale this app
  operates at, and it matches the low-tech style already in the codebase (no existing abstraction
  for ordering to reuse or extend).
- `ON DELETE CASCADE` on both foreign keys means deleting a collection or a bookmark automatically
  removes the corresponding `collection_bookmarks` rows — no manual cleanup code needed in
  `Store`, and it's the reason `Db.getConn()` already enabling `enforceForeignKeys(true)` matters
  here (it was already on for the existing table, just unused until now).
- **Bug found during testing, fixed here:** the original `init()` executed the whole multi-statement
  `SCHEMA` string as a single `Statement.execute(...)` call. That happened to work with one
  `CREATE TABLE`, but the `org.xerial:sqlite-jdbc` driver's `execute()` only runs the *first*
  statement in a `;`-separated string — it does not behave like a script runner. With three
  tables in one string, `collections`/`collection_bookmarks` were silently never created, and
  every collections endpoint failed with `no such table: collections`. Fixed by making `SCHEMA`
  a `String[]` (one statement per entry) and looping `stmt.execute(ddl)` for each. This was caught
  by the smoke test in §5, not by inspection — worth flagging since it's an easy trap with
  multi-line Java text blocks that *look* like one valid script.

---

## 2. `Collection.java` (new file)

```java
public record Collection(long id, String name, String createdAt) {}
```

**Reasoning:** exact structural mirror of `Bookmark.java`. Keeping a 1:1 shape between resource
records (record, field names, field types) means Jackson serialization "just works" the same
way it already does for bookmarks, with no new serialization configuration.

---

## 3. `CollectionBookmark.java` (new file)

```java
public record CollectionBookmark(Bookmark bookmark, int position) {}
```

**Reasoning:** this is the response body for `POST /collections/{id}/bookmarks` — the thing
created by that call isn't a `Bookmark` (which already exists) or a `Collection`, it's a
*membership* at a specific position. Returning `{bookmark: {...}, position: N}` tells the client
both what was linked and where it landed (always the end of the list on add), which the problem's
"add a bookmark to a collection" requirement implies the caller needs to know. Nested record
serialization requires no extra Jackson config — it serializes `Bookmark`'s fields inline exactly
as `writeJson` already does for top-level records.

---

## 4. `MoveResult.java` (new file)

```java
public record MoveResult(MoveResult.Status status, List<Bookmark> bookmarks) {
    public enum Status { OK, NOT_MEMBER, OUT_OF_RANGE }
}
```

**Reasoning:** every other `Store` method in this codebase signals "not found" with a single
`Optional`/`boolean`, because there's only ever one failure mode to report. `moveBookmark` has
**three** distinct outcomes that must map to different HTTP statuses — the bookmark isn't a
member of the collection (404), the requested position is out of range (400), or the move
succeeded (200). Reusing `Optional.empty()` for both failure cases would collapse two different
client errors into one status code, which would be a worse API than what `PROBLEM.md` asks for.
A small result type was the minimal way to keep that distinction without inventing a bigger
exception hierarchy — `Routes` just switches on `result.status()`.

---

## 5. `Store.java` — new methods

All new methods were written to match the file's existing conventions exactly: one
try-with-resources `Connection` per public call (no pooling), a private `Connection`-scoped
overload for methods that need to compose multiple statements (same pattern as the existing
`getBookmark(Connection, long)`), `Optional<T>` for "may not exist," `boolean` for delete-style
operations, `PreparedStatement` everywhere (no string-built SQL with user input).

**Collection CRUD** (mirrors `createBookmark`/`updateBookmark`/`deleteBookmark` 1:1):
- `listCollections()` — `SELECT ... ORDER BY id`.
- `getCollection(long)` / `getCollection(Connection, long)` — same public/private split as
  `getBookmark`, so callers that already hold a connection (e.g. inside `createCollection`) don't
  open a second one.
- `createCollection(String name)` — `INSERT ... RETURN_GENERATED_KEYS`, then re-fetches on the
  same connection so the response includes the DB-generated `created_at`.
- `updateCollection(long, String)` — `UPDATE`, 0 rows affected → `Optional.empty()` (→ 404 in
  `Routes`).
- `deleteCollection(long)` — `DELETE`, returns whether a row was removed. Relies on the FK cascade
  to clean up memberships; no explicit `DELETE FROM collection_bookmarks` needed.

**Membership & ordering** (new, since nothing like this existed before):
- `listCollectionBookmarks(long)` / `(Connection, long)` — `JOIN collection_bookmarks → bookmarks`,
  `ORDER BY position`. The ordering requirement (requirement 3) is satisfied entirely in SQL —
  no in-memory sort needed, so the API can't drift out of sync with how positions are stored.
- `addBookmarkToCollection(long collectionId, long bookmarkId)` — checks membership first
  (`isMember`); if already a member returns `Optional.empty()` (→ 409 in `Routes`, distinguishing
  "already there" from "collection/bookmark missing," which are checked separately in `Routes`
  *before* this call so each gets its own 404 message). Otherwise computes the next position as
  `COALESCE(MAX(position), -1) + 1` scoped to the collection (so the first bookmark added gets
  position 0) and inserts. Returns the full `CollectionBookmark` so the caller sees where it
  landed.
- `removeBookmarkFromCollection(long, long)` — looks up the current `position` (null → not a
  member → `false` → 404); deletes the row; then **compacts the gap** with
  `UPDATE ... SET position = position - 1 WHERE collection_id = ? AND position > ?`. Compacting on
  every removal keeps positions dense (0..n-1) at all times, which is what makes the `moveBookmark`
  range check (`0 <= newPosition < count`) simple and always correct — there's never a gap to
  reason about.
- `moveBookmark(long collectionId, long bookmarkId, int newPosition)` — this is requirement 4.
  Looks up the bookmark's current position (absent → `NOT_MEMBER`); checks `newPosition` against
  the live count of members in the collection (out of range → `OUT_OF_RANGE`); otherwise shifts
  every row strictly between the old and new position by ±1 (`shiftPositions`, a single
  `UPDATE ... WHERE position BETWEEN ? AND ?`) to open a slot, then writes the moved row's new
  position, then returns the freshly re-ordered list so the client can see the result of the move
  directly instead of having to issue a second `GET`.
  - `shiftPositions`/`currentPosition`/`countCollectionBookmarks`/`isMember` are small private
    helpers factored out because `moveBookmark`, `removeBookmarkFromCollection`, and
    `addBookmarkToCollection` all need some combination of them — each one does exactly one SQL
    statement, same granularity as the rest of the file.
  - No explicit transaction (`setAutoCommit(false)`) was added around the shift + set-position
    sequence. The existing codebase doesn't use transactions anywhere (e.g.
    `createBookmark`'s insert-then-fetch isn't wrapped either), and this is a single-process,
    `SetMaxOpenConns`-unbounded-but-effectively-serial toy app with no concurrent-write tests
    expected. Adding transaction handling here would be the one inconsistent part of the diff
    relative to everything around it, so it was deliberately left out rather than introduced
    only for this one method.

---

## 6. `Routes.java` — HTTP dispatch

**Change:** added one new top-level branch in `route()`:

```java
if (path.equals("/collections") || path.startsWith("/collections/")) {
    routeCollections(ex, method, path);
    return;
}
```

placed after the existing `/bookmarks` branches and before the final `throw new HttpError(404, ...)`
— purely additive, the two existing branches are untouched.

**New method `routeCollections`:** the existing dispatcher handles `/bookmarks` and `/bookmarks/{id}`
with `equals`/`startsWith`/`substring`, which works for a flat one-level resource. `/collections`
needs up to five path levels
(`/collections/{id}/bookmarks/{bookmarkId}/position`), so `routeCollections` splits the path into
segments (`path.split("/")`) and dispatches on `parts.length` + a couple of literal segment
checks (`parts[3].equals("bookmarks")`, `parts[5].equals("position")`). This was chosen over
nesting more `startsWith`/`substring` calls because four levels of that would be substantially
harder to read than the existing two; the segment-count `if`/`switch` ladder stays close in
spirit to the original (manual, no router library) while staying readable:

| `parts.length` | shape | methods |
|---|---|---|
| 2 | `/collections` | GET list, POST create |
| 3 | `/collections/{id}` | GET, PUT, DELETE |
| 4 | `/collections/{id}/bookmarks` | GET ordered list, POST add |
| 5 | `/collections/{id}/bookmarks/{bookmarkId}` | DELETE remove |
| 6 | `/collections/{id}/bookmarks/{bookmarkId}/position` | PUT move |

Each branch reuses the exact same primitives the `/bookmarks` branches use — `writeJson`,
`HttpError`, `Optional.orElseThrow(() -> new HttpError(404, ...))` — so error handling,
status codes, and response shape are indistinguishable in style from the pre-existing code.

**Supporting additions, and why each was needed:**
- `COLLECTION_NOT_FOUND` / `NOT_IN_COLLECTION` constants — parallel to the existing `NOT_FOUND`
  constant, so each resource type gets its own clear 404 message instead of reusing "bookmark not
  found" for a missing collection.
- `parseId(String, String notFoundMessage)` — the original `parseId(String)` hardcoded the
  `NOT_FOUND` ("bookmark not found") message, which is wrong when parsing a collection id that
  fails to parse (should say "collection not found"). Overloaded rather than replaced, so the
  single existing call site (`/bookmarks/{id}`) is untouched and still gets its original message
  via the 1-arg overload delegating to the 2-arg one.
- `readJsonObject(HttpExchange)` — extracted from the body-reading half of `parseInput` (JSON
  parse + "must be an object" check), because three new input parsers
  (`parseCollectionInput`, `parseBookmarkId`, `parsePosition`) all need the identical
  read-and-validate-shape step; duplicating it three times would've been the actual "copy-paste"
  smell the task conventions want avoided. `parseInput` itself was refactored to call it too,
  with no behavior change (same two `HttpError(400, "body must be a JSON object")` cases as
  before).
- `requiredLong(JsonNode, String)` — parallel to the existing `requiredString`, for the two new
  integer body fields (`bookmark_id` on add, `position` on move). Validates the field is present
  and an integral JSON number; left negative values to flow through to `moveBookmark`'s own
  range check rather than re-validating "non-negative" in two places.
- `addBookmarkToCollection(HttpExchange, long)` helper — pulled out of the `POST
  /collections/{id}/bookmarks` branch because it's a 3-step sequence (collection exists? →
  parse+validate `bookmark_id` → bookmark exists? → add) that would otherwise bloat the
  `parts.length == 4` case in the already-long `routeCollections` switch.

**Status codes chosen (not specified by `PROBLEM.md`, decided to match existing conventions):**
- `201` on successful add to a collection (a new membership resource was created), same as
  `POST /bookmarks`.
- `409` on adding a bookmark that's already a member — distinguishes "conflicting state" from
  "not found," consistent with the DB-level uniqueness the schema already enforces.
- `400` on an out-of-range move position — a client input problem, same bucket as missing
  `url`/`title`/`name`.
- `404` on moving/removing a bookmark that isn't a member of the collection — same bucket as
  every other "resource doesn't exist" case in the app.
- `204` on successful delete (collection delete, bookmark-removed-from-collection) — matches the
  existing `DELETE /bookmarks/{id}` convention exactly.

---

## 7. Verification

Ran `mvn compile` (clean compile, no errors) and then `mvn exec:java` against a throwaway
SQLite file, exercising every endpoint with `curl`:

- **Existing endpoints, re-checked after all changes landed:** `POST/GET/PUT/DELETE /bookmarks*`
  — all behave exactly as before (same status codes, same response shape). This directly confirms
  "existing bookmarks must keep working."
- **New endpoints:** created collections, renamed one, added three bookmarks to a collection
  (got them back in insertion order), added a duplicate (409), added a nonexistent bookmark id
  (404), moved a bookmark to position 0 and confirmed the returned order reflected it, attempted
  an out-of-range move (400) and a move on a non-member (404), removed a bookmark (204) and
  confirmed the remaining list compacted correctly, confirmed the same bookmark still appeared in
  a *second* collection it also belonged to (many-to-many, requirement 1/2), deleted a collection
  and confirmed its bookmarks still existed via `GET /bookmarks` (cascade only removed the
  membership, not the bookmark), and finally deleted a bookmark that was still a member of a
  collection and confirmed the membership row disappeared too (cascade in the other direction).
- This test run is also what surfaced the multi-statement `Statement.execute()` bug described in
  §1 — the first full run against a fresh DB failed every collections call with
  `no such table: collections` until `Db.init()` was fixed to execute each `CREATE TABLE`
  individually.
