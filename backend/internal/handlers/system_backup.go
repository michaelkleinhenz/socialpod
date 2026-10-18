package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"socialmedia/internal/backup"
	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/objectstore"
	"socialmedia/internal/uploadstore"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Full-system backups: every collection (uploads included, which hold the
// bytes of every uploaded file) bundled as an AES-encrypted ZIP and stored
// under backups/ in the S3 bucket.

const (
	systemBackupPrefix     = "backups/"
	minBackupPassphraseLen = 12
	defaultBackupRetention = 7
	maxBackupRetention     = 365
	// maxBackupUploadBytes bounds uploaded archives for restore.
	maxBackupUploadBytes = 20 << 30
)

// BackupObjectStore is the bucket API needed to store backup archives.
type BackupObjectStore interface {
	PutReader(ctx context.Context, key, contentType string, r io.ReadSeeker, size int64) error
	GetReader(ctx context.Context, key string) (io.ReadCloser, int64, error)
	List(ctx context.Context, prefix string) ([]objectstore.ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

var backupFrequencies = map[string]time.Duration{
	"off":    0,
	"hourly": time.Hour,
	"6h":     6 * time.Hour,
	"12h":    12 * time.Hour,
	"daily":  24 * time.Hour,
	"weekly": 7 * 24 * time.Hour,
}

var backupNameRe = regexp.MustCompile(`^socialpod-backup-\d{8}-\d{6}\.\d{3}Z(-[a-z-]+)?\.zip$`)

// backupTimeLayout gives names that sort chronologically, down to the millisecond.
const backupTimeLayout = "20060102-150405.000Z"

var errBackupBusy = errors.New("a backup or restore is already running")

func backupObjectName(now time.Time, kind string) string {
	return fmt.Sprintf("socialpod-backup-%s-%s.zip", now.UTC().Format(backupTimeLayout), kind)
}

// SystemBackupHandler serves the admin backup API and runs scheduled backups.
type SystemBackupHandler struct {
	DB        *database.MongoDB
	UploadDir string
	// Store is the S3 bucket; nil when none is configured.
	Store BackupObjectStore
	// Agents and OAuth hold state a restore replaces; nil skips them.
	Agents *AgentHandler
	OAuth  *OAuthHandler

	// mu serialises backup and restore runs.
	mu sync.Mutex
	// running is true while a backup or restore holds mu.
	running atomic.Bool
	stop    chan struct{}
}

func (h *SystemBackupHandler) loadConfig(ctx context.Context) models.BackupConfig {
	var cfg models.BackupConfig
	if err := h.DB.BackupConfig().FindOne(ctx, bson.M{}).Decode(&cfg); err != nil {
		cfg = models.BackupConfig{
			ID:        primitive.NewObjectID(),
			Frequency: "off",
			Retention: defaultBackupRetention,
			UpdatedAt: time.Now(),
		}
		h.DB.BackupConfig().InsertOne(ctx, cfg) //nolint:errcheck
	}
	if _, ok := backupFrequencies[cfg.Frequency]; !ok {
		cfg.Frequency = "off"
	}
	if cfg.Retention < 1 {
		cfg.Retention = defaultBackupRetention
	}
	return cfg
}

type backupConfigResponse struct {
	models.BackupConfig
	PassphraseSet     bool `json:"passphraseSet"`
	StorageConfigured bool `json:"storageConfigured"`
	Running           bool `json:"running"`
	MinPassphraseLen  int  `json:"minPassphraseLength"`
}

func (h *SystemBackupHandler) configResponse(cfg models.BackupConfig) backupConfigResponse {
	return backupConfigResponse{
		BackupConfig:      cfg,
		PassphraseSet:     cfg.Passphrase != "",
		StorageConfigured: h.Store != nil,
		Running:           h.running.Load(),
		MinPassphraseLen:  minBackupPassphraseLen,
	}
}

// GetConfig serves GET /api/admin/backups/config.
func (h *SystemBackupHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.configResponse(h.loadConfig(c.Request.Context())))
}

type backupConfigUpdate struct {
	Frequency  *string `json:"frequency"`
	Retention  *int    `json:"retention"`
	Passphrase *string `json:"passphrase"`
}

// applyBackupConfigUpdate validates body and applies it to cfg, returning the
// fields to store.
func applyBackupConfigUpdate(cfg *models.BackupConfig, body backupConfigUpdate) (bson.M, error) {
	cfg.UpdatedAt = time.Now()
	set := bson.M{"updatedAt": cfg.UpdatedAt}
	if body.Frequency != nil {
		if _, ok := backupFrequencies[*body.Frequency]; !ok {
			return nil, errors.New("frequency must be one of off, hourly, 6h, 12h, daily, weekly")
		}
		cfg.Frequency = *body.Frequency
		set["frequency"] = cfg.Frequency
	}
	if body.Retention != nil {
		if *body.Retention < 1 || *body.Retention > maxBackupRetention {
			return nil, fmt.Errorf("retention must be between 1 and %d", maxBackupRetention)
		}
		cfg.Retention = *body.Retention
		set["retention"] = cfg.Retention
	}
	if body.Passphrase != nil {
		if len([]rune(*body.Passphrase)) < minBackupPassphraseLen {
			return nil, fmt.Errorf("passphrase must be at least %d characters", minBackupPassphraseLen)
		}
		cfg.Passphrase = *body.Passphrase
		set["passphrase"] = cfg.Passphrase
	}
	if cfg.Frequency != "off" && cfg.Passphrase == "" {
		return nil, errors.New("set a passphrase before enabling scheduled backups")
	}
	return set, nil
}

// UpdateConfig serves PUT /api/admin/backups/config.
func (h *SystemBackupHandler) UpdateConfig(c *gin.Context) {
	var body backupConfigUpdate
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ctx := c.Request.Context()
	cfg := h.loadConfig(ctx)
	set, err := applyBackupConfigUpdate(&cfg, body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := h.DB.BackupConfig().UpdateOne(ctx, bson.M{"_id": cfg.ID}, bson.M{"$set": set}, options.Update().SetUpsert(true)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[Backup] config updated: frequency=%s retention=%d passphraseChanged=%v", cfg.Frequency, cfg.Retention, body.Passphrase != nil)
	c.JSON(http.StatusOK, h.configResponse(cfg))
}

type backupListEntry struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	Kind      string    `json:"kind"`
}

func (h *SystemBackupHandler) listBackups(ctx context.Context) ([]backupListEntry, error) {
	objs, err := h.Store.List(ctx, systemBackupPrefix)
	if err != nil {
		return nil, err
	}
	out := []backupListEntry{}
	for _, o := range objs {
		name := strings.TrimPrefix(o.Key, systemBackupPrefix)
		if !backupNameRe.MatchString(name) {
			continue
		}
		e := backupListEntry{Name: name, Size: o.Size, CreatedAt: o.LastModified, Kind: "manual"}
		rest := strings.TrimSuffix(strings.TrimPrefix(name, "socialpod-backup-"), ".zip")
		tsLen := len(backupTimeLayout)
		if ts, err := time.Parse(backupTimeLayout, rest[:tsLen]); err == nil {
			e.CreatedAt = ts
		}
		if len(rest) > tsLen+1 {
			e.Kind = rest[tsLen+1:]
		}
		out = append(out, e)
	}
	// Newest first; the timestamp in the name sorts chronologically.
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

func (h *SystemBackupHandler) requireStore(c *gin.Context) bool {
	if h.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backup storage is not configured (set the AWS_S3_BUCKET_NAME, AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables)"})
		return false
	}
	return true
}

// backupNameParam returns the :name parameter if it is a backup name.
func backupNameParam(c *gin.Context) (string, bool) {
	name := c.Param("name")
	if !backupNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup name"})
		return "", false
	}
	return name, true
}

// List serves GET /api/admin/backups.
func (h *SystemBackupHandler) List(c *gin.Context) {
	if !h.requireStore(c) {
		return
	}
	list, err := h.listBackups(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// RunNow serves POST /api/admin/backups: it starts a backup in the background.
func (h *SystemBackupHandler) RunNow(c *gin.Context) {
	if !h.requireStore(c) {
		return
	}
	cfg := h.loadConfig(c.Request.Context())
	if cfg.Passphrase == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "set a backup passphrase first"})
		return
	}
	if !h.mu.TryLock() {
		c.JSON(http.StatusConflict, gin.H{"error": errBackupBusy.Error()})
		return
	}
	h.running.Store(true)
	go func() {
		defer h.mu.Unlock()
		defer h.running.Store(false)
		if _, err := h.runBackupLocked(context.Background(), "manual", true); err != nil {
			log.Printf("[Backup] manual backup failed: %v", err)
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

// Download serves GET /api/admin/backups/:name/download.
func (h *SystemBackupHandler) Download(c *gin.Context) {
	if !h.requireStore(c) {
		return
	}
	name, ok := backupNameParam(c)
	if !ok {
		return
	}
	body, size, err := h.Store.GetReader(c.Request.Context(), systemBackupPrefix+name)
	if errors.Is(err, objectstore.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer body.Close()
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.DataFromReader(http.StatusOK, size, "application/zip", body, nil)
}

// Delete serves DELETE /api/admin/backups/:name.
func (h *SystemBackupHandler) Delete(c *gin.Context) {
	if !h.requireStore(c) {
		return
	}
	name, ok := backupNameParam(c)
	if !ok {
		return
	}
	if err := h.Store.Delete(c.Request.Context(), systemBackupPrefix+name); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[Backup] deleted %s", name)
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// RestoreStored serves POST /api/admin/backups/:name/restore, body
// {"passphrase": "..."} (optional; defaults to the configured passphrase).
func (h *SystemBackupHandler) RestoreStored(c *gin.Context) {
	if !h.requireStore(c) {
		return
	}
	name, ok := backupNameParam(c)
	if !ok {
		return
	}
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil && err != io.EOF {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
	}
	if !h.mu.TryLock() {
		c.JSON(http.StatusConflict, gin.H{"error": errBackupBusy.Error()})
		return
	}
	h.running.Store(true)
	defer h.mu.Unlock()
	defer h.running.Store(false)

	ctx := c.Request.Context()
	tmp, err := os.CreateTemp("", "socialpod-restore-*.zip")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	src, _, err := h.Store.GetReader(ctx, systemBackupPrefix+name)
	if errors.Is(err, objectstore.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	size, err := io.Copy(tmp, src)
	src.Close()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "downloading backup: " + err.Error()})
		return
	}
	h.restoreFrom(c, tmp, size, body.Passphrase, name)
}

// RestoreUpload serves POST /api/admin/backups/restore-upload, multipart with
// the fields passphrase and file.
func (h *SystemBackupHandler) RestoreUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackupUploadBytes)
	mr, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expected multipart form upload"})
		return
	}
	if !h.mu.TryLock() {
		c.JSON(http.StatusConflict, gin.H{"error": errBackupBusy.Error()})
		return
	}
	h.running.Store(true)
	defer h.mu.Unlock()
	defer h.running.Store(false)

	tmp, err := os.CreateTemp("", "socialpod-restore-*.zip")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	var passphrase, filename string
	var size int64
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reading upload: " + err.Error()})
			return
		}
		switch part.FormName() {
		case "passphrase":
			b, _ := io.ReadAll(io.LimitReader(part, 4096))
			passphrase = string(b)
		case "file":
			filename = part.FileName()
			if size, err = io.Copy(tmp, part); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "reading upload: " + err.Error()})
				return
			}
		}
		part.Close()
	}
	if size == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no backup file uploaded"})
		return
	}
	h.restoreFrom(c, tmp, size, passphrase, "upload:"+filename)
}

