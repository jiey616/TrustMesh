package store

import (
	"go.uber.org/zap"
	"trustmesh/backend/internal/config"
)

func NewWithConfig(cfg config.Config, log *zap.Logger) (*Store, error) {
	s := New()
	s.log = log
	s.opsRuntime = opsRuntimeFromConfig(cfg)
	if cfg.MongoEnabled {
		if err := s.enableMongo(cfg, log); err != nil {
			return nil, err
		}
	}
	return s, nil
}
