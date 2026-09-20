package handlers

import (
	"encoding/base64"
	"testing"
)

func TestCleanBase64_Plain(t *testing.T) {
	orig := []byte("hello world")
	encoded := base64.StdEncoding.EncodeToString(orig)
	cleaned := cleanBase64(encoded)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Fatalf("mismatch: got %q", decoded)
	}
}

func TestCleanBase64_DataURIPrefix(t *testing.T) {
	orig := []byte("hello world")
	encoded := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(orig)
	cleaned := cleanBase64(encoded)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Fatalf("mismatch: got %q", decoded)
	}
}

func TestCleanBase64_DataURIPNG(t *testing.T) {
	orig := []byte("png data here")
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(orig)
	cleaned := cleanBase64(encoded)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Fatalf("mismatch: got %q", decoded)
	}
}

func TestCleanBase64_WithWhitespace(t *testing.T) {
	orig := []byte("hello world")
	encoded := base64.StdEncoding.EncodeToString(orig)
	// Insert newlines and spaces
	withWS := encoded[:4] + "\n" + encoded[4:8] + " " + encoded[8:] + "\r\n"
	cleaned := cleanBase64(withWS)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Fatalf("mismatch: got %q", decoded)
	}
}

func TestCleanBase64_URLSafe(t *testing.T) {
	orig := []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa}
	encoded := base64.URLEncoding.EncodeToString(orig)
	cleaned := cleanBase64(encoded)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Fatalf("mismatch: got %q want %q", decoded, orig)
	}
}

func TestCleanBase64_NoPadding(t *testing.T) {
	orig := []byte("test")
	encoded := base64.RawStdEncoding.EncodeToString(orig)
	cleaned := cleanBase64(encoded)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Fatalf("mismatch: got %q", decoded)
	}
}

func TestCleanBase64_DataURIPlusURLSafe(t *testing.T) {
	orig := []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa}
	encoded := "data:image/png;base64," + base64.RawURLEncoding.EncodeToString(orig)
	cleaned := cleanBase64(encoded)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Fatalf("mismatch: got %q want %q", decoded, orig)
	}
}