// restoreFrom validates the archive, takes a safety backup of the current
// state (when storage and a passphrase are configured) and then restores.
// The caller holds h.mu.
func (h *SystemBackupHandler) restoreFrom(c *gin.Context, f *os.File, size int64, passphrase, source string) {
	ctx := c.Request.Context()
	cfg := h.loadConfig(ctx)
	if passphrase == "" {
		passphrase = cfg.Passphrase
	}
	if passphrase == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "passphrase required"})
		return
	}
	zr, err := backup.NewReader(f, size, passphrase)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate everything before touching any data.
	if _, err := backup.Validate(zr); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, backup.ErrWrongPassphrase) {
			// Not 401: the frontend treats 401 as an expired session.
			status = http.StatusUnprocessableEntity
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	var safety string
	if h.Store != nil && cfg.Passphrase != "" {
		name, err := h.runBackupLocked(ctx, "pre-restore", false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "safety backup before restore failed, nothing was changed: " + err.Error()})
			return
		}
		safety = name
	}

	log.Printf("[Backup] restoring from %s", source)
	// Agent runs in progress would go on writing into the data being
	// replaced; their state in the backup is closed after the restore.
	if h.Agents != nil {
		h.Agents.cancelAll()
	}
	// Detach from the request so a closed browser tab can't abort a
	// half-finished restore.
	rctx := context.WithoutCancel(ctx)
	opts := backup.RestoreOptions{KeepIfPresent: []string{"backup_config"}}
	if uploadstore.UsesBucket() {
		opts.Files = bucketFiles{db: h.DB}
	}
	res, err := backup.Restore(rctx, backup.MongoStore{DB: h.DB.Database}, zr, opts)
	if err != nil {
		log.Printf("[Backup] restore from %s failed: %v", source, err)
		msg := "restore failed: " + err.Error()
		if safety != "" {
			msg += " (the state before the restore was saved as " + safety + ")"
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
		return
	}
	h.DB.EnsureIndexes()
	// Agents that were running when the backup was taken would otherwise
	// stay marked running, refused a run until the next server start.
	if h.Agents != nil {
		h.Agents.recoverInterrupted("Interrupted by a backup restore")
	}
	// The ID token signing key is the backup's now.
	if h.OAuth != nil {
		h.OAuth.resetSigningKey()
	}
	// A backup from before the media library held the watermarks brings
	// their old collection back.
	if n, err := MigrateWatermarksToMedia(rctx, h.DB, h.UploadDir); err != nil {
		log.Printf("[Backup] moving restored watermarks into the media library: %v (%d moved)", err, n)
	}
	if err := pruneUploadCache(rctx, h.DB, h.UploadDir); err != nil {
		log.Printf("[Backup] pruning %s after restore: %v", h.UploadDir, err)
	}
	// A backup from before the bucket brings its files back inside their
	// records; move them out again.
	if n, err := uploadstore.MigrateToBucket(rctx, h.DB); err != nil {
		log.Printf("[Backup] moving restored uploads to the bucket: %v (%d moved)", err, n)
	} else if n > 0 {
		log.Printf("[Backup] moved %d restored uploads to the bucket", n)
	}
	log.Printf("[Backup] restore from %s complete: %d collections", source, len(res.Collections))
	c.JSON(http.StatusOK, gin.H{
		"restored":     true,
		"safetyBackup": safety,
		"collections":  res.Collections,
	})
}

