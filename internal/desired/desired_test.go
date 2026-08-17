package desired

import "testing"

const (
	testSHA    = "7bfe1179656431d785142ffb1afba3a6001a30f1"
	testDigest = "sha256:9f2c4b1d8e5a3c7f0b6d2e9a4c8f1b5d3e7a9c2f6b0d4e8a1c5f9b3d7e2a6c40"
)

func TestParseDeployV1JSON(t *testing.T) {
	raw := `{
  "apiVersion":"deploy/v1",
  "kind":"Release",
  "metadata":{"service":"app","environment":"prod"},
  "spec":{"image":"ghcr.io/acme/app","digest":"` + testDigest + `","rollout":{"strategy":"recreate"},"migration":{"required":false}},
  "provenance":{"git_sha":"` + testSHA + `"}
}`
	r, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if r.Spec.Digest != testDigest || r.Provenance.GitSHA != testSHA {
		t.Fatalf("unexpected release: %+v", r)
	}
	if r.ImageReference() != "ghcr.io/acme/app@"+testDigest {
		t.Fatalf("unexpected image ref %q", r.ImageReference())
	}
}

func TestRejectLegacySHAPointer(t *testing.T) {
	if _, err := Parse([]byte(testSHA)); err == nil {
		t.Fatal("expected legacy SHA pointer to be rejected")
	}
}

func TestRejectShortDigest(t *testing.T) {
	raw := `{"apiVersion":"deploy/v1","kind":"Release","metadata":{"service":"app","environment":"prod"},"spec":{"image":"ghcr.io/acme/app","digest":"sha256:abcd","rollout":{"strategy":"recreate"},"migration":{"required":false}},"provenance":{"git_sha":"` + testSHA + `"}}`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected short digest to be rejected")
	}
}

func TestRejectUnsupportedRollout(t *testing.T) {
	raw := `{"apiVersion":"deploy/v1","kind":"Release","metadata":{"service":"app","environment":"prod"},"spec":{"image":"ghcr.io/acme/app","digest":"` + testDigest + `","rollout":{"strategy":"blue-green"},"migration":{"required":false}},"provenance":{"git_sha":"` + testSHA + `"}}`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected blue-green to be rejected by v0.2 T1")
	}
}
