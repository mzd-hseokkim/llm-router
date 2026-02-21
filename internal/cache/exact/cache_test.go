package exact

import (
	"testing"
	"time"
)

func TestGzipRoundtrip(t *testing.T) {
	original := []byte(`{"response":{"id":"chatcmpl-123","choices":[{"message":{"content":"Hello, world!"}}]}}`)

	compressed, err := gzipCompress(original)
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) == 0 {
		t.Fatal("compressed should not be empty")
	}

	decompressed := maybeDecompress(compressed)
	if string(decompressed) != string(original) {
		t.Errorf("roundtrip mismatch: got %q", decompressed)
	}
}

func TestMaybeDecompress_NonGzip(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	out := maybeDecompress(data)
	if string(out) != string(data) {
		t.Errorf("non-gzip data should be returned as-is")
	}
}

func TestAge(t *testing.T) {
	entry := &Entry{CreatedAt: time.Now().Add(-5 * time.Second).Unix()}
	age := Age(entry)
	if age < 4 || age > 10 {
		t.Errorf("expected age ~5s, got %d", age)
	}
}
