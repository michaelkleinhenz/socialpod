package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestClassifyUploads(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)
	old := now.Add(-72 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	candidates := []uploadCandidate{
		{Filename: "in-use.jpg", Size: 10, CreatedAt: old, InDatabase: true, OnDisk: true},
		{Filename: "orphan-old.jpg", Size: 20, CreatedAt: old, InDatabase: true, OnDisk: true},
		{Filename: "orphan-recent.jpg", Size: 40, CreatedAt: recent, InDatabase: true, OnDisk: true},
		{Filename: "record-only.jpg", Size: 80, CreatedAt: old, InDatabase: true},
		{Filename: "file-only.jpg", Size: 160, CreatedAt: old, OnDisk: true},
		{Filename: "", Size: 320, CreatedAt: old, InDatabase: true},
	}
	referenced := map[string]bool{"in-use.jpg": true}

	orphans, totalFiles, totalBytes, skipped := classifyUploads(candidates, referenced, cutoff)

	if totalFiles != 5 {
		t.Errorf("totalFiles = %d, want 5 (the nameless record is not counted)", totalFiles)
	}
	if totalBytes != 310 {
		t.Errorf("totalBytes = %d, want 310", totalBytes)
	}
	if skipped != 1 {
		t.Errorf("skippedTooRecent = %d, want 1", skipped)
	}

	got := map[string]OrphanedUpload{}
	for _, o := range orphans {
		got[o.Filename] = o
	}
	for _, kept := range []string{"in-use.jpg", "orphan-recent.jpg"} {
		if _, swept := got[kept]; swept {
			t.Errorf("%s would be deleted but must be kept", kept)
		}
	}
	for _, want := range []string{"orphan-old.jpg", "record-only.jpg", "file-only.jpg"} {
		if _, swept := got[want]; !swept {
			t.Errorf("%s is orphaned and old enough, but was not swept", want)
		}
	}
	if o := got["file-only.jpg"]; o.InDatabase || !o.OnDisk {
		t.Errorf("file-only.jpg reported as onDisk=%v inDatabase=%v, want true/false", o.OnDisk, o.InDatabase)
	}
	if o := got["record-only.jpg"]; !o.InDatabase || o.OnDisk {
		t.Errorf("record-only.jpg reported as onDisk=%v inDatabase=%v, want false/true", o.OnDisk, o.InDatabase)
	}
}

// An empty reference set means "nothing is referenced", which is only correct
// when the reference scan actually succeeded — sweepOrphanedUploads aborts
// before this point when it did not.
func TestClassifyUploadsSweepsEverythingWhenNothingIsReferenced(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	candidates := []uploadCandidate{
		{Filename: "a.jpg", Size: 1, CreatedAt: old, InDatabase: true},
		{Filename: "b.jpg", Size: 2, CreatedAt: old, InDatabase: true},
	}

	orphans, _, _, _ := classifyUploads(candidates, map[string]bool{}, time.Now().Add(-24*time.Hour))
	if len(orphans) != 2 {
		t.Fatalf("got %d orphans, want 2", len(orphans))
	}
}

func TestClassifyUploadsOrdersNewestFirst(t *testing.T) {
	now := time.Now()
	candidates := []uploadCandidate{
		{Filename: "older.jpg", CreatedAt: now.Add(-96 * time.Hour), InDatabase: true},
		{Filename: "newer.jpg", CreatedAt: now.Add(-48 * time.Hour), InDatabase: true},
	}

	orphans, _, _, _ := classifyUploads(candidates, nil, now.Add(-24*time.Hour))
	if len(orphans) != 2 || orphans[0].Filename != "newer.jpg" {
		t.Fatalf("orphans not sorted newest first: %+v", orphans)
	}
}

// A clean store is the common case once the backlog has been swept, and the
// report still has to be readable: a nil sample marshals to JSON null and
// breaks a client that reads sample.length.
func TestSampleOrphansMarshalsAsEmptyArrayWhenNothingIsOrphaned(t *testing.T) {
	report := UploadSweepReport{DryRun: true, Sample: sampleOrphans(nil)}
	if report.Sample == nil {
		t.Fatal("sample is nil, want an empty slice")
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshalling the report: %v", err)
	}
	if !strings.Contains(string(encoded), `"sample":[]`) {
		t.Errorf("report JSON = %s, want an empty sample array", encoded)
	}
}

func TestSampleOrphansCapsTheSample(t *testing.T) {
	orphans := make([]OrphanedUpload, maxSweepSampleFiles+10)
	for i := range orphans {
		orphans[i].Filename = fmt.Sprintf("orphan-%d.jpg", i)
	}

	sample := sampleOrphans(orphans)
	if len(sample) != maxSweepSampleFiles {
		t.Fatalf("sample holds %d files, want %d", len(sample), maxSweepSampleFiles)
	}
	if sample[0].Filename != "orphan-0.jpg" {
		t.Errorf("sample starts at %s, want orphan-0.jpg", sample[0].Filename)
	}
}
