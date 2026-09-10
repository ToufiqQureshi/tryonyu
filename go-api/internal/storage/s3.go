// Package storage implements a minimal S3-compatible client using raw
// AWS Signature Version 4 signing — no AWS SDK, no minio-go, zero
// third-party dependencies. This matters for two real reasons, not just
// dependency-avoidance for its own sake:
//
//  1. It works unmodified against MinIO (local/dev), AWS S3, DigitalOcean
//     Spaces, Cloudflare R2, or any other S3-compatible provider — you
//     only ever change the Endpoint. No vendor lock to an SDK's client.
//  2. SigV4 is a stable, 10+ year old spec. This file will not break on
//     an upstream SDK major-version bump.
//
// If you'd rather use github.com/minio/minio-go or the AWS SDK, this
// file is a drop-in replaceable unit — swap it for that client behind
// the same PutObject/PresignGetURL method signatures and nothing else
// in the codebase needs to change.
package storage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	Endpoint  string // e.g. "http://localhost:9000" (MinIO) or "https://s3.ap-south-1.amazonaws.com"
	Region    string // e.g. "us-east-1" for MinIO default, "ap-south-1" for AWS Mumbai
	Bucket    string
	AccessKey string
	SecretKey string
}

func NewClient(endpoint, region, bucket, accessKey, secretKey string) *Client {
	return &Client{
		Endpoint:  strings.TrimRight(endpoint, "/"),
		Region:    region,
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
	}
}

const unsignedPayload = "UNSIGNED-PAYLOAD"

// PutObject uploads body to {bucket}/{key} using path-style addressing
// (required for MinIO; works fine against AWS S3 too). Used by
// UploadCustomerPhoto to store the one-time reference photo.
func (c *Client) PutObject(key string, body io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(io.LimitReader(body, size+1))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) != size && size > 0 {
		// size hint was wrong; proceed with actual length rather than fail
	}
	actualSize := int64(len(data))

	reqURL := fmt.Sprintf("%s/%s/%s", c.Endpoint, c.Bucket, url.PathEscape(key))
	req, err := http.NewRequest(http.MethodPut, reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = actualSize

	if err := c.signHeaderAuth(req, unsignedPayload); err != nil {
		return fmt.Errorf("sign request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3 put failed: %s: %s", resp.Status, string(respBody))
	}
	return nil
}

// PresignGetURL returns a time-limited GET URL for {bucket}/{key} — this
// is what the Go API hands back as result_image_url so the actual image
// bytes flow browser<->S3 directly, never through our API process.
func (c *Client) PresignGetURL(key string, expiry time.Duration) (string, error) {
	host := strings.TrimPrefix(strings.TrimPrefix(c.Endpoint, "https://"), "http://")
	scheme := "https"
	if strings.HasPrefix(c.Endpoint, "http://") {
		scheme = "http"
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, c.Region)
	credential := fmt.Sprintf("%s/%s", c.AccessKey, credentialScope)

	canonicalURI := "/" + c.Bucket + "/" + url.PathEscape(key)

	query := url.Values{}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", credential)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", strconv.Itoa(int(expiry.Seconds())))
	query.Set("X-Amz-SignedHeaders", "host")
	canonicalQuery := canonicalQueryString(query)

	canonicalHeaders := "host:" + host + "\n"
	signedHeaders := "host"

	canonicalRequest := strings.Join([]string{
		http.MethodGet,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		unsignedPayload,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hashHex(canonicalRequest),
	}, "\n")

	signingKey := c.deriveSigningKey(dateStamp)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	query.Set("X-Amz-Signature", signature)

	return fmt.Sprintf("%s://%s%s?%s", scheme, host, canonicalURI, query.Encode()), nil
}

// signHeaderAuth signs req in place with an Authorization header (used
// for PUT — the SDK-equivalent of "header-based SigV4 auth", as opposed
// to the query-string based signing PresignGetURL uses).
func (c *Client) signHeaderAuth(req *http.Request, payloadHash string) error {
	host := req.URL.Host
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, c.Region)

	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonicalHeaders, signedHeaders := canonicalizeHeaders(req.Header, host)

	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hashHex(canonicalRequest),
	}, "\n")

	signingKey := c.deriveSigningKey(dateStamp)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.AccessKey, credentialScope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authHeader)
	return nil
}

func (c *Client) deriveSigningKey(dateStamp string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+c.SecretKey), dateStamp)
	kRegion := hmacSHA256(kDate, c.Region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hashHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// canonicalizeHeaders builds the CanonicalHeaders + SignedHeaders strings
// per the SigV4 spec: lowercase names, sorted, trimmed values.
func canonicalizeHeaders(h http.Header, host string) (canonical, signed string) {
	names := []string{"host"}
	values := map[string]string{"host": host}

	for name := range h {
		lower := strings.ToLower(name)
		if lower == "host" || lower == "authorization" {
			continue
		}
		if strings.HasPrefix(lower, "x-amz-") || lower == "content-type" {
			names = append(names, lower)
			values[lower] = strings.TrimSpace(h.Get(name))
		}
	}
	sort.Strings(names)

	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		b.WriteString(":")
		b.WriteString(values[n])
		b.WriteString("\n")
	}
	return b.String(), strings.Join(names, ";")
}

func canonicalQueryString(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v.Get(k)))
	}
	return strings.Join(parts, "&")
}
