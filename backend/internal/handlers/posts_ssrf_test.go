package handlers

import (
	"net"
	"net/url"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"2606:4700:4700::1111", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %s", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"ftp scheme rejected", "ftp://example.com/file.jpg", true},
		{"file scheme rejected", "file:///etc/passwd", true},
		{"empty host rejected", "http:///path", true},
		{"localhost rejected", "http://localhost/secret", true},
		{"127.0.0.1 rejected", "http://127.0.0.1/secret", true},
		{"10.x rejected", "http://10.0.0.1/internal", true},
		{"192.168.x rejected", "http://192.168.1.1/admin", true},
		{"169.254 rejected", "http://169.254.169.254/latest/meta-data/", true},
		{"valid https", "https://example.com/image.jpg", false},
		{"valid http", "http://example.com/image.jpg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("failed to parse URL %s: %v", tt.rawURL, err)
			}
			err = validateURL(u)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%s) error = %v, wantErr %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}