// bucketFiles gives backups the uploads kept in the S3 bucket: every record
// whose bytes are there.
type bucketFiles struct{ db *database.MongoDB }

func (f bucketFiles) Each(ctx context.Context, fn func(name string, data []byte) error) error {
	cur, err := f.db.Uploads().Find(ctx, bson.M{"storage": uploadstore.StorageBucket},
		options.Find().SetProjection(bson.M{"filename": 1}).SetNoCursorTimeout(true))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var u models.Upload
		if err := cur.Decode(&u); err != nil {
			return err
		}
		data, _, err := uploadstore.GetBucket(ctx, u.Filename)
		if errors.Is(err, objectstore.ErrNotFound) {
			// Deleted since the records were dumped, or lost before; the
			// backup goes on without it.
			log.Printf("[Backup] upload %s is not in the bucket, left out", u.Filename)
			continue
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", u.Filename, err)
		}
		if err := fn(u.Filename, data); err != nil {
			return err
		}
	}
	return cur.Err()
}

func (f bucketFiles) Write(ctx context.Context, name string, data []byte) error {
	return uploadstore.PutBucket(ctx, name, "", data)
}

// pruneUploadCache removes the files in UPLOAD_DIR that no restored record
// knows, so the cache does not serve the replaced state. The rest refill from
// the bucket (or the records) when read.
func pruneUploadCache(ctx context.Context, db *database.MongoDB, uploadDir string) error {
	if uploadDir == "" {
		return nil
	}
	names, err := db.Uploads().Distinct(ctx, "filename", bson.M{})
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(names))
	for _, n := range names {
		if s, ok := n.(string); ok {
			keep[s] = true
		}
	}
	entries, err := os.ReadDir(uploadDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Type().IsRegular() && !keep[e.Name()] {
			os.Remove(filepath.Join(uploadDir, e.Name())) //nolint:errcheck
		}
	}
	return nil
}

