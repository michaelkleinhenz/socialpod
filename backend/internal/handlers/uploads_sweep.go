package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"socialmedia/internal/database"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// defaultSweepMinAgeHours keeps very recent uploads out of a sweep. A file is
// stored the moment it is imported (POST /upload, /upload-from-url, a capture),
// minutes before the post or draft that will reference it exists, so a sweep
// with no age guard would delete images out from under an editor that is still
// open. A day is comfortably longer than any editing session.
const defaultSweepMinAgeHours = 24

// maxSweepSampleFiles caps how many filenames a report lists. The counts and
// byte totals always cover everything; the sample is only there to eyeball
// what a sweep would touch.
const maxSweepSampleFiles = 50

// OrphanedUpload is one file no document references any more.
type OrphanedUpload struct {
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	// OnDisk and InDatabase say where the orphan still exists. An upload is
	// normally both; a file whose record was lost (or vice versa) shows up
	// with one of them false.
	OnDisk     bool `json:"onDisk"`
	InDatabase bool `json:"inDatabase"`
}

// UploadSweepReport is what a scan found and, for a sweep, what it deleted.
type UploadSweepReport struct {
	DryRun bool `json:"dryRun"`
	// MinAgeHours is the age guard the sweep ran with.
	MinAgeHours int `json:"minAgeHours"`
	// TotalFiles/TotalBytes cover every upload known, referenced or not.
	TotalFiles int   `json:"totalFiles"`
	TotalBytes int64 `json:"totalBytes"`
	// OrphanedFiles/OrphanedBytes cover the unreferenced ones old enough to
	// sweep. SkippedTooRecent counts unreferenced ones the age guard kept.
	OrphanedFiles    int   `json:"orphanedFiles"`
	OrphanedBytes    int64 `json:"orphanedBytes"`
	SkippedTooRecent int   `json:"skippedTooRecent"`
	// DeletedFiles/DeletedBytes are zero on a dry run.
	DeletedFiles int   `json:"deletedFiles"`
	DeletedBytes int64 `json:"deletedBytes"`
	// Sample lists up to maxSweepSampleFiles of the orphans, newest first.
	Sample []OrphanedUpload `json:"sample"`
	// Errors describes what could not be deleted. A sweep keeps going past a
	// failure, so this can be non-empty on an otherwise successful run.
	Errors []string `json:"errors,omitempty"`
}

// referencedUploadFilenames collects the filename of every upload any document
// still points at. It reads whole fields rather than testing candidates, so a
// sweep costs one query per referencing field however many uploads exist.
func referencedUploadFilenames(ctx context.Context, db *database.MongoDB) (map[string]bool, error) {
	referenced := map[string]bool{}
	for _, ref := range uploadReferences {
		values, err := ref.collection(db).Distinct(ctx, ref.field, bson.M{ref.field: bson.M{"$ne": nil}})
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", ref.field, err)
		}
		for _, v := range values {
			url, ok := v.(string)
			if !ok {
				continue
			}
			if name := uploadFilename(url); name != "" {
				referenced[name] = true
			}
		}
	}
	return referenced, nil
}

// uploadCandidate is one stored upload a sweep saw, from the uploads
// collection, the upload directory, or both.
type uploadCandidate struct {
	Filename   string
	Size       int64
	CreatedAt  time.Time
	InDatabase bool
	OnDisk     bool
}

// classifyUploads separates the uploads a sweep may delete from the ones it
// must keep: an upload goes only if nothing references it and it was stored
// before cutoff. It also returns the totals for the report.
func classifyUploads(candidates []uploadCandidate, referenced map[string]bool, cutoff time.Time) (orphans []OrphanedUpload, totalFiles int, totalBytes int64, skippedTooRecent int) {
	for _, c := range candidates {
		if c.Filename == "" {
			continue
		}
		totalFiles++
		totalBytes += c.Size

		if referenced[c.Filename] {
			continue
		}
		if c.CreatedAt.After(cutoff) {
			skippedTooRecent++
			continue
		}
		orphans = append(orphans, OrphanedUpload{
			Filename:   c.Filename,
			Size:       c.Size,
			CreatedAt:  c.CreatedAt,
			OnDisk:     c.OnDisk,
			InDatabase: c.InDatabase,
		})
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].CreatedAt.After(orphans[j].CreatedAt)
	})
	return orphans, totalFiles, totalBytes, skippedTooRecent
}

