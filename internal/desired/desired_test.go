package desired

import "testing"

func TestParseFullSHA(t *testing.T) {
	sha := "7bfe1179656431d785142ffb1afba3a6001a30f1"
	r, err := Parse([]byte(sha))
	if err != nil {
		t.Fatal(err)
	}
	if r.GitSHA != sha || r.ImageTag != "sha-7bfe11796564" {
		t.Fatalf("unexpected release: %+v", r)
	}
}

func TestParseJSON(t *testing.T) {
	raw := `{"git_sha":"7bfe1179656431d785142ffb1afba3a6001a30f1","image_tag":"release-42"}`
	r, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if r.ImageTag != "release-42" {
		t.Fatalf("unexpected tag %q", r.ImageTag)
	}
}

func TestRejectShortSHA(t *testing.T) {
	if _, err := Parse([]byte("7bfe11796564")); err == nil {
		t.Fatal("expected short SHA to be rejected")
	}
}