// ── Backup execution ─────────────────────────────────────────────────────────

// runBackupLocked writes a backup to the bucket, records the outcome and
// (when prune is set) applies retention. The caller holds h.mu.
func (h *SystemBackupHandler) runBackupLocked(ctx context.Context, kind string, prune bool) (string, error) {
	cfg := h.loadConfig(ctx)
	now := time.Now()
	name, err := h.writeBackup(ctx, cfg.Passphrase, kind, now)
	set := bson.M{"lastAttemptAt": now}
	if err != nil {
		set["lastError"] = err.Error()
	} else {
		set["lastError"] = ""
		set["lastSuccessAt"] = now
		set["lastName"] = name
	}
	h.DB.BackupConfig().UpdateOne(ctx, bson.M{"_id": cfg.ID}, bson.M{"$set": set}) //nolint:errcheck
	if err != nil {
		return "", err
	}
	log.Printf("[Backup] stored %s", name)
	if prune {
		h.prune(ctx, cfg.Retention)
	}
	return name, nil
}

func (h *SystemBackupHandler) writeBackup(ctx context.Context, passphrase, kind string, now time.Time) (string, error) {
	if h.Store == nil {
		return "", fmt.Errorf("backup storage is not configured")
	}
	if passphrase == "" {
		return "", fmt.Errorf("backup passphrase is not set")
	}
	tmp, err := os.CreateTemp("", "socialpod-backup-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	var files backup.Files
	if uploadstore.UsesBucket() {
		files = bucketFiles{db: h.DB}
	}
	if _, err := backup.Create(ctx, backup.MongoStore{DB: h.DB.Database}, tmp, passphrase, kind, files); err != nil {
		return "", err
	}
	size, err := tmp.Seek(0, io.SeekEnd)
	if err != nil {
		return "", err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	name := backupObjectName(now, kind)
	if err := h.Store.PutReader(ctx, systemBackupPrefix+name, "application/zip", tmp, size); err != nil {
		return "", fmt.Errorf("uploading backup: %w", err)
	}
	return name, nil
}

// prune deletes all but the newest retention backups. Every kind counts,
// safety backups taken before a restore included.
func (h *SystemBackupHandler) prune(ctx context.Context, retention int) {
	if retention < 1 {
		return
	}
	list, err := h.listBackups(ctx)
	if err != nil {
		log.Printf("[Backup] retention: listing failed: %v", err)
		return
	}
	for i := retention; i < len(list); i++ {
		if err := h.Store.Delete(ctx, systemBackupPrefix+list[i].Name); err != nil {
			log.Printf("[Backup] retention: deleting %s failed: %v", list[i].Name, err)
			continue
		}
		log.Printf("[Backup] retention: deleted %s", list[i].Name)
	}
}

// systemBackupDue reports whether a scheduled backup should run now. After a
// failure it retries at most every 30 minutes.
func systemBackupDue(cfg models.BackupConfig, now time.Time) bool {
	interval := backupFrequencies[cfg.Frequency]
	if interval == 0 || cfg.Passphrase == "" {
		return false
	}
	if cfg.LastSuccessAt != nil && now.Sub(*cfg.LastSuccessAt) < interval {
		return false
	}
	retry := min(interval, 30*time.Minute)
	if cfg.LastAttemptAt != nil && now.Sub(*cfg.LastAttemptAt) < retry {
		return false
	}
	return true
}

// tick runs a scheduled backup if one is due.
func (h *SystemBackupHandler) tick(now time.Time) error {
	if h.Store == nil {
		return nil
	}
	ctx := context.Background()
	if !systemBackupDue(h.loadConfig(ctx), now) {
		return nil
	}
	if !h.mu.TryLock() {
		return nil
	}
	h.running.Store(true)
	defer h.mu.Unlock()
	defer h.running.Store(false)
	_, err := h.runBackupLocked(ctx, "scheduled", true)
	return err
}

// StartScheduler checks every five minutes whether a scheduled backup is due.
func (h *SystemBackupHandler) StartScheduler() {
	h.stop = make(chan struct{})
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			if err := h.tick(time.Now()); err != nil {
				log.Printf("[Backup] scheduled backup failed: %v", err)
			}
			select {
			case <-t.C:
			case <-h.stop:
				return
			}
		}
	}()
}

// StopScheduler stops the loop StartScheduler started.
func (h *SystemBackupHandler) StopScheduler() {
	if h.stop != nil {
		close(h.stop)
	}
}
