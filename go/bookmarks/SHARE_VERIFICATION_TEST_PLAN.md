# Public share links — verification test plan

Checklist for read-only public links on a **single bookmark** or a **whole collection** (ordered bookmark list). Each case has a **description** and **expected outcome** for store tests, HTTP tests, or manual checks.

Pair with **`VERIFICATION_TEST_PLAN.md`** for bookmarks, collections, membership, and ordering — those behaviors must stay intact after adding shares.

---

## Conventions (from current implementation)

| Topic | Behavior |
|--------|----------|
| Create bookmark share | `POST /bookmarks/{id}/share` |
| Create collection share | `POST /collections/{id}/share` |
| Public read | `GET /public/{token}` — **no credentials** |
| Revoke | `DELETE /public/{token}` — **no credentials** (anyone who knows the token can revoke) |
| Create response | **201**, JSON `ShareLink`: `token`, `kind`, `bookmark_id` or `collection_id`, `created_at`, `public_path` (`/public/{token}`), optional `expires_at`, `revoked_at` when set |
| Public response | **200**, JSON `PublicShareView`: `kind` + `bookmark` **or** `collection` + `bookmarks` |
| Inactive link | **404** `{"error":"share not found"}` — unknown token, revoked, expired, or backing resource gone |
| Optional expiry body | Empty body or zero `Content-Length` → no expiry. JSON object optional. |
| `expires_at` | RFC3339; stored/compared in UTC; inactive when `now >= expires_at` |
| `ttl_seconds` | Positive integer; expiry = `now + ttl` at parse time (UTC). If both `ttl_seconds` and `expires_at` are sent, **`ttl_seconds` wins** |
| Multiple links | Same bookmark/collection may have **many** active share rows (new token each POST) |
| FK cleanup | Deleting bookmark/collection **CASCADE** deletes related `share_links` rows |
| Path ids | Invalid `{id}` on create → **404** `"bookmark not found"` / `"collection not found"` |

---

## 1. Regression — shares must not break existing API

Run a **smoke subset** of `VERIFICATION_TEST_PLAN.md` after share work (or full suite before release).

### 1.1 Bookmarks CRUD unchanged

**Description:** Create, list, get, update, delete bookmarks via `/bookmarks` routes.

**Expected:** Same status codes and validation as before; no new fields required on bookmark payloads.

### 1.2 Collections CRUD unchanged

**Description:** Create/list/get/update/delete collections; empty name still **400**.

**Expected:** Unchanged behavior.

### 1.3 Membership and ordering unchanged

**Description:** Add/remove bookmarks in collections; list order; move position; idempotent add/remove.

**Expected:** Unchanged behavior; share routes do not alter membership or positions.

### 1.4 Route table intact

**Description:** All routes in `registerRoutes` still respond (not shadowed by `/public/...`).

**Expected:** Bookmark and collection endpoints behave as in the collections verification doc; wrong methods still **405** where applicable.

### 1.5 Share create does not mutate resource

**Description:** POST share on bookmark B and collection C; GET bookmark/collection and collection bookmark list before vs after.

**Expected:** Identical data; only new row in `share_links`.

---

## 2. Create share — bookmark

### 2.1 Create with empty body

**Description:** Create bookmark; `POST /bookmarks/{id}/share` with no body (or `Content-Length: 0`).

**Expected:** **201**; `kind` `"bookmark"`; `bookmark_id` matches; `public_path` `/public/{token}`; `token` non-empty; no `expires_at` (or omitted); `revoked_at` absent/null on create.

### 2.2 Public read — bookmark payload

**Description:** Use `token` from 2.1; `GET /public/{token}`.

**Expected:** **200**; `kind` `"bookmark"`; `bookmark` object matches `GET /bookmarks/{id}` (url, title, description, id); no `collection` / `bookmarks` fields required (may be omitted).

### 2.3 Create — bookmark not found

**Description:** `POST /bookmarks/99999/share` and `POST /bookmarks/not-a-number/share`.

**Expected:** **404** `"bookmark not found"`.

### 2.4 Multiple shares for one bookmark

**Description:** POST share twice on the same bookmark without revoking.

**Expected:** Two **201** responses with **different** `token` values; both `GET /public/{token}` return **200** until revoked/expired independently.

### 2.5 Create with `{}`

**Description:** POST share with body `{}`.

