# Curl test run results

**Date:** 2026-11-05  
**Server:** `DB_PATH=bookmarks-test.db` `PORT=8081` (`go run .`)  
**Base URL:** `http://localhost:8081`  
**Automated tests:** `go test ./... -race -count=1` → **PASS** (`ok bookmarks 1.402s`)

All manual curls below match expectations from **`testing.md`** / Plan §4.3.

---

## Summary

| ID | Test | Expected HTTP | Actual HTTP | Pass |
|----|------|---------------|-------------|------|
| 1.1 | Create bookmark A | 201 | 201 | ✓ |
| 1.2 | Create bookmark B | 201 | 201 | ✓ |
| 1.3 | Create bookmark C | 201 | 201 | ✓ |
| 2.1 | Create collection | 201 | 201 | ✓ |
| 2.2 | List collections | 200 | 200 | ✓ |
| 2.3 | Get collection 1 | 200 | 200 | ✓ |
| 2.4 | Update collection 1 | 200 | 200 | ✓ |
| 3.1 | Add bookmark 1 to collection | 201 | 201 | ✓ |
| 3.2 | Add bookmark 2 | 201 | 201 | ✓ |
| 3.3 | Add bookmark 3 | 201 | 201 | ✓ |
| 3.4 | Re-add bookmark 1 | 200 | 200 | ✓ |
| 3.5 | List collection bookmarks (A,B,C) | 200 | 200 | ✓ |
| 3.6 | Remove bookmark 2 | 204 | 204 | ✓ |
| 3.7 | Remove bookmark 2 again | 204 | 204 | ✓ |
| 3.8 | List after remove (A,C) | 200 | 200 | ✓ |
| 4.0 | Re-add bookmark 2 | 201 | 201 | ✓ |
| 4.1 | Move bookmark 3 to position 0 | 200 | 200 | ✓ |
| 4.2 | List after move (C,A,B) | 200 | 200 | ✓ |
| 4.3 | Move noop same position | 200 | 200 | ✓ |
| 4.4 | Bad position 99 | 400 | 400 | ✓ |
| 5.1 | Empty collection name | 400 | 400 | ✓ |
| 5.2 | Get collection 9999 | 404 | 404 | ✓ |
| 5.3 | Add missing bookmark 9999 | 404 | 404 | ✓ |
| 5.4 | Move empty body | 400 | 400 | ✓ |
| 6.1 | List all bookmarks | 200 | 200 | ✓ |
| 6.2 | Get bookmark 1 | 200 | 200 | ✓ |
| 6.3 | Delete bookmark 1 | 204 | 204 | ✓ |
| 6.4 | Delete bookmark 1 again | 204 | 204 | ✓ |
| 6.5 | Delete collection 1 | 204 | 204 | ✓ |
| 6.6 | Delete collection 1 again | 204 | 204 | ✓ |

**Overall:** 28/28 passed

---

## Detailed responses

### 1 — Setup bookmarks

**1.1** `POST /bookmarks` (A)

```json
{"id":1,"url":"https://a.example","title":"A","description":"","created_at":"2026-11-05 11:06:28"}
```

HTTP **201**

**1.2** `POST /bookmarks` (B) — HTTP **201**, `"id":2`

**1.3** `POST /bookmarks` (C) — HTTP **201**, `"id":3`

---

### 2 — Collection CRUD

**2.1** `POST /collections`

```json
{"id":1,"name":"Reading list","created_at":"2026-11-05 11:06:28"}
```

HTTP **201**

**2.2** `GET /collections` — HTTP **200**, one collection in array

**2.3** `GET /collections/1` — HTTP **200**

**2.4** `PUT /collections/1` — HTTP **200**, `"name":"Reading list (2026)"`

---

### 3 — Membership

**3.1** `PUT /collections/1/bookmarks/1` — HTTP **201**, bookmark JSON body

**3.2** `PUT .../bookmarks/2` — HTTP **201**

**3.3** `PUT .../bookmarks/3` — HTTP **201**

**3.4** `PUT .../bookmarks/1` — HTTP **200**

**3.5** `GET /collections/1/bookmarks` — HTTP **200**, ids `[1,2,3]` in order

**3.6** `DELETE .../bookmarks/2` — HTTP **204**

**3.7** `DELETE .../bookmarks/2` — HTTP **204**

**3.8** `GET /collections/1/bookmarks` — HTTP **200**, ids `[1,3]`

---

### 4 — Move / reorder

**4.0** `PUT .../bookmarks/2` — HTTP **201**

**4.1** `PUT .../bookmarks/3/position` `{"position":0}` — HTTP **200**, bookmark C JSON

**4.2** `GET /collections/1/bookmarks` — HTTP **200**, ids `[3,1,2]`

**4.3** Same move again — HTTP **200**

**4.4** `{"position":99}` — HTTP **400**

```json
{"error":"position out of range"}
```

---

### 5 — Validation & 404

**5.1** `POST /collections` `{"name":""}` — HTTP **400**

```json
{"error":"name is required"}
```

**5.2** `GET /collections/9999` — HTTP **404**

```json
{"error":"collection not found"}
```

**5.3** `PUT /collections/1/bookmarks/9999` — HTTP **404**

**5.4** `PUT .../position` `{}` — HTTP **400**

```json
{"error":"position is required"}
```

---

### 6 — Bookmark regression & idempotent deletes

**6.1** `GET /bookmarks` — HTTP **200**, 3 bookmarks

**6.2** `GET /bookmarks/1` — HTTP **200**

**6.3** `DELETE /bookmarks/1` — HTTP **204**

**6.4** `DELETE /bookmarks/1` — HTTP **204**

**6.5** `DELETE /collections/1` — HTTP **204**

**6.6** `DELETE /collections/1` — HTTP **204**

---

## Notes

- Fresh DB assumed ids **1** for first bookmark and collection.
- Run used port **8081** because **8080** was already in use on the machine.
- Re-run: see commands in **`testing.md`**.
