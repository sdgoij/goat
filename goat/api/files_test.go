package api

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/mdlayher/goat/goat/common"
	"github.com/mdlayher/goat/goat/data"
)

// TestGetFilesJSON verifies that /api/files returns proper JSON output
func TestGetFilesJSON(t *testing.T) {
	log.Println("TestGetFilesJSON()")

	// Load config
	config := common.LoadConfig()
	common.Static.Config = config

	// Generate mock data.FileRecord
	file := data.FileRecord{
		InfoHash: "deadbeef",
		Verified: true,
	}

	// Save mock file
	if !file.Save() {
		t.Fatalf("Failed to save mock file")
	}

	// Load mock file to fetch ID
	file = file.Load(file.InfoHash, "info_hash")
	if file == (data.FileRecord{}) {
		t.Fatalf("Failed to load mock file")
	}

	// Request output JSON from API for this file
	var file2 data.FileRecord
	err := json.Unmarshal(getFilesJSON(file.ID), &file2)
	if err != nil {
		t.Fatalf("Failed to unmarshal result JSON for single file")
	}

	// Verify objects are the same
	if file.ID != file2.ID {
		t.Fatalf("ID, expected %d, got %d", file.ID, file2.ID)
	}

	// Request output JSON from API for all files
	var allFiles []data.FileRecord
	err = json.Unmarshal(getFilesJSON(-1), &allFiles)
	if err != nil {
		t.Fatalf("Failed to unmarshal result JSON for all files")
	}

	// Verify known file is in result set
	found := false
	for _, f := range allFiles {
		if f.ID == file.ID {
			found = true
		}
	}

	if !found {
		t.Fatalf("Expected file not found in all files result set")
	}

	// Delete mock file
	if !file.Delete() {
		t.Fatalf("Failed to delete mock file")
	}
}