**Expected:** **201**, no expiry (same as empty body).

### 2.6 Invalid JSON on create

**Description:** POST share with body `[]` or `not-json`.

**Expected:** **400** `"body must be a JSON object"`.

---

## 3. Create share — collection

### 3.1 Create with empty body

**Description:** Create collection (with or without members); `POST /collections/{id}/share`.

**Expected:** **201**; `kind` `"collection"`; `collection_id` set; `public_path` present.

### 3.2 Public read — collection + ordered bookmarks

**Description:** Collection with at least two bookmarks in a **known** order (via add sequence and/or move); create share; `GET /public/{token}`.

**Expected:** **200**; `kind` `"collection"`; `collection.name` matches; `bookmarks` array order matches `GET /collections/{id}/bookmarks` (same ids in same order).

### 3.3 Empty collection share

**Description:** New collection, no members; create share; GET public.

**Expected:** **201** on create; **200** on GET with `bookmarks: []` (not `null`).

### 3.4 Create — collection not found

**Description:** `POST /collections/99999/share` and invalid path id.

**Expected:** **404** `"collection not found"`.

### 3.5 Multiple shares for one collection

**Description:** Two POST shares on same collection.

**Expected:** Independent tokens; both work until revoked.

---

## 4. Expiry — `expires_at` and `ttl_seconds`

### 4.1 No expiry — link stays valid

**Description:** Create share without expiry; GET public immediately and after delay (no revoke).

**Expected:** **200** each time (until resource deleted or revoked).

### 4.2 Expires in the past at create time

**Description:** POST `{"expires_at":"<RFC3339 one hour ago>"}`.

**Expected:** **201** (link row exists); immediate `GET /public/{token}` → **404** `"share not found"`.

### 4.3 Expires in the future

**Description:** POST with `expires_at` one hour from now.

**Expected:** **201**; GET public **200** now; store-level check with `ResolvePublicShare(token, now+2h)` → `ErrShareNotFound` (or HTTP GET after wall-clock expiry → **404**).

### 4.4 Expiry boundary (exact time)

**Description:** Store test: set `expires_at` = `T`; resolve with `now = T` (UTC).

**Expected:** Inactive — **404** / `ErrShareNotFound` (`now >= expires_at`).

### 4.5 Invalid `expires_at`

**Description:** POST `{"expires_at":"tomorrow"}`, `{"expires_at":""}`, or non-RFC3339 string.

**Expected:** **400** `"expires_at must be a valid RFC3339 timestamp"`.

### 4.6 `ttl_seconds` happy path

**Description:** POST `{"ttl_seconds":3600}`.

**Expected:** **201**; response includes `expires_at` roughly now+1h (RFC3339 UTC); GET public **200** before TTL; after TTL elapsed → **404**.

### 4.7 `ttl_seconds` validation

**Description:** POST `{"ttl_seconds":0}` and `{"ttl_seconds":-1}`.

**Expected:** **400** `"ttl_seconds must be positive"`.

### 4.8 `ttl_seconds` overrides `expires_at`

**Description:** POST body with both `"ttl_seconds":60` and `"expires_at":"<far future>"`.

**Expected:** Effective expiry ~60s from create (TTL wins); public GET fails after ~60s, not at far-future time.

### 4.9 Timezone in `expires_at`

**Description:** POST `{"expires_at":"2030-01-01T00:00:00+05:30"}` (valid RFC3339).

**Expected:** **201**; parsed and stored consistently; active/inactive matches UTC interpretation.

---

## 5. Revoke

### 5.1 Revoke active link

**Description:** Create share; `DELETE /public/{token}`.

**Expected:** **204**; following `GET /public/{token}` → **404** `"share not found"`.

### 5.2 Revoke idempotent

**Description:** DELETE same token again after revoke.

**Expected:** **204** (no error body).

### 5.3 Revoke unknown token

**Description:** `DELETE /public/{random-valid-format-but-unknown-token}`.

**Expected:** **204** (idempotent revoke; no distinction from revoked token — by design).

### 5.4 Revoke does not delete bookmark/collection

**Description:** Revoke bookmark share; GET bookmark still **200**; collection share revoke leaves collection and members intact.

**Expected:** Only link inactive; resources unchanged.

### 5.5 One of multiple shares

**Description:** Two tokens for same bookmark; revoke token A only.

