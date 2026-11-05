# Collections & ordering — verification test plan

Fresh-eyes checklist for the bookmark manager extension. Each case has a **description** and **expected outcome** so you can implement or run them as store-level tests, HTTP integration tests, or manual `curl` checks.

## Conventions (from current implementation)

| Topic | Behavior |
|--------|----------|
| Base URL | `http://localhost:8080` (or `PORT`) |
| Errors | JSON `{"error":"<message>"}` |
| Collection name | Required after `TrimSpace`; whitespace-only → 400 |
| Positions | **0-based**, valid range `[0, n-1]` where `n` = bookmarks in that collection |
| Add membership | `PUT /collections/{id}/bookmarks/{bookmark_id}` — **201** first time, **200** if already in collection |
| Remove membership | `DELETE ...` — **204** even if bookmark was not in collection (idempotent) |
| Delete bookmark/collection | **204** even when id does not exist (matches existing bookmark handlers) |
| Empty lists | JSON array `[]`, not `null` |
| Path ids | Non-numeric `{id}` / `{bookmark_id}` → **404** with `"collection not found"` or `"bookmark not found"` (depending on route) |

---

## 1. Regression — existing bookmarks API

These must still pass unchanged after collections work.

### 1.1 List bookmarks on empty DB

**Description:** `GET /bookmarks` on a fresh database.

**Expected:** **200**, body `[]`.

### 1.2 Create bookmark — happy path

**Description:** `POST /bookmarks` with `{"url":"https://example.com","title":"Example","description":"note"}`.

**Expected:** **201**, JSON includes `id`, `url`, `title`, `description`, `created_at`.

### 1.3 Create bookmark — validation

**Description:** POST with missing/empty `url` or `title` (including `"url":"   "` after trim).

**Expected:** **400**, error mentions required field.

### 1.4 Create bookmark — invalid JSON

**Description:** POST with body `not-json` or `[]`.

**Expected:** **400**, `"body must be a JSON object"`.

### 1.5 Get / update / delete bookmark

**Description:** Create a bookmark; `GET /bookmarks/{id}` returns it; `PUT` with valid body updates; `DELETE` returns **204**; subsequent `GET` → **404** `"bookmark not found"`.

**Expected:** Status codes and messages as above; list endpoint no longer includes deleted id.

### 1.6 Get bookmark — invalid id

**Description:** `GET /bookmarks/abc` and `GET /bookmarks/99999` (non-existent numeric id).

**Expected:** **404** `"bookmark not found"` for both (invalid path id is treated as not found, not 400).

---

## 2. Collection CRUD

### 2.1 Create collection — happy path

**Description:** `POST /collections` with `{"name":"Reading list"}`.

**Expected:** **201**, body has `id > 0`, `name`, `created_at`.

### 2.2 Create collection — name required

**Description:** POST with `{}`, `{"name":""}`, or `{"name":"   "}`.

**Expected:** **400**, `"name is required"`.

### 2.3 Create collection — invalid JSON

**Description:** POST with non-object body.

**Expected:** **400**, `"body must be a JSON object"`.

### 2.4 List collections

**Description:** Create two collections; `GET /collections`.

**Expected:** **200**, array length 2, ordered by `id` ascending (stable creation order).

### 2.5 Get collection — happy path

**Description:** `GET /collections/{id}` for an existing id.

**Expected:** **200**, same fields as create response.

### 2.6 Get collection — not found

**Description:** `GET /collections/99999` and `GET /collections/not-a-number`.

**Expected:** **404** `"collection not found"`.

### 2.7 Update collection — happy path

**Description:** `PUT /collections/{id}` with `{"name":"Renamed"}`.

**Expected:** **200**, `name` updated; `GET` reflects new name.

### 2.8 Update collection — not found

**Description:** `PUT /collections/99999` with valid name body.

**Expected:** **404** `"collection not found"`.

### 2.9 Update collection — validation

