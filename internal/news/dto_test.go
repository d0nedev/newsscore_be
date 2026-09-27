package news

import (
	"testing"
	"time"
	"uuid"
)

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Persib Menang 2-1 atas Persija!": "persib-menang-2-1-atas-persija",
		"  --Ciro Alves: 'Kami siap'  ":   "ciro-alves-kami-siap",
		"Égalité à Surabaya":              "galit-surabaya",
		"日本語":                             "",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	v, err := WriteRequest{Title: " Persib Juara ", Category: "Resmi", Links: Links{TeamIDs: []string{uuid.New().String()}}}.validate()
	if err != nil || v.Slug != "persib-juara" || v.Title != "Persib Juara" || len(v.links) != 1 || !v.links[0].Team.Valid {
		t.Fatalf("valid request: %+v %v", v, err)
	}

	for name, req := range map[string]WriteRequest{
		"no title":    {Category: "Resmi"},
		"bad slug":    {Title: "x", Category: "Resmi", Slug: "Bad Slug"},
		"http image":  {Title: "x", Category: "Resmi", Image: "http://x/y.png"},
		"bad link id": {Title: "x", Category: "Resmi", Links: Links{MatchIDs: []string{"nope"}}},
	} {
		if _, err := req.validate(); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestRelativeLabel(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		30 * time.Second:    "baru saja",
		5 * time.Minute:     "5 menit lalu",
		2 * time.Hour:       "2 jam lalu",
		3 * 24 * time.Hour:  "3 hari lalu",
		10 * 24 * time.Hour: "17.09.2026",
	} {
		if got := relativeLabel(now.Add(-d), now); got != want {
			t.Errorf("%v ago = %q, want %q", d, got, want)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	c := cursor{PublishedAt: time.UnixMicro(1790489011123456), ID: uuid.New()}
	got, err := decodeCursor(c.encode())
	if err != nil || !got.PublishedAt.Equal(c.PublishedAt) || got.ID != c.ID {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if _, err := decodeCursor("!!"); err == nil {
		t.Error("garbage cursor accepted")
	}
}