**Expected:** A → **404** on GET; B still **200**.

---

## 6. Public access — read-only and errors

### 6.1 No auth required for GET

**Description:** GET public without any headers/cookies.

**Expected:** **200** when link active.

### 6.2 Public endpoint is read-only

**Description:** `POST`, `PUT`, `PATCH` on `/public/{token}`.

**Expected:** **405** (or consistent non-mutating response); must not revoke or change data except via `DELETE`.

### 6.3 Unknown / garbage token

**Description:** `GET /public/does-not-exist`.

**Expected:** **404** `"share not found"`.

### 6.4 GET after revoke

**Description:** Covered in 5.1; explicit error body shape.

**Expected:** JSON `error` key `"share not found"`.

### 6.5 No write via public GET

**Description:** Confirm response has no side effects (repeat GET; bookmark list count unchanged).

**Expected:** Idempotent read only.

### 6.6 Public view scope — bookmark link

**Description:** DB has multiple bookmarks; share only B.

**Expected:** Public JSON includes only B, not other bookmarks.

### 6.7 Public view scope — collection link

**Description:** Two collections with different members; share C1 only.

**Expected:** Public payload only C1 and its members, in order.

---

## 7. Live data vs snapshot

Shares resolve **current** data at GET time (not a frozen copy).

### 7.1 Bookmark share sees updates

**Description:** Create bookmark share; `PUT /bookmarks/{id}` changing title; GET public again.

**Expected:** **200**; public `bookmark.title` matches latest PUT.

### 7.2 Collection share sees membership changes

**Description:** Share collection; add new bookmark to collection; GET public.

**Expected:** New bookmark appears at end of `bookmarks` (per current order rules).

### 7.3 Collection share sees removal

**Description:** Share collection with [A,B]; remove B via membership DELETE; GET public.

**Expected:** **200**; list `[A]` only.

### 7.4 Collection share sees reorder

**Description:** Share ordered list; move bookmark via position endpoint; GET public.

**Expected:** Order matches `GET /collections/{id}/bookmarks`.

### 7.5 Collection metadata update

**Description:** Share collection; `PUT /collections/{id}` rename; GET public.

**Expected:** `collection.name` updated on public view.

### 7.6 Empty collection after share

**Description:** Share non-empty collection; remove all members; GET public.

**Expected:** **200**, `bookmarks: []` (collection row still exists).

---

## 8. Resource deletion and cascades

### 8.1 Delete bookmark invalidates bookmark share

**Description:** Create share for B; `DELETE /bookmarks/{b}`; GET public.

**Expected:** **404** `"share not found"` (row removed via CASCADE, or resolve treats missing bookmark as not found).

### 8.2 Delete bookmark does not affect other shares

**Description:** Share B1 and B2; delete B1 only.

**Expected:** B1 link **404**; B2 link still **200**.

### 8.3 Delete collection invalidates collection share

**Description:** Share collection; `DELETE /collections/{id}`; GET public.

**Expected:** **404** `"share not found"`.

### 8.4 Delete collection does not delete bookmarks

**Description:** After 8.3, `GET /bookmarks` still lists former members.

**Expected:** Bookmarks still exist (only collection and junction rows removed).

### 8.5 Bookmark deleted from shared collection only

**Description:** Share collection; delete one member bookmark globally (`DELETE /bookmarks/{id}`).

**Expected:** Public GET **200**; list no longer includes deleted bookmark; order of survivors preserved.

---

## 9. Interaction with collections edge cases

These ensure shares don’t mask ordering/membership bugs from the collections feature.

### 9.1 Share then add with position gap scenario

**Description:** If collection has non-contiguous positions after bookmark DELETE (see collections plan §6.2): share collection, GET public order still matches authenticated list.

**Expected:** Public `bookmarks` order identical to `GET /collections/{id}/bookmarks` (even if internal positions are gapped).

### 9.2 Share collection with duplicate membership impossible

**Description:** Normal add idempotency; public list never duplicates same bookmark id.

**Expected:** Each id at most once in public array.

---

## 10. Security & robustness (behavioral, not pentest)

### 10.1 Token entropy

**Description:** Create many shares; tokens unique (32 random bytes, base64url).

**Expected:** No collisions in reasonable sample; length stable.

### 10.2 Token URL-safe path