**Description:** PUT with empty/whitespace name on existing collection.

**Expected:** **400** `"name is required"`; name unchanged on subsequent GET.

### 2.10 Delete collection — happy path

**Description:** `DELETE /collections/{id}` for existing collection.

**Expected:** **204**; `GET /collections/{id}` → **404**.

### 2.11 Delete collection — idempotent

**Description:** DELETE same id twice, or DELETE non-existent numeric id.

**Expected:** **204** both times (no error body).

### 2.12 Delete collection — invalid path id

**Description:** `DELETE /collections/abc`.

**Expected:** **404** `"collection not found"`.

---

## 3. Membership — add & remove

### 3.1 Add bookmark to collection — first add

**Description:** Create collection C and bookmark B; `PUT /collections/{c}/bookmarks/{b}` with empty body.

**Expected:** **201**, response body is the bookmark JSON (same shape as `GET /bookmarks/{b}`).

### 3.2 Add bookmark — append order

**Description:** Add B1, then B2, then B3 to the same collection (each first add).

**Expected:** `GET /collections/{c}/bookmarks` returns `[B1, B2, B3]` in that order.

### 3.3 Add bookmark — idempotent re-add

**Description:** PUT same `(collection, bookmark)` again without removing.

**Expected:** **200** (not 201), same bookmark JSON; list order unchanged; no duplicate rows (still length 3 in 3.2 setup).

### 3.4 Add bookmark — collection not found

**Description:** PUT `/collections/99999/bookmarks/{validBookmarkId}`.

**Expected:** **404** `"collection not found"`.

### 3.5 Add bookmark — bookmark not found

**Description:** PUT `/collections/{validCollectionId}/bookmarks/99999`.

**Expected:** **404** `"bookmark not found"`.

### 3.6 Add bookmark — invalid path ids

**Description:** PUT `/collections/abc/bookmarks/1` and PUT `/collections/1/bookmarks/xyz`.

**Expected:** **404** with appropriate `"collection not found"` / `"bookmark not found"` message.

### 3.7 Same bookmark in multiple collections

**Description:** Create C1, C2 and bookmark B; add B to C1 then to C2.

**Expected:** Both succeed; `GET /collections/{c1}/bookmarks` and `GET /collections/{c2}/bookmarks` each contain B; orders are independent (see §5.2).

### 3.8 Remove bookmark from collection — happy path

**Description:** With [B1, B2, B3] in C, DELETE `/collections/{c}/bookmarks/{b2}`.

**Expected:** **204**; list becomes `[B1, B3]` in that order.

### 3.9 Remove bookmark — reindex positions

**Description:** After 3.8, add new bookmark B4 to C.

**Expected:** List `[B1, B3, B4]` with no gaps in **display order**; add succeeds (**201**), not 500 from unique `(collection_id, position)` violation.

### 3.10 Remove bookmark — idempotent

**Description:** DELETE same `(collection, bookmark)` again, or DELETE bookmark not in collection but collection exists.

**Expected:** **204** each time.

### 3.11 Remove bookmark — collection not found

**Description:** DELETE `/collections/99999/bookmarks/{id}`.

**Expected:** **404** `"collection not found"`.

### 3.12 Remove bookmark — invalid bookmark path id

**Description:** DELETE `/collections/1/bookmarks/abc`.

**Expected:** **404** `"bookmark not found"`.

---

## 4. List bookmarks in a collection

### 4.1 Empty collection

**Description:** New collection, no members; `GET /collections/{id}/bookmarks`.

**Expected:** **200**, `[]`.

### 4.2 Ordered list

**Description:** After adds in known order, GET list.

**Expected:** **200**, ids match add order, full bookmark objects (url, title, etc.).

### 4.3 List — collection not found

**Description:** `GET /collections/99999/bookmarks`.

**Expected:** **404** `"collection not found"`.

### 4.4 List — invalid collection id

**Description:** `GET /collections/foo/bookmarks`.

