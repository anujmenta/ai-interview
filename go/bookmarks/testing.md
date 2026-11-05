# Manual integration testing (curl)

Run from **`go/bookmarks`** with the API listening on port **8080**.

## Prerequisites

```bash
cd go/bookmarks
rm -f bookmarks-test.db   # optional: clean slate
export DB_PATH=bookmarks-test.db
export PORT=8080                 # use 8081 if 8080 is in use
go run .                    # separate terminal
export BASE=http://localhost:8080   # match PORT
```

Automated unit tests (not curl):

```bash
go test ./... -race -count=1
```

---

## 1. Setup bookmarks (Plan §4.3)

| # | Description | Expected |
|---|-------------|----------|
| 1.1 | Create bookmark A | **201** |
| 1.2 | Create bookmark B | **201** |
| 1.3 | Create bookmark C | **201** |

```bash
curl -s -w "\nHTTP:%{http_code}\n" -X POST "$BASE/bookmarks" -H "Content-Type: application/json" \
  -d '{"url":"https://a.example","title":"A","description":""}'
curl -s -w "\nHTTP:%{http_code}\n" -X POST "$BASE/bookmarks" -H "Content-Type: application/json" \
  -d '{"url":"https://b.example","title":"B","description":""}'
curl -s -w "\nHTTP:%{http_code}\n" -X POST "$BASE/bookmarks" -H "Content-Type: application/json" \
  -d '{"url":"https://c.example","title":"C","description":""}'
```

Assumes ids **1**, **2**, **3** on a fresh database.

---

## 2. Collection CRUD (Plan §4.3)

| # | Description | Expected |
|---|-------------|----------|
| 2.1 | Create collection | **201** |
| 2.2 | List collections | **200** |
| 2.3 | Get collection 1 | **200** |
| 2.4 | Update collection 1 | **200** |

```bash
curl -s -w "\nHTTP:%{http_code}\n" -X POST "$BASE/collections" -H "Content-Type: application/json" \
  -d '{"name":"Reading list"}'
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/collections"
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/collections/1"
curl -s -w "\nHTTP:%{http_code}\n" -X PUT "$BASE/collections/1" -H "Content-Type: application/json" \
  -d '{"name":"Reading list (2026)"}'
```

---

## 3. Membership — add / remove / list (Plan §4.3)

| # | Description | Expected |
|---|-------------|----------|
| 3.1 | Add bookmark 1 to collection | **201** |
| 3.2 | Add bookmark 2 | **201** |
| 3.3 | Add bookmark 3 | **201** |
| 3.4 | Re-add bookmark 1 (idempotent) | **200** |
| 3.5 | List bookmarks in order | **200**, order A,B,C |
| 3.6 | Remove bookmark 2 | **204** |
| 3.7 | Remove bookmark 2 again | **204** |
| 3.8 | List after remove | **200**, A,C |

```bash
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/1"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/2"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/3"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/1"
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/collections/1/bookmarks"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X DELETE "$BASE/collections/1/bookmarks/2"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X DELETE "$BASE/collections/1/bookmarks/2"
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/collections/1/bookmarks"
```

---

## 4. Move / reorder (Plan §4.3)

Re-add bookmark 2 before move sequence so collection is 1,2,3.

| # | Description | Expected |
|---|-------------|----------|
| 4.0 | Re-add bookmark 2 | **201** or **200** |
| 4.1 | Move bookmark 3 to position 0 | **200** |
| 4.2 | List after move | **200**, C,A,B |
| 4.3 | Move noop (same position) | **200** |
| 4.4 | Bad position 99 | **400** |

```bash
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/2"
curl -s -w "\nHTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/3/position" \
  -H "Content-Type: application/json" -d '{"position":0}'
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/collections/1/bookmarks"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/3/position" \
  -H "Content-Type: application/json" -d '{"position":0}'
curl -s -w "\nHTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/3/position" \
  -H "Content-Type: application/json" -d '{"position":99}'
```

---

## 5. Validation & 404 samples (Plan §4.3)

| # | Description | Expected |
|---|-------------|----------|
| 5.1 | Empty collection name | **400** |
| 5.2 | Get collection 9999 | **404** |
| 5.3 | Add missing bookmark 9999 | **404** |
| 5.4 | Move with empty body | **400** |

```bash
curl -s -w "\nHTTP:%{http_code}\n" -X POST "$BASE/collections" -H "Content-Type: application/json" -d '{"name":""}'
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/collections/9999"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/9999"
curl -s -w "\nHTTP:%{http_code}\n" -X PUT "$BASE/collections/1/bookmarks/1/position" \
  -H "Content-Type: application/json" -d '{}'
```

---

## 6. Bookmark API regression (Plan §4.2 / setup)

| # | Description | Expected |
|---|-------------|----------|
| 6.1 | List all bookmarks | **200** |
| 6.2 | Get bookmark 1 | **200** |
| 6.3 | Delete bookmark (idempotent) | **204** |
| 6.4 | Delete bookmark again | **204** |
| 6.5 | Delete collection (idempotent) | **204** ×2 |

```bash
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/bookmarks"
curl -s -w "\nHTTP:%{http_code}\n" "$BASE/bookmarks/1"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X DELETE "$BASE/bookmarks/1"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X DELETE "$BASE/bookmarks/1"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X DELETE "$BASE/collections/1"
curl -s -o /dev/null -w "HTTP:%{http_code}\n" -X DELETE "$BASE/collections/1"
```

---

## Results

Latest full run: **`result.md`** (28/28 curls passed; `go test ./... -race` passed).
