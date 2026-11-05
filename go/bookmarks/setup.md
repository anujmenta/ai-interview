# Bookmark Manager — local setup

Run the existing Go REST API and smoke-test it with `curl`.

## Prerequisites

- **Go 1.26+** (see `go.mod`)
- Terminal with `curl`

## 1. Start the server

From the repo root:

```bash
cd go/bookmarks
go mod tidy
go run .
```

You should see:

```text
listening on http://localhost:8080
```

Leave this process running. Stop it with `Ctrl+C`.

### Optional environment variables

| Variable  | Default        | Purpose              |
|-----------|----------------|----------------------|
| `PORT`    | `8080`         | HTTP listen port     |
| `DB_PATH` | `bookmarks.db` | SQLite database file |

Example:

```bash
PORT=3000 DB_PATH=/tmp/bookmarks.db go run .
```

## 2. Try the APIs (`curl`)

Set a base URL (default port):

```bash
export BASE=http://localhost:8080
```

### Create a bookmark — `POST /bookmarks`

Required JSON fields: `url`, `title`. Optional: `description`.

```bash
curl -s -X POST "$BASE/bookmarks" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev","title":"Go","description":"Go language site"}'
```

Expected: **201** and a JSON bookmark (includes `id`, `created_at`).

### List bookmarks — `GET /bookmarks`

```bash
curl -s "$BASE/bookmarks"
```

Expected: **200** and a JSON array.

### Get one bookmark — `GET /bookmarks/{id}`

```bash
curl -s "$BASE/bookmarks/1"
```

Expected: **200** if the id exists; **404** with `{"error":"bookmark not found"}` if not.

### Update a bookmark — `PUT /bookmarks/{id}`

Same body rules as create.

```bash
curl -s -X PUT "$BASE/bookmarks/1" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/doc","title":"Go Docs","description":"Documentation"}'
```

Expected: **200** with updated JSON; **404** if id missing.

### Delete a bookmark — `DELETE /bookmarks/{id}`

```bash
curl -s -o /dev/null -w "%{http_code}\n" -X DELETE "$BASE/bookmarks/1"
```

Expected: **204** (empty body), including on repeat delete (idempotent).

### Validation error example

```bash
curl -s -X POST "$BASE/bookmarks" \
  -H "Content-Type: application/json" \
  -d '{"url":"","title":"Missing URL"}'
```

Expected: **400** with `{"error":"url is required"}`.

## 3. End-to-end script (optional)

Replace `1` with an `id` from a create response if needed:

```bash
export BASE=http://localhost:8080

curl -s -X POST "$BASE/bookmarks" -H "Content-Type: application/json" \
  -d '{"url":"https://example.com","title":"Example","description":"demo"}'

curl -s "$BASE/bookmarks"
curl -s "$BASE/bookmarks/1"
curl -s -X PUT "$BASE/bookmarks/1" -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/updated","title":"Example updated","description":""}'
curl -s -o /dev/null -w "DELETE status: %{http_code}\n" -X DELETE "$BASE/bookmarks/1"
curl -s -o /dev/null -w "DELETE again: %{http_code}\n" -X DELETE "$BASE/bookmarks/1"
```

Expected: both deletes return **204**.

## 4. Collections API (`curl`)

Collections group bookmarks. A bookmark can belong to many collections; order is **per collection** (array order in list responses).

```bash
export BASE=http://localhost:8080
```

### Collection CRUD

```bash
# Create (201)
curl -s -X POST "$BASE/collections" -H "Content-Type: application/json" \
  -d '{"name":"Reading list"}'

# List (200)
curl -s "$BASE/collections"

# Get (200) / missing (404)
curl -s "$BASE/collections/1"
curl -s "$BASE/collections/9999"

# Update (200)
curl -s -X PUT "$BASE/collections/1" -H "Content-Type: application/json" \
  -d '{"name":"Reading list (2026)"}'

# Delete (204, idempotent — safe to repeat)
curl -s -o /dev/null -w "%{http_code}\n" -X DELETE "$BASE/collections/1"
curl -s -o /dev/null -w "%{http_code}\n" -X DELETE "$BASE/collections/1"
```

Duplicate collection **names** are allowed; identity is the numeric **`id`**.

### Add / remove bookmarks in a collection

Create bookmarks first (see §2), then:

```bash
# Add (201 first time, 200 if already in collection)
curl -s -o /dev/null -w "add: %{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/1"
curl -s -o /dev/null -w "re-add: %{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/1"

# Ordered list (200) — order is JSON array order, not a position field
curl -s "$BASE/collections/1/bookmarks"

# Remove (204, idempotent)
curl -s -o /dev/null -w "remove: %{http_code}\n" -X DELETE "$BASE/collections/1/bookmarks/1"
curl -s -o /dev/null -w "remove again: %{http_code}\n" -X DELETE "$BASE/collections/1/bookmarks/1"
```

Errors: missing collection → **404** `collection not found`; missing bookmark on add → **404** `bookmark not found`.

### Reorder within a collection

`position` is **0-based** (first item is `0`).

```bash
curl -s -X PUT "$BASE/collections/1/bookmarks/3/position" \
  -H "Content-Type: application/json" \
  -d '{"position":0}'

curl -s "$BASE/collections/1/bookmarks"
```

| Case | Status | Example message |
|------|--------|-----------------|
| Missing `position` in body | **400** | `position is required` |
| Index out of range | **400** | `position out of range` |
| Bookmark not in collection (empty collection counts) | **404** | `bookmark not in collection` |
| Unknown bookmark id | **404** | `bookmark not found` |
| Same position as now | **200** | no-op |

```bash
curl -s -X PUT "$BASE/collections/1/bookmarks/1/position" \
  -H "Content-Type: application/json" -d '{}'
curl -s -X PUT "$BASE/collections/1/bookmarks/1/position" \
  -H "Content-Type: application/json" -d '{"position":99}'
```

## 5. Public share links

Create a read-only link (no auth). Optional expiry:

```bash
# Share one bookmark (201) — copy token or public_path from response
curl -s -X POST "$BASE/bookmarks/1/share" -H "Content-Type: application/json" \
  -d '{"ttl_seconds":86400}'

# Share a collection list in user order
curl -s -X POST "$BASE/collections/1/share"

# View (200) — no credentials
curl -s "$BASE/public/<token>"

# Revoke (204, idempotent)
curl -s -o /dev/null -w "%{http_code}\n" -X DELETE "$BASE/public/<token>"
```

Expired or revoked links return **404** `{"error":"share not found"}`.

## Notes

- The SQLite file (`bookmarks.db` by default) is created in the current working directory when you run the server.
- `bookmarks.db` is listed in `.gitignore`; delete it to reset data locally.