**Expected:** **404** `"collection not found"`.

### 4.5 List after moves only

**Description:** Build list of 3+, reorder only via move endpoint (§5); never remove.

**Expected:** GET order always matches last successful move sequence.

---

## 5. Move bookmark within collection

Endpoint: `PUT /collections/{id}/bookmarks/{bookmark_id}/position`  
Body: `{"position": <int>}` (field required; `null` or omitted → 400).

### 5.1 Move to front

**Description:** Order [A, B, C]; move C to position `0`.

**Expected:** **200**, bookmark JSON for C; list `[C, A, B]`.

### 5.2 Move to back

**Description:** Order [A, B, C]; move A to position `2`.

**Expected:** List `[B, C, A]`.

### 5.3 Move down one slot

**Description:** Order [A, B, C]; move A to position `1`.

**Expected:** List `[B, A, C]`.

### 5.4 Move to same position (no-op)

**Description:** Move B to its current index (e.g. position `1` when B is already at index 1).

**Expected:** **200**, bookmark unchanged; list order unchanged.

### 5.5 Move in second collection unchanged

**Description:** B in C1 (order [A, B]) and C2 (order [B, X]); move B within C1 only.

**Expected:** C1 order updates; C2 order still `[B, X]`.

### 5.6 Move — position out of range (high)

**Description:** Collection has `n` members; move with `"position": n` or `99`.

**Expected:** **400** `"position out of range"`; list unchanged.

### 5.7 Move — position out of range (negative)

**Description:** `"position": -1`.

**Expected:** **400** `"position out of range"`.

### 5.8 Move — bookmark not in collection

**Description:** Bookmark exists but was never added to this collection (or was removed); move with valid position `0`.

**Expected:** **404** `"bookmark not in collection"`.

### 5.9 Move — bookmark does not exist

**Description:** Move `/collections/{c}/bookmarks/99999/position` with `{"position":0}`.

**Expected:** **404** `"bookmark not found"`.

### 5.10 Move — collection does not exist

**Description:** Move on `/collections/99999/bookmarks/{b}/position`.

**Expected:** **404** `"collection not found"`.

### 5.11 Move — empty collection

**Description:** Collection with zero members; existing bookmark B; move B to `0`.

**Expected:** **404** `"bookmark not in collection"` (not position out of range).

### 5.12 Move — single-member collection

**Description:** Only one bookmark in collection; move to position `0`.

**Expected:** **200**, order unchanged.

### 5.13 Move — request validation

**Description:** Body `{}`, `{"position":null}`, invalid JSON, or non-object.

**Expected:** **400** — `"position is required"` or `"body must be a JSON object"`.

### 5.14 Move — invalid path ids

**Description:** `/collections/x/bookmarks/1/position` and `/collections/1/bookmarks/y/position` with valid body.

**Expected:** **404** with correct error key.

---

## 6. Cascades & lifecycle (high risk)

These catch FK/CASCADE and position-index bugs that unit tests often miss.

### 6.1 Delete bookmark removes membership rows

**Description:** Add B to C; `DELETE /bookmarks/{b}`; `GET /collections/{c}/bookmarks`.

**Expected:** **200**, `[]` (membership gone; collection still exists).

### 6.2 Delete bookmark then add another to same collection

**Description:** C contains [B1, B2, B3]; delete B2 via `DELETE /bookmarks/{b2}`; add B4 via PUT membership.

**Expected:** Add returns **201** (not 500); list order is `[B1, B3, B4]` (B3 still after B1). **Critical:** must not hit UNIQUE constraint on `(collection_id, position)`.

### 6.3 Delete collection removes memberships only

**Description:** C with members; delete C; bookmarks still listed on `GET /bookmarks`.

**Expected:** Collection gone; bookmarks remain; other collections unaffected.

### 6.4 Delete collection — list bookmarks on deleted id

**Description:** After deleting collection, `GET /collections/{id}/bookmarks`.

