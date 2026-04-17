package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type DailyWriter struct {
	mu sync.Mutex

	dir      string
	baseName string
	levelTag string

	dailyRotate bool
	alsoBySize  bool
	timeNow     func() time.Time

	maxSizeMB  int
	maxBackups int
	maxAgeDays int
	compress   bool

	curDate  string
	curPath  string
	lj       *lumberjack.Logger
	file     *os.File
	useLocal bool
}

func NewDailyWriter(
	dir, baseName, levelTag string,
	dailyRotate, alsoBySize bool,
	maxSizeMB, maxBackups, maxAgeDays int,
	compress bool,
) (*DailyWriter, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "storage/logs"
	}
	if strings.TrimSpace(baseName) == "" {
		baseName = "app"
	}

	w := &DailyWriter{
		dir:         dir,
		baseName:    baseName,
		levelTag:    strings.ToUpper(strings.TrimSpace(levelTag)),
		dailyRotate: dailyRotate,
		alsoBySize:  alsoBySize,
		timeNow:     time.Now,
		maxSizeMB:   clampMin(maxSizeMB, 1),
		maxBackups:  clampMin(maxBackups, 0),
		maxAgeDays:  clampMin(maxAgeDays, 0),
		compress:    compress,
		useLocal:    true,
	}

	if err := w.ensureOpenLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *DailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.reopenIfNeededLocked(); err != nil {
		return 0, err
	}

	if w.alsoBySize {
		return w.lj.Write(p)
	}

	return w.file.Write(p)
}

func (w *DailyWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}

func (w *DailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closeLocked()
}

func (w *DailyWriter) reopenIfNeededLocked() error {
	nextPath := w.buildFilename(currentDate(w.timeNow, w.dailyRotate))
	if nextPath != w.curPath {
		_ = w.closeLocked()
		return w.ensureOpenLocked()
	}

	if _, err := os.Stat(w.curPath); err != nil {
		if os.IsNotExist(err) {
			_ = w.closeLocked()
			return w.ensureOpenLocked()
		}
		return fmt.Errorf("stat log file: %w", err)
	}

	return nil
}

func (w *DailyWriter) closeLocked() error {
	if w.lj != nil {
		err := w.lj.Close()
		w.lj = nil
		w.curPath = ""
		return err
	}

	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		w.curPath = ""
		return err
	}

	return nil
}

func (w *DailyWriter) ensureOpenLocked() error {
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}

	date := currentDate(w.timeNow, w.dailyRotate)
	w.curDate = date
	filename := w.buildFilename(date)

	if w.alsoBySize {
		w.lj = &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    w.maxSizeMB,
			MaxBackups: w.maxBackups,
			MaxAge:     w.maxAgeDays,
			Compress:   w.compress,
			LocalTime:  w.useLocal,
		}
		w.file = nil
		w.curPath = filename
		return nil
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", filename, err)
	}

	w.file = file
	w.lj = nil
	w.curPath = filename
	return nil
}

func (w *DailyWriter) buildFilename(date string) string {
	parts := []string{w.baseName}
	if w.levelTag != "" {
		parts = append(parts, w.levelTag)
	}
	if w.dailyRotate && date != "" {
		parts = append(parts, date)
	}
	return filepath.Join(w.dir, strings.Join(parts, ".")+".log")
}

func currentDate(now func() time.Time, dailyRotate bool) string {
	if !dailyRotate {
		return ""
	}
	return now().Format("2006-01-02")
}

func clampMin(v, min int) int {
	if v < min {
		return min
	}
	return v
}
