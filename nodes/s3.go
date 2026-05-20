package nodes

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// S3 performs operations against AWS S3 or any S3-compatible store.
// Actions: put, get, delete, list.
// Uses AWS Signature V4 — zero SDK dependency.
//
// Example:
//
//	nodes.NewS3("us-east-1", "ACCESS_KEY", "SECRET_KEY", "my-bucket")
type S3 struct {
	Region    string
	AccessKey string
	SecretKey string
	Bucket    string
	Endpoint  string // optional: for MinIO, R2, etc.
}

func NewS3(region, accessKey, secretKey, bucket string) *S3 {
	return &S3{Region: region, AccessKey: accessKey, SecretKey: secretKey, Bucket: bucket}
}

func (s *S3) WithEndpoint(endpoint string) *S3 {
	s.Endpoint = endpoint
	return s
}

func (s *S3) Name() string { return "s3" }

func (s *S3) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Reads, writes, deletes, and lists objects in AWS S3 or S3-compatible storage (MinIO, R2).",
		Params: map[string]any{
			"action":  map[string]any{"type": "string", "desc": "put | get | delete | list"},
			"key":     map[string]any{"type": "string", "desc": "Object key (path in bucket)."},
			"body":    map[string]any{"type": "string", "desc": "Content to upload (put)."},
			"bucket":  map[string]any{"type": "string", "desc": "Bucket name. Falls back to constructor."},
			"prefix":  map[string]any{"type": "string", "desc": "Key prefix filter (list)."},
		},
	}
}

func (s *S3) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	if action == "" {
		action = "get"
	}
	bucket := s.Bucket
	if v, ok := in["bucket"].(string); ok && v != "" {
		bucket = v
	}
	if bucket == "" {
		return nil, fmt.Errorf("s3: bucket is required")
	}

	key, _ := in["key"].(string)
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", bucket, s.Region)
	}

	switch action {
	case "put":
		body, _ := in["body"].(string)
		return s.request(ctx, "PUT", endpoint+"/"+key, []byte(body), bucket, key)
	case "get":
		if key == "" {
			return nil, fmt.Errorf("s3: key required for get")
		}
		return s.request(ctx, "GET", endpoint+"/"+key, nil, bucket, key)
	case "delete":
		if key == "" {
			return nil, fmt.Errorf("s3: key required for delete")
		}
		return s.request(ctx, "DELETE", endpoint+"/"+key, nil, bucket, key)
	case "list":
		prefix, _ := in["prefix"].(string)
		url := endpoint + "/?list-type=2"
		if prefix != "" {
			url += "&prefix=" + prefix
		}
		return s.request(ctx, "GET", url, nil, bucket, "")
	default:
		return nil, fmt.Errorf("s3: unknown action %q", action)
	}
}

func (s *S3) request(ctx context.Context, method, url string, body []byte, bucket, key string) (orchkit.Output, error) {
	now := time.Now().UTC()
	date := now.Format("20060102")
	datetime := now.Format("20060102T150405Z")

	var bodyReader io.Reader
	bodyHash := sha256hex([]byte{})
	if body != nil {
		bodyHash = sha256hex(body)
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	req.Header.Set("x-amz-date", datetime)
	req.Header.Set("x-amz-content-sha256", bodyHash)
	req.Header.Set("host", req.URL.Host)

	// Build canonical request.
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		req.URL.Host, bodyHash, datetime)

	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := req.URL.RawQuery

	canonical := strings.Join([]string{
		method, canonicalURI, canonicalQuery,
		canonicalHeaders, signedHeaders, bodyHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", date, s.Region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", datetime, credentialScope, sha256hex([]byte(canonical)),
	}, "\n")

	signingKey := s.signingKey(date)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		s.AccessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("authorization", auth)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("s3: error %d: %s", resp.StatusCode, respBody)
	}

	return orchkit.Output{
		"status": resp.StatusCode,
		"body":   string(respBody),
		"key":    key,
		"bucket": bucket,
	}, nil
}

func (s *S3) signingKey(date string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+s.SecretKey), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(s.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// silence unused import warning
var _ = sort.Strings
