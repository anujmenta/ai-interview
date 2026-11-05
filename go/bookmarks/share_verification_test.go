package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func shareTokenFromCreate(body string) string {
	var sl ShareLink
	if err := json.Unmarshal([]byte(body), &sl); err != nil {
		return ""
	}
	return sl.Token
}

func publicBookmarkIDs(body string) []int64 {
	var v PublicShareView
	json.Unmarshal([]byte(body), &v)
	return bookmarkIDsFromList(mustJSON(v.Bookmarks))
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestShareVerificationPlan maps SHARE_VERIFICATION_TEST_PLAN.md.
func TestShareVerificationPlan(t *testing.T) {
	t.Run("1_regression_smoke", func(t *testing.T) {
		_, mux := verificationMux(t)
		if code := vReq(mux, http.MethodGet, "/bookmarks", "").Code; code != http.StatusOK {
			t.Fatal(code)
		}
		vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"Before"}`)
		snap := vReq(mux, http.MethodGet, "/bookmarks/1", "").Body.String()
		if vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Code != http.StatusCreated {
			t.Fatal()
		}
		if vReq(mux, http.MethodGet, "/bookmarks/1", "").Body.String() != snap {
			t.Fatal("share create mutated bookmark")
		}
	})

	t.Run("2_bookmark_create", func(t *testing.T) {
		t.Run("2.1_empty_body", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			rec := vReq(mux, http.MethodPost, "/bookmarks/1/share", "")
			if rec.Code != http.StatusCreated {
				t.Fatal(rec.Body.String())
			}
			var sl ShareLink
			json.Unmarshal(rec.Body.Bytes(), &sl)
			if sl.Kind != "bookmark" || sl.BookmarkID == nil || *sl.BookmarkID != 1 {
				t.Fatal(sl)
			}
			if sl.RevokedAt != nil {
				t.Fatal("revoked_at should be absent")
			}
		})

		t.Run("2.2_public_read", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T","description":"d"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			priv := vReq(mux, http.MethodGet, "/bookmarks/1", "").Body.String()
			pub := vReq(mux, http.MethodGet, "/public/"+tok, "")
			if pub.Code != http.StatusOK {
				t.Fatal(pub.Code)
			}
			if !strings.Contains(pub.Body.String(), `"title":"T"`) {
				t.Fatal(pub.Body.String())
			}
			if !strings.Contains(priv, `"title":"T"`) {
				t.Fatal(priv)
			}
		})

		t.Run("2.3_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			for _, p := range []string{"/bookmarks/99999/share", "/bookmarks/abc/share"} {
				rec := vReq(mux, http.MethodPost, p, "")
				if rec.Code != http.StatusNotFound {
					t.Fatalf("%s: %d", p, rec.Code)
				}
			}
		})

		t.Run("2.4_multiple_shares", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			r1 := vReq(mux, http.MethodPost, "/bookmarks/1/share", "")
			r2 := vReq(mux, http.MethodPost, "/bookmarks/1/share", "")
			t1, t2 := shareTokenFromCreate(r1.Body.String()), shareTokenFromCreate(r2.Body.String())
			if t1 == "" || t2 == "" || t1 == t2 {
				t.Fatal("expected distinct tokens")
			}
			if vReq(mux, http.MethodGet, "/public/"+t1, "").Code != http.StatusOK {
				t.Fatal()
			}
			if vReq(mux, http.MethodGet, "/public/"+t2, "").Code != http.StatusOK {
				t.Fatal()
			}
		})

		t.Run("2.5_empty_object", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			if vReq(mux, http.MethodPost, "/bookmarks/1/share", `{}`).Code != http.StatusCreated {
				t.Fatal()
			}
		})

		t.Run("2.6_invalid_json", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			for _, body := range []string{`[]`, `not-json`} {
				rec := vReq(mux, http.MethodPost, "/bookmarks/1/share", body)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("body %q: %d", body, rec.Code)
				}
			}
		})
	})

	t.Run("3_collection_create", func(t *testing.T) {
		t.Run("3.1_empty_body", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			rec := vReq(mux, http.MethodPost, "/collections/1/share", "")
			if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"kind":"collection"`) {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("3.2_order_matches", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"a","title":"A"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"b","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/2", "")
			priv := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			pub := publicBookmarkIDs(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String())
			if len(priv) != 2 || strings.Join(int64s(priv), ",") != strings.Join(int64s(pub), ",") {
				t.Fatalf("priv %v pub %v", priv, pub)
			}
		})

		t.Run("3.3_empty_collection", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			rec := vReq(mux, http.MethodGet, "/public/"+tok, "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bookmarks":[]`) {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("3.4_not_found", func(t *testing.T) {
			_, mux := verificationMux(t)
			if vReq(mux, http.MethodPost, "/collections/99999/share", "").Code != http.StatusNotFound {
				t.Fatal()
			}
			if vReq(mux, http.MethodPost, "/collections/abc/share", "").Code != http.StatusNotFound {
				t.Fatal()
			}
		})

		t.Run("3.5_multiple_shares", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			t1 := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			t2 := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			if t1 == t2 {
				t.Fatal()
			}
		})
	})

	t.Run("4_expiry", func(t *testing.T) {
		t.Run("4.2_past_at_create", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", `{"expires_at":"`+past+`"}`).Body.String())
			if vReq(mux, http.MethodGet, "/public/"+tok, "").Code != http.StatusNotFound {
				t.Fatal()
			}
		})

		t.Run("4.4_boundary", func(t *testing.T) {
			s := openTestDB(t)
			b, _ := s.CreateBookmark("u", "T", "")
			exp := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
			sl, _ := s.CreateBookmarkShare(b.ID, &exp)
			if _, err := s.ResolvePublicShare(sl.Token, exp); err != ErrShareNotFound {
				t.Fatalf("at expiry: %v", err)
			}
			if _, err := s.ResolvePublicShare(sl.Token, exp.Add(-time.Second)); err != nil {
				t.Fatalf("before expiry: %v", err)
			}
		})

		t.Run("4.5_invalid_expires_at", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			for _, body := range []string{`{"expires_at":"tomorrow"}`, `{"expires_at":""}`} {
				rec := vReq(mux, http.MethodPost, "/bookmarks/1/share", body)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("%s: %d", body, rec.Code)
				}
			}
		})

		t.Run("4.6_ttl_in_response", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			rec := vReq(mux, http.MethodPost, "/bookmarks/1/share", `{"ttl_seconds":3600}`)
			var sl ShareLink
			json.Unmarshal(rec.Body.Bytes(), &sl)
			if sl.ExpiresAt == nil || *sl.ExpiresAt == "" {
				t.Fatal("expected expires_at from ttl")
			}
		})

		t.Run("4.7_ttl_validation", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			for _, body := range []string{`{"ttl_seconds":0}`, `{"ttl_seconds":-1}`} {
				if vReq(mux, http.MethodPost, "/bookmarks/1/share", body).Code != http.StatusBadRequest {
					t.Fatal(body)
				}
			}
		})

		t.Run("4.8_ttl_wins", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			before := time.Now().UTC()
			rec := vReq(mux, http.MethodPost, "/bookmarks/1/share",
				`{"ttl_seconds":30,"expires_at":"2099-01-01T00:00:00Z"}`)
			var sl ShareLink
			json.Unmarshal(rec.Body.Bytes(), &sl)
			exp, err := time.Parse(time.RFC3339, *sl.ExpiresAt)
			if err != nil {
				t.Fatal(err)
			}
			if exp.After(before.Add(2 * time.Minute)) {
				t.Fatalf("ttl should win over far future expires_at, got %v", exp)
			}
		})

		t.Run("4.9_timezone", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			rec := vReq(mux, http.MethodPost, "/bookmarks/1/share", `{"expires_at":"2030-01-01T00:00:00+05:30"}`)
			if rec.Code != http.StatusCreated {
				t.Fatal(rec.Code)
			}
			tok := shareTokenFromCreate(rec.Body.String())
			if vReq(mux, http.MethodGet, "/public/"+tok, "").Code != http.StatusOK {
				t.Fatal()
			}
		})
	})

	t.Run("5_revoke", func(t *testing.T) {
		t.Run("5.3_unknown_token", func(t *testing.T) {
			_, mux := verificationMux(t)
			if vReq(mux, http.MethodDelete, "/public/not-a-real-token-value", "").Code != http.StatusNoContent {
				t.Fatal()
			}
		})

		t.Run("5.4_resources_unchanged", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/public/"+tok, "")
			if vReq(mux, http.MethodGet, "/bookmarks/1", "").Code != http.StatusOK {
				t.Fatal()
			}
		})

		t.Run("5.5_one_of_two", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			t1 := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			t2 := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/public/"+t1, "")
			if vReq(mux, http.MethodGet, "/public/"+t1, "").Code != http.StatusNotFound {
				t.Fatal()
			}
			if vReq(mux, http.MethodGet, "/public/"+t2, "").Code != http.StatusOK {
				t.Fatal()
			}
		})
	})

	t.Run("6_public_access", func(t *testing.T) {
		_, mux := verificationMux(t)
		vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u1","title":"T1"}`)
		vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u2","title":"T2"}`)
		tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())

		t.Run("6.2_read_only", func(t *testing.T) {
			for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
				rec := vReq(mux, m, "/public/"+tok, `{}`)
				if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
					t.Fatalf("%s: %d", m, rec.Code)
				}
			}
		})

		t.Run("6.3_unknown", func(t *testing.T) {
			rec := vReq(mux, http.MethodGet, "/public/does-not-exist", "")
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "share not found") {
				t.Fatal(rec.Body.String())
			}
		})

		t.Run("6.6_scope_bookmark", func(t *testing.T) {
			rec := vReq(mux, http.MethodGet, "/public/"+tok, "")
			if strings.Contains(rec.Body.String(), `"T2"`) {
				t.Fatal("leaked other bookmark")
			}
		})

		t.Run("6.7_scope_collection", func(t *testing.T) {
			vReq(mux, http.MethodPost, "/collections", `{"name":"C1"}`)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C2"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodPut, "/collections/2/bookmarks/2", "")
			t1 := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			body := vReq(mux, http.MethodGet, "/public/"+t1, "").Body.String()
			if strings.Contains(body, `"C2"`) || strings.Contains(body, `"T2"`) {
				t.Fatal(body)
			}
		})
	})

	t.Run("7_live_data", func(t *testing.T) {
		t.Run("7.1_bookmark_update", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"Old"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			vReq(mux, http.MethodPut, "/bookmarks/1", `{"url":"u","title":"New"}`)
			if !strings.Contains(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String(), `"title":"New"`) {
				t.Fatal()
			}
		})

		t.Run("7.2_add_member", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"a","title":"A"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"b","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/2", "")
			ids := publicBookmarkIDs(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String())
			if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
				t.Fatalf("got %v", ids)
			}
		})

		t.Run("7.3_remove_member", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"a","title":"A"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"b","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/2", "")
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/collections/1/bookmarks/2", "")
			ids := publicBookmarkIDs(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String())
			if len(ids) != 1 || ids[0] != 1 {
				t.Fatalf("got %v", ids)
			}
		})

		t.Run("7.4_reorder", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			for i := 1; i <= 3; i++ {
				vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
				vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(int64(i)), "")
			}
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/3/position", `{"position":0}`)
			priv := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
			pub := publicBookmarkIDs(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String())
			if strings.Join(int64s(priv), ",") != strings.Join(int64s(pub), ",") {
				t.Fatalf("priv %v pub %v", priv, pub)
			}
		})

		t.Run("7.5_collection_rename", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"Old"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			vReq(mux, http.MethodPut, "/collections/1", `{"name":"New"}`)
			if !strings.Contains(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String(), `"name":"New"`) {
				t.Fatal()
			}
		})
	})

	t.Run("8_cascades", func(t *testing.T) {
		t.Run("8.1_delete_bookmark", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/bookmarks/1", "")
			if vReq(mux, http.MethodGet, "/public/"+tok, "").Code != http.StatusNotFound {
				t.Fatal()
			}
		})

		t.Run("8.2_delete_one_bookmark", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u1","title":"T1"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u2","title":"T2"}`)
			t1 := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			t2 := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/2/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/bookmarks/1", "")
			if vReq(mux, http.MethodGet, "/public/"+t1, "").Code != http.StatusNotFound {
				t.Fatal()
			}
			if vReq(mux, http.MethodGet, "/public/"+t2, "").Code != http.StatusOK {
				t.Fatal()
			}
		})

		t.Run("8.3_delete_collection", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/collections/1", "")
			if vReq(mux, http.MethodGet, "/public/"+tok, "").Code != http.StatusNotFound {
				t.Fatal()
			}
		})

		t.Run("8.5_member_deleted_globally", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"a","title":"A"}`)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"b","title":"B"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/2", "")
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/bookmarks/2", "")
			ids := publicBookmarkIDs(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String())
			if len(ids) != 1 || ids[0] != 1 {
				t.Fatalf("got %v", ids)
			}
		})
	})

	t.Run("9_gap_order", func(t *testing.T) {
		_, mux := verificationMux(t)
		vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
		for i := 1; i <= 3; i++ {
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			vReq(mux, http.MethodPut, "/collections/1/bookmarks/"+itoa(int64(i)), "")
		}
		vReq(mux, http.MethodDelete, "/bookmarks/2", "")
		tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/collections/1/share", "").Body.String())
		priv := bookmarkIDsFromList(vBody(vReq(mux, http.MethodGet, "/collections/1/bookmarks", "")))
		pub := publicBookmarkIDs(vReq(mux, http.MethodGet, "/public/"+tok, "").Body.String())
		if strings.Join(int64s(priv), ",") != strings.Join(int64s(pub), ",") {
			t.Fatalf("priv %v pub %v", priv, pub)
		}
	})

	t.Run("10_hardening", func(t *testing.T) {
		t.Run("10.1_unique_tokens", func(t *testing.T) {
			s := openTestDB(t)
			b, _ := s.CreateBookmark("u", "T", "")
			seen := map[string]bool{}
			for i := 0; i < 20; i++ {
				sl, err := s.CreateBookmarkShare(b.ID, nil)
				if err != nil {
					t.Fatal(err)
				}
				if seen[sl.Token] {
					t.Fatal("duplicate token")
				}
				seen[sl.Token] = true
			}
		})

		t.Run("10.3_revoke_without_get", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			tok := shareTokenFromCreate(vReq(mux, http.MethodPost, "/bookmarks/1/share", "").Body.String())
			vReq(mux, http.MethodDelete, "/public/"+tok, "")
			if vReq(mux, http.MethodGet, "/public/"+tok, "").Code != http.StatusNotFound {
				t.Fatal()
			}
		})

		t.Run("10.4_wrong_method_create", func(t *testing.T) {
			_, mux := verificationMux(t)
			vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
			rec := vReq(mux, http.MethodGet, "/bookmarks/1/share", "")
			if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
				t.Fatalf("got %d", rec.Code)
			}
		})
	})

	t.Run("11_store", func(t *testing.T) {
		s := openTestDB(t)
		b, _ := s.CreateBookmark("u", "T", "")
		sl, _ := s.CreateBookmarkShare(b.ID, nil)
		_ = s.RevokeShare(sl.Token)
		if _, err := s.ResolvePublicShare(sl.Token, time.Now().UTC()); err != ErrShareNotFound {
			t.Fatal(err)
		}
		if _, err := s.ResolvePublicShare("missing", time.Now().UTC()); err != ErrShareNotFound {
			t.Fatal(err)
		}
	})
}

func int64s(ids []int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = itoa(id)
	}
	return out
}
