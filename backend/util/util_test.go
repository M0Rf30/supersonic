package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	// Create source file with content
	srcPath := filepath.Join(tmpDir, "source.txt")
	testContent := []byte("Hello, World!\nThis is test content.")
	if err := os.WriteFile(srcPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test successful copy
	dstPath := filepath.Join(tmpDir, "destination.txt")
	if err := CopyFile(srcPath, dstPath); err != nil {
		t.Errorf("CopyFile failed: %v", err)
	}

	// Verify destination file exists and has correct content
	copiedContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Errorf("Failed to read destination file: %v", err)
	}
	if string(copiedContent) != string(testContent) {
		t.Errorf("Content mismatch: got %q, want %q", copiedContent, testContent)
	}

	// Test copying non-existent file
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.txt")
	if err := CopyFile(nonExistentPath, dstPath); err == nil {
		t.Error("Expected error when copying non-existent file, got nil")
	}

	// Test copying to invalid destination (directory that doesn't exist)
	invalidDst := filepath.Join(tmpDir, "nonexistent", "dest.txt")
	if err := CopyFile(srcPath, invalidDst); err == nil {
		t.Error("Expected error when copying to invalid destination, got nil")
	}
}

func TestGetLocalIP(t *testing.T) {
	ip, err := GetLocalIP()
	if err != nil {
		// Skip test if no network interface is available (e.g., in some CI environments)
		t.Skipf("Skipping GetLocalIP test: %v", err)
	}

	if ip == "" {
		t.Error("GetLocalIP returned empty string without error")
	}

	// Basic validation that it looks like an IPv4 address
	// Should have format like "192.168.1.1"
	if len(ip) < 7 || len(ip) > 15 {
		t.Errorf("GetLocalIP returned suspicious IP format: %q", ip)
	}

	// Check it contains at least 3 dots (basic IPv4 check)
	dotCount := 0
	for _, ch := range ip {
		if ch == '.' {
			dotCount++
		}
	}
	if dotCount != 3 {
		t.Errorf("GetLocalIP returned invalid IPv4 format (expected 3 dots): %q", ip)
	}

	// Should not be loopback
	if ip == "127.0.0.1" {
		t.Error("GetLocalIP returned loopback address, expected non-loopback")
	}
}