**Description:** GET `/public/{token}` where token contains `-`, `_` (base64url).

**Expected:** **200** when active (mux path captures full token).

### 10.3 Revoke without ever GETting

**Description:** POST create → DELETE revoke without intermediate GET.

**Expected:** **204**; GET → **404**.

### 10.4 Create share wrong HTTP method

**Description:** `GET /bookmarks/1/share`, `PUT /collections/1/share`.

**Expected:** **405**; no share row created.

---

## 11. Store-level tests (optional, `ResolvePublicShare` / `shareActive`)

Use injected `now` for deterministic expiry/revoke checks.

### 11.1 `shareActive` — revoked timestamp set

**Description:** Revoke link; resolve with any `now`.

**Expected:** `ErrShareNotFound`.

### 11.2 `GetShareLink` missing token

**Description:** Resolve random token.

**Expected:** `nil` link → `ErrShareNotFound`.

### 11.3 Malformed `kind` in DB (if injectable)

**Description:** If row had invalid `kind`, resolve would hit default case.

**Expected:** `ErrShareNotFound` (defensive).

---

## 12. Priority map

| Priority | Cases | Rationale |
|----------|--------|-----------|
| P0 | 2.1–2.3, 3.2–3.4, 4.2–4.3, 5.1–5.2, 6.3, 7.1–7.4, 8.1–8.3 | Core create/read/revoke/expiry + live collection order |
| P1 | 2.4, 3.3, 4.5–4.8, 5.5, 6.6–6.7, 8.4–8.5, 1.1–1.5 | Validation, multi-link, regression |
| P2 | 4.4, 4.9, 9.x, 10.x, 11.x | Boundaries, cross-feature, hardening |

---

## 13. Already covered by `shares_test.go` (extend, don’t duplicate blindly)

| Existing test | Covers |
|---------------|--------|
| `TestShareBookmarkPublic` | 2.1–2.2, 5.1–5.2, 5.3 (revoke again) |
| `TestShareCollectionOrdered` | 3.2 (order B before A) |
| `TestShareExpires` | 4.2 |
| `TestShareTTL` | 4.3 (store, fixed clock via `now` param) |

**High-value gaps to implement next:** 4.6–4.8 (HTTP TTL), 2.4 / 5.5 (multiple tokens), 7.1–7.4 (live updates), 8.1–8.5 (cascades), 3.3 (empty collection), 2.3 / 3.4 (404 create), 1.x regression slice.

---

## 14. Manual smoke script

1. Create bookmark → POST share → copy `public_path` → open GET in “incognito” (no special headers) → see bookmark.
2. DELETE public URL → GET again → 404.
3. Create collection, add two bookmarks in defined order → POST collection share → GET public → verify order matches `/collections/{id}/bookmarks`.
4. Rename bookmark → refresh public GET → see new title.
5. Run `go test ./...` including `VERIFICATION_TEST_PLAN`-driven tests in `verification_test.go` if present.

Document failures with: create response, token, GET status/body snippet, and authenticated list for comparison.

---

## 15. Automated coverage (`share_verification_test.go`)

Run: `go test -run TestShareVerificationPlan -v`

| Section | Cases | Notes |
|---------|-------|--------|
| §1 | 1.1–1.5 (smoke) | Full collections regression remains in `TestVerificationPlan` |
| §2 | 2.1–2.6 | |
| §3 | 3.1–3.5 | |
| §4 | 4.2, 4.4–4.9 | 4.1/4.3 overlap `TestShareTTL`; 4.6 checks `expires_at` on create |
| §5 | 5.1–5.2 in `shares_test.go`; 5.3–5.5 here | |
| §6 | 6.2–6.3, 6.6–6.7 | 6.1/6.4–6.5 implicit |
| §7 | 7.1–7.5 | 7.6 same as 3.3 empty `bookmarks` |
| §8 | 8.1–8.3, 8.5 | 8.4 in verification plan |
| §9 | 9.1–9.2 | Gap order + no dup ids |
| §10 | 10.1, 10.3–10.4 | |
| §11 | 11.1–11.2 | |

**Fix applied during verification:** `PublicShareView.bookmarks` must not use `json:"omitempty"` so empty collections return `"bookmarks":[]` (plan §3.3).

**Fix applied:** `parseShareInput` uses `io.EOF` instead of `Content-Length == 0` for empty bodies.

