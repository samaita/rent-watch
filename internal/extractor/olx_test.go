package extractor

import "testing"

func TestCanonicalizeURL(t *testing.T) {
	got := canonicalizeURL("https://www.olx.co.id/item/foo?tracking=1#top")
	want := "https://www.olx.co.id/item/foo"
	if got != want {
		t.Fatalf("canonicalizeURL() = %q, want %q", got, want)
	}
}

func TestParseOLXCard(t *testing.T) {
	card := rawCard{
		URL:   "https://www.olx.co.id/item/foo",
		Title: "Rumah Bagus",
		Text:  "Rp 850.000.000\n120 m2\n3 KT\n2 KM\nKelurahan Nogotirto",
	}
	got := parseOLXCard(card)
	if got.Price == nil || *got.Price != 850000000 {
		t.Fatalf("price = %v", got.Price)
	}
	if got.SizeM2 == nil || *got.SizeM2 != 120 {
		t.Fatalf("size = %v", got.SizeM2)
	}
	if got.Bedrooms == nil || *got.Bedrooms != 3 {
		t.Fatalf("bedrooms = %v", got.Bedrooms)
	}
	if got.Bathrooms == nil || *got.Bathrooms != 2 {
		t.Fatalf("bathrooms = %v", got.Bathrooms)
	}
	if got.LocationKelurahan != "Kelurahan Nogotirto" {
		t.Fatalf("location = %q", got.LocationKelurahan)
	}
}
