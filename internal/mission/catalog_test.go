package mission

import "testing"

func TestEmbeddedCatalog(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	items := catalog.All()
	if len(items) != 10 {
		t.Fatalf("len(All()) = %d, want 10", len(items))
	}
	for index, item := range items {
		if item.Number != index+1 {
			t.Errorf("mission at index %d has number %d", index, item.Number)
		}
	}

	byID, found := catalog.Find("linux-find-logs")
	if !found || byID.Number != 4 {
		t.Fatalf("Find(id) = %#v, %v", byID, found)
	}
	byNumber, found := catalog.Find("04")
	if !found || byNumber.ID != byID.ID {
		t.Fatalf("Find(04) = %#v, %v", byNumber, found)
	}
}

func TestNextReturnsFirstIncompleteMission(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Next(func(id string) bool {
		return id == "linux-orientation" || id == "linux-config-crawl"
	})
	if !found || item.Number != 3 {
		t.Fatalf("Next() = %#v, %v; want mission 3", item, found)
	}
}
