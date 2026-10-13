package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"socialmedia/internal/models"
	"socialmedia/internal/objectstore"

	"github.com/gin-gonic/gin"
)

func TestBackupObjectNameMatchesPattern(t *testing.T) {
	now := time.Date(2026, 10, 13, 4, 5, 6, 789e6, time.UTC)
	for _, kind := range []string{"manual", "scheduled", "pre-restore"} {
		name := backupObjectName(now, kind)
		if !backupNameRe.MatchString(name) {
			t.Errorf("%s does not match the backup name pattern", name)
		}
	}
	for _, bad := range []string{"../x.zip", "socialpod-backup-x.zip", "socialpod-backup-20261013-040506.789Z-manual.zip/../a"} {
		if backupNameRe.MatchString(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestSystemBackupDue(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	cases := []struct {
		name string
		cfg  models.BackupConfig
		want bool
	}{
		{"off", models.BackupConfig{Frequency: "off", Passphrase: "x"}, false},
		{"no passphrase", models.BackupConfig{Frequency: "daily"}, false},
		{"never ran", models.BackupConfig{Frequency: "daily", Passphrase: "x"}, true},
		{"ran recently", models.BackupConfig{Frequency: "daily", Passphrase: "x", LastSuccessAt: ago(2 * time.Hour), LastAttemptAt: ago(2 * time.Hour)}, false},
		{"interval passed", models.BackupConfig{Frequency: "daily", Passphrase: "x", LastSuccessAt: ago(25 * time.Hour), LastAttemptAt: ago(25 * time.Hour)}, true},
		{"failed just now", models.BackupConfig{Frequency: "daily", Passphrase: "x", LastSuccessAt: ago(25 * time.Hour), LastAttemptAt: ago(10 * time.Minute)}, false},
		{"retry after failure", models.BackupConfig{Frequency: "daily", Passphrase: "x", LastSuccessAt: ago(25 * time.Hour), LastAttemptAt: ago(31 * time.Minute)}, true},
	}
	for _, c := range cases {
		if got := systemBackupDue(c.cfg, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestApplyBackupConfigUpdate(t *testing.T) {
	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }

	cfg := models.BackupConfig{Frequency: "off", Retention: 7}
	if _, err := applyBackupConfigUpdate(&cfg, backupConfigUpdate{Frequency: str("daily")}); err == nil {
		t.Error("enabling a schedule without a passphrase should fail")
	}
	cfg = models.BackupConfig{Frequency: "off", Retention: 7}
	if _, err := applyBackupConfigUpdate(&cfg, backupConfigUpdate{Passphrase: str("short")}); err == nil {
		t.Error("a short passphrase should fail")
	}
	for _, r := range []int{0, maxBackupRetention + 1} {
		cfg = models.BackupConfig{Frequency: "off", Retention: 7}
		if _, err := applyBackupConfigUpdate(&cfg, backupConfigUpdate{Retention: num(r)}); err == nil {
			t.Errorf("retention %d should fail", r)
		}
	}
	cfg = models.BackupConfig{Frequency: "off", Retention: 7}
	if _, err := applyBackupConfigUpdate(&cfg, backupConfigUpdate{Frequency: str("monthly")}); err == nil {
		t.Error("an unknown frequency should fail")
	}
	cfg = models.BackupConfig{Frequency: "off", Retention: 7}
	set, err := applyBackupConfigUpdate(&cfg, backupConfigUpdate{Frequency: str("weekly"), Retention: num(3), Passphrase: str("a long enough passphrase")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Frequency != "weekly" || cfg.Retention != 3 || cfg.Passphrase != "a long enough passphrase" || set["passphrase"] != cfg.Passphrase {
		t.Fatalf("update not applied: %+v %v", cfg, set)
	}
}

func TestBackupListAndPrune(t *testing.T) {
	ctx := context.Background()
	store := objectstore.NewMemory()
	h := &SystemBackupHandler{Store: store}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	kinds := []string{"scheduled", "manual", "pre-restore", "scheduled", "scheduled"}
	for i, k := range kinds {
		store.Put(ctx, systemBackupPrefix+backupObjectName(base.Add(time.Duration(i)*time.Hour), k), "application/zip", []byte("zip"))
	}
	store.Put(ctx, systemBackupPrefix+"notes.txt", "text/plain", []byte("ignored"))

	list, err := h.listBackups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 5 {
		t.Fatalf("listed %d backups, want 5", len(list))
	}
	if !list[0].CreatedAt.Equal(base.Add(4*time.Hour)) || list[0].Kind != "scheduled" || list[2].Kind != "pre-restore" {
		t.Fatalf("list not newest first or kinds wrong: %+v", list)
	}

	h.prune(ctx, 2)
	list, _ = h.listBackups(ctx)
	if len(list) != 2 || !list[1].CreatedAt.Equal(base.Add(3*time.Hour)) {
		t.Fatalf("prune kept the wrong backups: %+v", list)
	}
}

func TestBackupRoutesWithoutStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SystemBackupHandler{}
	r := gin.New()
	// The same shape as main.go, so a route conflict panics here.
	r.GET("/backups", h.List)
	r.POST("/backups", h.RunNow)
	r.POST("/backups/restore-upload", h.RestoreUpload)
	r.GET("/backups/:name/download", h.Download)
	r.POST("/backups/:name/restore", h.RestoreStored)
	r.DELETE("/backups/:name", h.Delete)

	for _, path := range []string{"/backups", "/backups/socialpod-backup-20261013-040506.789Z-manual.zip/download"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s without storage: %d, want 503", path, w.Code)
		}
	}
}
