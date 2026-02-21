// Package bedrock implements AWS Signature Version 4 signing for HTTP requests.
// This avoids a dependency on the full AWS SDK.
package bedrock

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	awsDateTimeFormat = "20060102T150405Z"
	awsDateFormat     = "20060102"
)

// signRequest adds AWS Signature V4 headers to the request.
// The request body must already be set (as a bytes.Reader or similar seekable reader).
// bodyHash is the SHA-256 hex digest of the request body ("" means it will be computed from r.Body).
func signRequest(r *http.Request, region, service, accessKeyID, secretAccessKey, sessionToken string, now time.Time, bodyBytes []byte) {
	dateTime := now.UTC().Format(awsDateTimeFormat)
	date := now.UTC().Format(awsDateFormat)

	// Required headers for SigV4.
	r.Header.Set("X-Amz-Date", dateTime)
	if sessionToken != "" {
		r.Header.Set("X-Amz-Security-Token", sessionToken)
	}

	bodyHash := hashHex(bodyBytes)
	r.Header.Set("X-Amz-Content-Sha256", bodyHash)

	// Collect headers to sign (sorted alphabetically).
	headersToSign := []string{"content-type", "host", "x-amz-content-sha256", "x-amz-date"}
	if sessionToken != "" {
		headersToSign = append(headersToSign, "x-amz-security-token")
	}
	sort.Strings(headersToSign)

	canonicalHeaders := buildCanonicalHeaders(r, headersToSign)
	signedHeaders := strings.Join(headersToSign, ";")

	// Step 1: Canonical request.
	canonicalURI := r.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := r.URL.Query().Encode()

	canonicalRequest := strings.Join([]string{
		r.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")

	// Step 2: String to sign.
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", date, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		dateTime,
		credentialScope,
		hashHex([]byte(canonicalRequest)),
	}, "\n")

	// Step 3: Calculate signature.
	signingKey := buildSigningKey(secretAccessKey, date, region, service)
	signature := hmacHex(signingKey, stringToSign)

	// Step 4: Add Authorization header.
	r.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKeyID, credentialScope, signedHeaders, signature,
	))
}

func buildCanonicalHeaders(r *http.Request, headersToSign []string) string {
	var sb strings.Builder
	for _, h := range headersToSign {
		var val string
		switch h {
		case "host":
			val = r.Host
			if val == "" {
				val = r.URL.Host
			}
		default:
			val = strings.TrimSpace(r.Header.Get(http.CanonicalHeaderKey(h)))
		}
		sb.WriteString(h)
		sb.WriteByte(':')
		sb.WriteString(val)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func buildSigningKey(secretKey, date, region, service string) []byte {
	kDate := hmacBytes([]byte("AWS4"+secretKey), date)
	kRegion := hmacBytes(kDate, region)
	kService := hmacBytes(kRegion, service)
	kSigning := hmacBytes(kService, "aws4_request")
	return kSigning
}

func hmacBytes(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hmacHex(key []byte, data string) string {
	return hex.EncodeToString(hmacBytes(key, data))
}

func hashHex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
