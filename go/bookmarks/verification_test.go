package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func verificationMux(t *testing.T) (*Store, *http.ServeMux) {
	t.Helper()
	s := openTestDB(t)
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return s, mux
}

func vReq(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func vBody(rec *httptest.ResponseRecorder) string {
	return strings.TrimSpace(rec.Body.String())
}

func bookmarkIDsFromList(body string) []int64 {
	var list []Bookmark
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return nil
	}
	ids := make([]int64, len(list))
	for i, b := range list {
		ids[i] = b.ID
	}
	return ids
}

// TestVerificationPlan maps cases in VERIFICATION_TEST_PLAN.md.
func TestVerificationPlan(t *testing.T) {
	t.Run("1_regression_bookmarks", func(t *testing.T) {
		_, mux := verificationMux(t)

		t.Run("1.1_list_empty", func(t *testing.T) {
			rec := vReq(mux, http.MethodGet, "/bookmarks", "")
			if rec.Code != http.StatusOK || vBody(rec) != "[]" {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
		})

		t.Run("1.2_create_happy", func(t *testing.T) {
			rec := vReq(mux, http.MethodPost, "/bookmarks",
				`{"url":"https://example.com","title":"Example","description":"note"}`)
			if rec.Code != http.StatusCreated {
				t.Fatalf("got %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), `"url":"https://example.com"`) {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("1.3_validation", func(t *testing.T) {
			rec := vReq(mux, http.MethodPost, "/bookmarks", `{"url":"   ","title":"T"}`)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "url is required") {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
			rec = vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":""}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d", rec.Code)
			}
		})

		t.Run("1.4_invalid_json", func(t *testing.T) {
			rec := vReq(mux, http.MethodPost, "/bookmarks", `not-json`)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "body must be a JSON object") {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
			rec = vReq(mux, http.MethodPost, "/bookmarks", `[]`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d", rec.Code)
			}
		})

		t.Run("1.5_get_update_delete", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://x","title":"X","description":""}`)
			if rec.Code != http.StatusCreated {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodGet, "/bookmarks/1", "")
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodPut, "/bookmarks/1", `{"url":"https://y","title":"Y","description":""}`)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "https://y") {
				t.Fatal(rec.Body.String())
			}
			rec = vReq(mux, http.MethodDelete, "/bookmarks/1", "")
			if rec.Code != http.StatusNoContent {
				t.Fatalf("delete got %d", rec.Code)
			}
			rec = vReq(mux, http.MethodGet, "/bookmarks/1", "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("get after delete got %d", rec.Code)
			}
			rec = vReq(mux, http.MethodGet, "/bookmarks", "")
			if strings.Contains(rec.Body.String(), `"id":1`) {
				t.Fatal("deleted id still in list")
			}
		})

		t.Run("1.6_get_invalid_id", func(t *testing.T) {
			_, mux := verificationMux(t)
			for _, path := range []string{"/bookmarks/abc", "/bookmarks/99999"} {
				rec := vReq(mux, http.MethodGet, path, "")
				if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "bookmark not found") {
					t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
				}
			}
		})
	})

	t.Run("2_collection_crud", func(t *testing.T) {
		t.Run("2.1_create_happy", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodPost, "/collections", `{"name":"Reading list"}`)
			if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"name":"Reading list"`) {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})

		t.Run("2.2_name_required", func(t *testing.T) {
			_, mux := verificationMux(t)
			for _, body := range []string{`{}`, `{"name":""}`, `{"name":"   "}`} {
				rec := vReq(mux, http.MethodPost, "/collections", body)
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "name is required") {
					t.Fatalf("body %s: %d %s", body, rec.Code, rec.Body.String())
				}
			}
		})

		t.Run("2.3_invalid_json", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodPost, "/collections", `[]`)
			if rec.Code != http.StatusBadRequest {
				t.Fatal(rec.Code)
			}
		})

		t.Run("2.4_list_two", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"A"}`)
			vReq(mux, http.MethodPost, "/collections", `{"name":"B"}`)
			rec := vReq(mux, http.MethodGet, "/collections", "")
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code)
			}
			var cols []Collection
			json.Unmarshal(rec.Body.Bytes(), &cols)
			if len(cols) != 2 || cols[0].ID >= cols[1].ID {
				t.Fatalf("order: %+v", cols)
			}
		})

		t.Run("2.5_get_happy", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"X"}`)
			rec := vReq(mux, http.MethodGet, "/collections/1", "")
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code)
			}
		})

		t.Run("2.6_get_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			for _, path := range []string{"/collections/99999", "/collections/not-a-number"} {
				rec := vReq(mux, http.MethodGet, path, "")
				if rec.Code != http.StatusNotFound {
					t.Fatalf("%s: %d", path, rec.Code)
				}
			}
		})

		t.Run("2.7_update_happy", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"Old"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1", `{"name":"Renamed"}`)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Renamed") {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("2.8_update_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodPut, "/collections/99999", `{"name":"X"}`)
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("2.9_update_validation", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"Keep"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1", `{"name":"   "}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodGet, "/collections/1", "")
			if !strings.Contains(rec.Body.String(), "Keep") {
				t.Fatal("name changed on failed update")
			}
		})

		t.Run("2.10_delete_happy", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"D"}`)
			rec := vReq(mux, http.MethodDelete, "/collections/1", "")
			if rec.Code != http.StatusNoContent {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodGet, "/collections/1", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("2.11_delete_idempotent", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"D"}`)
			vReq(mux, http.MethodDelete, "/collections/1", "")
			rec := vReq(mux, http.MethodDelete, "/collections/1", "")
			if rec.Code != http.StatusNoContent {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodDelete, "/collections/99999", "")
			if rec.Code != http.StatusNoContent {
				t.Fatalf("non-existent numeric delete: got %d", rec.Code)
			}
		})

		t.Run("2.12_delete_invalid_path", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodDelete, "/collections/abc", "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("got %d", rec.Code)
			}
		})
	})

	t.Run("3_membership", func(t *testing.T) {
		setup := func(t *testing.T) (*http.ServeMux, int64, int64, int64, int64) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://a","title":"A"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://b","title":"B"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://c","title":"C"}`)
			return mux, 1, 1, 2, 3
		}

		t.Run("3.1_first_add", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://a","title":"A"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"title":"A"`) {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})

		t.Run("3.2_append_order", func(t *testing.T) {
			mux, c, b1, b2, b3 := setup(t)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/2", "")
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/3", "")
			rec := vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")
			ids := bookmarkIDsFromList(vBody(rec))
			if len(ids) != 3 || ids[0] != b1 || ids[1] != b2 || ids[2] != b3 {
				t.Fatalf("got %v want [%d %d %d]", ids, b1, b2, b3)
			}
			_ = c
		})

		t.Run("3.3_readd", func(t *testing.T) {
			mux, _, _, _, _ := setup(t)
			for i := int64(1); i <= 3; i++ {
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(i), "")
			}
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")
			if len(bookmarkIDsFromList(vBody(rec))) != 3 {
				t.Fatal("duplicate length")
			}
		})

		t.Run("3.4_collection_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			rec := vReq(mux, http.MethodPut, "/collections/99999/bookmarks/1", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("3.5_bookmark_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/99999", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("3.6_invalid_path_ids", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodPut, "/collections/abc/bookmarks/1", "")
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "collection not found") {
				t.Fatal(rec.Body.String())
			}
			rec = vReq(mux, http.MethodPut, "/collections/1/bookmarks/xyz", "")
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "bookmark not found") {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("3.7_two_collections", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C1"}`)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C2"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodPut, "/collections/2/bookmarks/1", "")
			r1 := vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")
			r2 := vReq(mux, http.MethodGet, "/collections/2/bookmarks", "")
			if len(bookmarkIDsFromList(vBody(r1))) != 1 || len(bookmarkIDsFromList(vBody(r2))) != 1 {
				t.Fatal("both should contain B")
			}
		})

		t.Run("3.8_remove_happy", func(t *testing.T) {
			mux, _, b1, _, b3 := setup(t)
			for i := int64(1); i <= 3; i++ {
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(i), "")
			}
			rec := vReq(mux, http.MethodDelete, "/collections/1/bookmarks/2", "")
			if rec.Code != http.StatusNoContent {
				t.Fatal(rec.Code)
			}
			ids := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			if len(ids) != 2 || ids[0] != b1 || ids[1] != b3 {
				t.Fatalf("got %v", ids)
			}
		})

		t.Run("3.9_reindex_after_remove_add", func(t *testing.T) {
			mux, _, b1, _, b3 := setup(t)
			for i := int64(1); i <= 3; i++ {
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(i), "")
			}
			vReq(mux, http.MethodDelete, "/collections/1/bookmarks/2", "")
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://d","title":"D"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/4", "")
			if rec.Code != http.StatusCreated {
				t.Fatalf("add B4: %d %s", rec.Code, rec.Body.String())
			}
			ids := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			if len(ids) != 3 || ids[0] != b1 || ids[1] != b3 || ids[2] != 4 {
				t.Fatalf("got %v want [%d %d 4]", ids, b1, b3)
			}
		})

		t.Run("3.10_remove_idempotent", func(t *testing.T) {
			mux, _, _, _, _ := setup(t)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodDelete, "/collections/1/bookmarks/1", "")
			rec := vReq(mux, http.MethodDelete, "/collections/1/bookmarks/1", "")
			if rec.Code != http.StatusNoContent {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodDelete, "/collections/1/bookmarks/99", "")
			if rec.Code != http.StatusNoContent {
				t.Fatal(rec.Code)
			}
		})

		t.Run("3.11_remove_collection_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodDelete, "/collections/99999/bookmarks/1", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("3.12_remove_invalid_bookmark_path", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			rec := vReq(mux, http.MethodDelete, "/collections/1/bookmarks/abc", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})
	})

	t.Run("4_list_in_collection", func(t *testing.T) {
		t.Run("4.1_empty", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			rec := vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")
			if rec.Code != http.StatusOK || vBody(rec) != "[]" {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})

		t.Run("4.3_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodGet, "/collections/99999/bookmarks", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("4.2_ordered_list", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			for i := 1; i <= 2; i++ {
				vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(int64(i)), "")
			}
			rec := vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")
			var list []Bookmark
			json.Unmarshal(rec.Body.Bytes(), &list)
			if len(list) != 2 || list[0].URL == "" || list[0].Title == "" {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("4.5_list_after_moves", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			for i, title := range []string{"A", "B", "C"} {
				vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://`+title+`","title":"`+title+`"}`)
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(int64(i+1)), "")
			}
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/3/position", `{"position":0}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":2}`)
			ids := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			if ids[0] != 3 || ids[2] != 1 {
				t.Fatalf("got %v", ids)
			}
		})

		t.Run("4.4_invalid_id", func(t *testing.T) {
			_, mux := verificationMux(t)
			rec := vReq(mux, http.MethodGet, "/collections/foo/bookmarks", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})
	})

	t.Run("5_move", func(t *testing.T) {
		setupABC := func(t *testing.T) *http.ServeMux {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			for i, title := range []string{"A", "B", "C"} {
				vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://`+title+`","title":"`+title+`"}`)
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(int64(i+1)), "")
			}
			return mux
		}

		t.Run("5.1_move_front", func(t *testing.T) {
			mux := setupABC(t)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/3/position", `{"position":0}`)
			ids := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			if len(ids) != 3 || ids[0] != 3 {
				t.Fatalf("got %v", ids)
			}
		})

		t.Run("5.2_move_back", func(t *testing.T) {
			mux := setupABC(t)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":2}`)
			ids := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			if ids[0] != 2 || ids[1] != 3 || ids[2] != 1 {
				t.Fatalf("got %v", ids)
			}
		})

		t.Run("5.3_move_down_one", func(t *testing.T) {
			mux := setupABC(t)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":1}`)
			ids := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			if ids[0] != 2 || ids[1] != 1 || ids[2] != 3 {
				t.Fatalf("got %v", ids)
			}
		})

		t.Run("5.4_noop", func(t *testing.T) {
			mux := setupABC(t)
			before := vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", ""))
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/2/position", `{"position":1}`)
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code)
			}
			if vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")) != before {
				t.Fatal("order changed on noop")
			}
		})

		t.Run("5.5_second_collection_unchanged", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C1"}`)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C2"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"a","title":"A"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"b","title":"B"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"x","title":"X"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/2", "")
			vReq(mux, http.MethodPut, "/collections/2/bookmarks/2", "")
			vReq(mux, http.MethodPut, "/collections/2/bookmarks/3", "")
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/2/position", `{"position":0}`)
			c2 := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/2/bookmarks", "")))
			if len(c2) != 2 || c2[0] != 2 || c2[1] != 3 {
				t.Fatalf("C2 changed: %v", c2)
			}
		})

		t.Run("5.6_position_high", func(t *testing.T) {
			mux := setupABC(t)
			before := vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", ""))
			for _, pos := range []string{`99`, `3`} {
				rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":`+pos+`}`)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("position %s: got %d", pos, rec.Code)
				}
			}
			if vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")) != before {
				t.Fatal("list changed")
			}
		})

		t.Run("5.7_position_negative", func(t *testing.T) {
			mux := setupABC(t)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":-1}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatal(rec.Code)
			}
		})

		t.Run("5.8_not_in_collection", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":0}`)
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "bookmark not in collection") {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})

		t.Run("5.9_bookmark_not_exist", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/99999/position", `{"position":0}`)
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("5.10_collection_not_exist", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			rec := vReq(mux, http.MethodPut, "/collections/99999/bookmarks/1/position", `{"position":0}`)
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})

		t.Run("5.11_empty_collection", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":0}`)
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "bookmark not in collection") {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})

		t.Run("5.12_single_member", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":0}`)
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code)
			}
		})

		t.Run("5.13_validation", func(t *testing.T) {
			mux := setupABC(t)
			for _, body := range []string{`{}`, `{"position":null}`, `not json`} {
				rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/1/position", body)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("body %q: %d", body, rec.Code)
				}
			}
		})

		t.Run("5.14_invalid_path", func(t *testing.T) {
			mux := setupABC(t)
			rec := vReq(mux, http.MethodPut, "/collections/x/bookmarks/1/position", `{"position":0}`)
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
			rec = vReq(mux, http.MethodPut, "/collections/1/bookmarks/y/position", `{"position":0}`)
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})
	})

	t.Run("6_cascades", func(t *testing.T) {
		t.Run("6.1_delete_bookmark_clears_membership", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodDelete, "/bookmarks/1", "")
			rec := vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")
			if vBody(rec) != "[]" {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("6.2_delete_middle_bookmark_then_add", func(t *testing.T) {
			s, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			for i := 1; i <= 3; i++ {
				vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			}
			for i := int64(1); i <= 3; i++ {
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(i), "")
			}
			vReq(mux, http.MethodDelete, "/bookmarks/2", "")
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u4","title":"B4"}`)
			rec := vReq(mux, http.MethodPut, "/collections/1/bookmarks/4", "")
			if rec.Code != http.StatusCreated {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			ids := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			if len(ids) != 3 || ids[0] != 1 || ids[1] != 3 || ids[2] != 4 {
				t.Fatalf("got %v", ids)
			}
			_ = s
		})

		t.Run("6.3_delete_collection_keeps_bookmarks", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodDelete, "/collections/1", "")
			rec := vReq(mux, http.MethodGet, "/bookmarks", "")
			if !strings.Contains(rec.Body.String(), `"id":1`) {
				t.Fatal("bookmark gone")
			}
		})

		t.Run("6.4_list_after_delete_collection", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodDelete, "/collections/1", "")
			rec := vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")
			if rec.Code != http.StatusNotFound {
				t.Fatal(rec.Code)
			}
		})
	})

	t.Run("7_concurrency", func(t *testing.T) {
		t.Run("7.1_parallel_add_different", func(t *testing.T) {
			s := openTestDB(t)
			c, _ := s.CreateCollection("C")
			b1, _ := s.CreateBookmark("https://a", "A", "")
			b2, _ := s.CreateBookmark("https://b", "B", "")
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); s.AddCollectionBookmark(c.ID, b1.ID) }()
			go func() { defer wg.Done(); s.AddCollectionBookmark(c.ID, b2.ID) }()
			wg.Wait()
			list, _ := s.ListCollectionBookmarks(c.ID)
			if len(list) != 2 {
				t.Fatalf("len %d", len(list))
			}
		})

		t.Run("7.2_parallel_add_same", func(t *testing.T) {
			s := openTestDB(t)
			c, _ := s.CreateCollection("C")
			b, _ := s.CreateBookmark("https://a", "A", "")
			var wg sync.WaitGroup
			var c1, c2 bool
			wg.Add(2)
			go func() { defer wg.Done(); x, _ := s.AddCollectionBookmark(c.ID, b.ID); if x { c1 = true } }()
			go func() { defer wg.Done(); x, _ := s.AddCollectionBookmark(c.ID, b.ID); if x { c2 = true } }()
			wg.Wait()
			list, _ := s.ListCollectionBookmarks(c.ID)
			if len(list) != 1 {
				t.Fatalf("want 1 row got %d", len(list))
			}
			if c1 && c2 {
				t.Fatal("both reported created")
			}
		})
	})

	t.Run("8_routing", func(t *testing.T) {
		_, mux := verificationMux(t)

		t.Run("8.1_wrong_method", func(t *testing.T) {
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			rec := vReq(mux, http.MethodPost, "/collections/1", `{"name":"X"}`)
			if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
				t.Fatalf("POST /collections/1: got %d want 405 or 404", rec.Code)
			}
			rec = vReq(mux, http.MethodGet, "/collections/1/bookmarks/1", "")
			if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
				t.Fatalf("GET membership: got %d", rec.Code)
			}
		})

		t.Run("8.2_routes_exist", func(t *testing.T) {
			paths := []struct {
				method string
				path   string
				body   string
			}{
				{http.MethodGet, "/collections", ""},
				{http.MethodPost, "/collections", `{"name":"R"}`},
				{http.MethodGet, "/collections/1", ""},
				{http.MethodPut, "/collections/1", `{"name":"R2"}`},
				{http.MethodGet, "/collections/1/bookmarks", ""},
				{http.MethodPut, "/collections/1/bookmarks/1", ""},
				{http.MethodPut, "/collections/1/bookmarks/1/position", `{"position":0}`},
			}
			for _, p := range paths {
				rec := vReq(mux, p.method, p.path, p.body)
				if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "404 page not found") {
					t.Fatalf("unwired route %s %s", p.method, p.path)
				}
			}
		})
	})
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
