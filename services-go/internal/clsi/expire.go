package clsi

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Forgetting old projects.
//
// A compile directory is a whole project plus everything LaTeX wrote beside
// it, and an output directory is several copies of a PDF. Nothing removes them
// when somebody stops working on a project, so this does: they are a cache,
// and the next compile writes them again.

// sweepEvery is how often the directories are looked at. Often enough that a
// busy site does not fill its disk between sweeps, rarely enough that it is not
// walking the disk continuously.
const sweepEvery = 30 * time.Minute

// Expire removes what has not been compiled for a while, until the context
// ends.
func (s *Service) Expire(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(sweepEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweep()
			}
		}
	}()
}

func (s *Service) sweep() {
	cutoff := time.Now().Add(-s.options.ProjectLife)
	for _, root := range []string{s.options.CompilesDir, s.options.OutputDir} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if err := os.RemoveAll(path); err != nil && s.log != nil {
				s.log.Warn("could not remove an expired compile directory",
					slog.String("path", path), slog.Any("err", err))
			}
		}
	}
}