**Expected:** **404** `"collection not found"`.

---

## 7. Concurrency & transactions (optional, store-level)

### 7.1 Parallel add two different bookmarks

**Description:** Two goroutines/requests: add B1 and B2 to empty collection C at the same time.

**Expected:** Both succeed; final list length 2; distinct positions; no duplicate positions.

### 7.2 Parallel add same bookmark

**Description:** Two concurrent PUTs adding the same B to C.

**Expected:** One **201**, one **200** (or both **200**); exactly one membership row; no error.

---

## 8. HTTP method & route smoke

### 8.1 Wrong method on collection routes

**Description:** e.g. `POST /collections/1`, `GET /collections/1/bookmarks/1`, `PATCH` where only PUT is defined.

**Expected:** Go 1.22+ mux returns **405 Method Not Allowed** (or consistent not-routed behavior); must not mutate state.

### 8.2 Route wiring sanity

**Description:** Confirm these paths exist and respond (not 404 from missing route):

- `GET /collections`
- `POST /collections`
- `GET|PUT|DELETE /collections/{id}`
- `GET /collections/{id}/bookmarks`
- `PUT|DELETE /collections/{id}/bookmarks/{bookmark_id}`
- `PUT /collections/{id}/bookmarks/{bookmark_id}/position`

**Expected:** Defined methods return app status codes (2xx/4xx), not mux “404 page not found” for wrong path pattern.

---

## 9. Suggested implementation map

| Priority | Cases | Rationale |
|----------|--------|-----------|
| P0 | 3.9, 6.2, 5.1–5.8, 3.1–3.6, 2.2, 4.1 | Core feature + position integrity |
| P1 | 6.1, 6.3, 3.7, 5.5, 5.12, 5.13 | Multi-collection, cascades, validation |
| P2 | 1.x, 8.x, 7.x | Regression, routing, concurrency |

---

## 10. Quick manual script pattern

For exploratory checks, use a empty DB (`DB_PATH=:memory:` or delete `bookmarks.db`), start the server, then:

1. Create two collections and three bookmarks.
2. Add all three to collection 1; verify list order.
3. Move middle item to index 0; verify list.
4. Remove middle item via DELETE membership; add a fourth bookmark; verify no 500.
5. Delete a bookmark that sits **between** two others in a collection; add a new bookmark to that collection; verify add succeeds and order is sensible (case **6.2**).

Document any failure with request, response status, body, and collection list snapshot — that makes gaps easy to turn into automated tests later.

---

## 11. Automated coverage (`verification_test.go`)

All cases **§1–§8** are implemented as subtests under **`TestVerificationPlan`** (run with `go test -run TestVerificationPlan -v`).

| Section | Cases | Test prefix |
|---------|-------|-------------|
| §1 | 1.1–1.6 | `TestVerificationPlan/1_regression_bookmarks/1.x_*` |
| §2 | 2.1–2.12 | `TestVerificationPlan/2_collection_crud/2.x_*` |
| §3 | 3.1–3.12 | `TestVerificationPlan/3_membership/3.x_*` |
| §4 | 4.1–4.5 | `TestVerificationPlan/4_list_in_collection/4.x_*` |
| §5 | 5.1–5.14 | `TestVerificationPlan/5_move/5.x_*` |
| §6 | 6.1–6.4 | `TestVerificationPlan/6_cascades/6.x_*` |
| §7 | 7.1–7.2 | `TestVerificationPlan/7_concurrency/7.x_*` |
| §8 | 8.1–8.2 | `TestVerificationPlan/8_routing/8.x_*` |

**§8.1** accepts **405** or **404** depending on Go `ServeMux` behavior for wrong methods.

**Bug found via §6.2:** append-after-CASCADE used `COUNT(*)` for next position, colliding with gap positions after `DELETE /bookmarks/{id}`. Fixed to `MAX(position)+1` in `AddCollectionBookmark`.

