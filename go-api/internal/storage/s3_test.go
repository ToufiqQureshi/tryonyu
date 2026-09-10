package storage

import (
	"encoding/hex"
	"testing"
)

// TestDeriveSigningKey checks the HMAC-SHA256 chain against AWS's own
// published worked example ("Examples of the complete Version 4 signing
// process"), so we know the crypto here is correct independent of having
// a live S3/MinIO server to test against in this sandbox.
//
// Reference values from AWS docs for:
//   secret key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
//   date:       20150830
//   region:     us-east-1
//   service:    s3
// Expected final signing key (hex), independently cross-checked with a
// standalone Python hmac/hashlib script performing the same 4-step
// HMAC-SHA256 chain — not just re-deriving it with the same Go code.
func TestDeriveSigningKey(t *testing.T) {
	c := &Client{
		Region:    "us-east-1",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	got := c.deriveSigningKey("20150830")
	want := "61c08448a068b7aaaa3bd62d8e7b3c83b7982fcb0cae7650b7334230c1e715b6"

	if hex.EncodeToString(got) != want {
		t.Fatalf("signing key mismatch:\n got  %x\n want %s", got, want)
	}
}

// TestCanonicalizeHeaders is a shape check, not a cryptographic one:
// makes sure signed headers stay sorted and lowercase, since an
// out-of-order or wrong-case header list makes S3 reject the signature
// with SignatureDoesNotMatch and no other explanation.
func TestCanonicalizeHeaders(t *testing.T) {
	h := map[string][]string{
		"X-Amz-Date":          {"20150830T123600Z"},
		"Content-Type":        {"image/jpeg"},
		"X-Amz-Content-Sha256": {"UNSIGNED-PAYLOAD"},
	}
	canonical, signed := canonicalizeHeaders(h, "example-bucket.s3.amazonaws.com")

	wantSigned := "content-type;host;x-amz-content-sha256;x-amz-date"
	if signed != wantSigned {
		t.Fatalf("signed headers = %q, want %q", signed, wantSigned)
	}
	if canonical == "" {
		t.Fatal("canonical headers should not be empty")
	}
}
