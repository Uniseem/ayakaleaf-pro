package docstore

import "context"

// NewPersistor builds the archiving backend named by the configuration.
//
// server-ce never sets BACKEND -- settings.js has no docstore section at all --
// so archiving is off in every self-hosted deployment and this returns the
// disabled persistor. The S3 and GCS backends are wired up separately.
func NewPersistor(ctx context.Context, cfg ArchiveConfig) (Persistor, error) {
	if !cfg.Enabled() {
		return DisabledPersistor{}, nil
	}
	return newObjectPersistor(ctx, cfg)
}
