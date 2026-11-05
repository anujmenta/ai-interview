package main

import (
	"testing"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	t.Setenv("DB_PATH", t.Name()+".db")
	// Use unique in-memory DB per test via file in temp - actually use :memory: by setting DB_PATH
	// openDB uses DB_PATH env - for :memory: we need to pass it
	t.Setenv("DB_PATH", "file:"+t.Name()+"?mode=memory&cache=shared")
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Store{db: db}
}

func TestCollectionCRUD(t *testing.T) {
	s := openTestDB(t)

	c, err := s.CreateCollection("Reading")
	if err != nil || c.Name != "Reading" || c.ID == 0 {
		t.Fatalf("create: %+v err=%v", c, err)
	}

	list, err := s.ListCollections()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v err=%v", list, err)
	}

	got, err := s.GetCollection(c.ID)
	if err != nil || got.Name != "Reading" {
		t.Fatalf("get: %+v err=%v", got, err)
	}

	updated, err := s.UpdateCollection(c.ID, "Reading 2")
	if err != nil || updated.Name != "Reading 2" {
		t.Fatalf("update: %+v err=%v", updated, err)
	}

	if _, err := s.GetCollection(999); err != nil {
		t.Fatal(err)
	}
	if c999, _ := s.GetCollection(999); c999 != nil {
		t.Fatal("expected nil collection")
	}

	ok, err := s.DeleteCollection(c.ID)
	if err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	if _, err := s.DeleteCollection(c.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.DeleteCollection(c.ID); ok {
		t.Fatal("expected false on second delete")
	}
}

func TestMembershipAndOrder(t *testing.T) {
	s := openTestDB(t)
	c, _ := s.CreateCollection("C1")
	b1, _ := s.CreateBookmark("https://a", "A", "")
	b2, _ := s.CreateBookmark("https://b", "B", "")
	b3, _ := s.CreateBookmark("https://c", "C", "")

	created, err := s.AddCollectionBookmark(c.ID, b1.ID)
	if err != nil || !created {
		t.Fatalf("add b1: created=%v err=%v", created, err)
	}
	created, err = s.AddCollectionBookmark(c.ID, b1.ID)
	if err != nil || created {
		t.Fatalf("re-add b1: created=%v err=%v", created, err)
	}
	if _, err := s.AddCollectionBookmark(c.ID, b2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCollectionBookmark(c.ID, b3.ID); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListCollectionBookmarks(c.ID)
	if err != nil || len(list) != 3 || list[0].ID != b1.ID || list[2].ID != b3.ID {
		t.Fatalf("list: %+v err=%v", list, err)
	}

	if err := s.RemoveCollectionBookmark(c.ID, b2.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListCollectionBookmarks(c.ID)
	if len(list) != 2 || list[0].ID != b1.ID || list[1].ID != b3.ID {
		t.Fatalf("after remove: %+v", list)
	}
	if err := s.RemoveCollectionBookmark(c.ID, b2.ID); err != nil {
		t.Fatal("idempotent remove")
	}

	c2, _ := s.CreateCollection("C2")
	if _, err := s.AddCollectionBookmark(c2.ID, b1.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAddErrors(t *testing.T) {
	s := openTestDB(t)
	c, _ := s.CreateCollection("C")
	b, _ := s.CreateBookmark("https://a", "A", "")

	if _, err := s.AddCollectionBookmark(999, b.ID); err != ErrCollectionNotFound {
		t.Fatalf("got %v", err)
	}
	if _, err := s.AddCollectionBookmark(c.ID, 999); err != ErrBookmarkNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestMoveBookmark(t *testing.T) {
	s := openTestDB(t)
	c, _ := s.CreateCollection("C")
	b1, _ := s.CreateBookmark("https://a", "A", "")
	b2, _ := s.CreateBookmark("https://b", "B", "")
	b3, _ := s.CreateBookmark("https://c", "C", "")
	for _, id := range []int64{b1.ID, b2.ID, b3.ID} {
		if _, err := s.AddCollectionBookmark(c.ID, id); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.MoveCollectionBookmark(c.ID, b3.ID, 0); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListCollectionBookmarks(c.ID)
	if list[0].ID != b3.ID || list[1].ID != b1.ID {
		t.Fatalf("order: %+v", list)
	}

	if _, err := s.MoveCollectionBookmark(c.ID, b1.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveCollectionBookmark(c.ID, 999, 0); err != ErrBookmarkNotFound {
		t.Fatalf("unknown bookmark: got %v", err)
	}
	bExtra, _ := s.CreateBookmark("https://x", "X", "")
	if _, err := s.MoveCollectionBookmark(c.ID, bExtra.ID, 0); err != ErrNotInCollection {
		t.Fatalf("not in collection: got %v", err)
	}
	if _, err := s.MoveCollectionBookmark(c.ID, b1.ID, 99); err != ErrPositionOutOfRange {
		t.Fatalf("bad position: got %v", err)
	}
}

func TestMoveEmptyCollection(t *testing.T) {
	s := openTestDB(t)
	c, _ := s.CreateCollection("C")
	b, _ := s.CreateBookmark("https://a", "A", "")
	if _, err := s.MoveCollectionBookmark(c.ID, b.ID, 0); err != ErrNotInCollection {
		t.Fatalf("empty collection move: got %v", err)
	}
}

func TestCascadeDelete(t *testing.T) {
	s := openTestDB(t)
	c, _ := s.CreateCollection("C")
	b, _ := s.CreateBookmark("https://a", "A", "")
	if _, err := s.AddCollectionBookmark(c.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteBookmark(b.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListCollectionBookmarks(c.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("expected empty list, got %+v err=%v", list, err)
	}
}

func TestAddAfterBookmarkCascadeDelete(t *testing.T) {
	s := openTestDB(t)
	c, _ := s.CreateCollection("C")
	b1, _ := s.CreateBookmark("https://a", "A", "")
	b2, _ := s.CreateBookmark("https://b", "B", "")
	b3, _ := s.CreateBookmark("https://c", "C", "")
	for _, id := range []int64{b1.ID, b2.ID, b3.ID} {
		if _, err := s.AddCollectionBookmark(c.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DeleteBookmark(b2.ID); err != nil {
		t.Fatal(err)
	}
	b4, _ := s.CreateBookmark("https://d", "D", "")
	if created, err := s.AddCollectionBookmark(c.ID, b4.ID); err != nil || !created {
		t.Fatalf("add after cascade: created=%v err=%v", created, err)
	}
	list, _ := s.ListCollectionBookmarks(c.ID)
	if len(list) != 3 || list[0].ID != b1.ID || list[1].ID != b3.ID || list[2].ID != b4.ID {
		t.Fatalf("got %+v", list)
	}
}

func TestConcurrentAdds(t *testing.T) {
	s := openTestDB(t)
	c, _ := s.CreateCollection("C")
	b1, _ := s.CreateBookmark("https://a", "A", "")
	b2, _ := s.CreateBookmark("https://b", "B", "")

	done := make(chan error, 2)
	go func() { _, err := s.AddCollectionBookmark(c.ID, b1.ID); done <- err }()
	go func() { _, err := s.AddCollectionBookmark(c.ID, b2.ID); done <- err }()
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.ListCollectionBookmarks(c.ID)
	if len(list) != 2 {
		t.Fatalf("expected 2 members, got %d", len(list))
	}
}