// collectUploadCandidates reads both halves of the upload store: the records
// in the uploads collection and the files in uploadDir. The two can drift
// apart — a file written before its record was inserted, a record whose file
// was lost — so an upload present in only one of them still counts, and one
// present in both counts once.
func collectUploadCandidates(ctx context.Context, db *database.MongoDB, uploadDir string) ([]uploadCandidate, []string, error) {
	var problems []string
	byName := map[string]*uploadCandidate{}

	// The projection keeps the stored image bytes out of the cursor: every
	// record carries its file's data, and a sweep reads every record.
	projection := options.Find().SetProjection(bson.M{"filename": 1, "size": 1, "createdAt": 1})
	cursor, err := db.Uploads().Find(ctx, bson.M{}, projection)
	if err != nil {
		return nil, problems, fmt.Errorf("listing uploads: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var rec struct {
			Filename  string    `bson:"filename"`
			Size      int64     `bson:"size"`
			CreatedAt time.Time `bson:"createdAt"`
		}
		if err := cursor.Decode(&rec); err != nil {
			problems = append(problems, "failed to read an upload record: "+err.Error())
			continue
		}
		if rec.Filename == "" {
			continue
		}
		byName[rec.Filename] = &uploadCandidate{
			Filename:   rec.Filename,
			Size:       rec.Size,
			CreatedAt:  rec.CreatedAt,
			InDatabase: true,
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, problems, fmt.Errorf("listing uploads: %w", err)
	}

	entries, err := os.ReadDir(uploadDir)
	if err != nil && !os.IsNotExist(err) {
		// Without the directory listing a sweep would miss recordless files
		// but could still delete safely, so this is a problem, not an abort.
		problems = append(problems, "failed to read the upload directory: "+err.Error())
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if known, ok := byName[name]; ok {
			known.OnDisk = true
			continue
		}
		info, err := entry.Info()
		if err != nil {
			problems = append(problems, "failed to stat "+name+": "+err.Error())
			continue
		}
		byName[name] = &uploadCandidate{
			Filename:  name,
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
			OnDisk:    true,
		}
	}

	candidates := make([]uploadCandidate, 0, len(byName))
	for _, c := range byName {
		candidates = append(candidates, *c)
	}
	return candidates, problems, nil
}

// sweepOrphanedUploads finds every upload no document references and, unless
// dryRun is set, deletes it from disk and from the uploads collection.
// Anything stored more recently than minAge is left alone.
//
// A failure to build the reference set or to list the uploads aborts the
// sweep — deleting without either would delete files that are in use.
// Per-file failures are collected and the sweep continues.
func sweepOrphanedUploads(ctx context.Context, db *database.MongoDB, uploadDir string, minAge time.Duration, dryRun bool) (*UploadSweepReport, error) {
	referenced, err := referencedUploadFilenames(ctx, db)
	if err != nil {
		return nil, err
	}

	candidates, problems, err := collectUploadCandidates(ctx, db, uploadDir)
	if err != nil {
		return nil, err
	}

	orphans, totalFiles, totalBytes, skipped := classifyUploads(candidates, referenced, time.Now().Add(-minAge))

	report := &UploadSweepReport{
		DryRun:           dryRun,
		TotalFiles:       totalFiles,
		TotalBytes:       totalBytes,
		SkippedTooRecent: skipped,
		Errors:           problems,
	}
	for _, orphan := range orphans {
		report.OrphanedFiles++
		report.OrphanedBytes += orphan.Size
	}
	report.Sample = orphans
	if len(report.Sample) > maxSweepSampleFiles {
		report.Sample = report.Sample[:maxSweepSampleFiles]
	}

	if dryRun {
		return report, nil
	}

	filenames := make([]string, 0, len(orphans))
	for _, orphan := range orphans {
		if err := os.Remove(filepath.Join(uploadDir, orphan.Filename)); err != nil && !os.IsNotExist(err) {
			report.Errors = append(report.Errors, "failed to delete file "+orphan.Filename+": "+err.Error())
			continue
		}
		filenames = append(filenames, orphan.Filename)
		report.DeletedFiles++
		report.DeletedBytes += orphan.Size
	}
	if len(filenames) > 0 {
		if _, err := db.Uploads().DeleteMany(ctx, bson.M{"filename": bson.M{"$in": filenames}}); err != nil {
			report.Errors = append(report.Errors, "failed to delete upload records: "+err.Error())
		}
	}
	log.Printf("[UploadSweep] deleted %d orphaned files (%d bytes), %d errors", report.DeletedFiles, report.DeletedBytes, len(report.Errors))

	return report, nil
}

// SweepUploads reports on, and optionally deletes, uploads nothing references
// any more — the backlog left by deletes and image replacements from before
// cleanup existed, plus imports abandoned before anything referenced them.
//
// It scans by default and only deletes when asked explicitly:
//
//	GET  /api/admin/uploads/orphans          scan, delete nothing
//	POST /api/admin/uploads/sweep            scan, delete nothing
//	POST /api/admin/uploads/sweep?delete=true  delete what it finds
//
// "minAgeHours" overrides how recent an upload has to be to be spared; 0 is
// allowed but will delete files an open editor may be about to reference.
func (h *AdminHandler) SweepUploads(c *gin.Context) {
	minAgeHours := defaultSweepMinAgeHours
	if raw := c.Query("minAgeHours"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "minAgeHours must be a non-negative number of hours"})
			return
		}
		minAgeHours = parsed
	}

	dryRun := c.Request.Method != http.MethodPost || c.Query("delete") != "true"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	report, err := sweepOrphanedUploads(ctx, h.DB, h.UploadDir, time.Duration(minAgeHours)*time.Hour, dryRun)
	if err != nil {
		log.Printf("[UploadSweep] aborted: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sweep failed, nothing was deleted: " + err.Error()})
		return
	}
	report.MinAgeHours = minAgeHours
	c.JSON(http.StatusOK, report)
}
