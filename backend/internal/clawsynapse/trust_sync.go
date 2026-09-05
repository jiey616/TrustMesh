package clawsynapse

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/store"
)

type TrustRequestSyncer struct {
	client   *Client
	store    *store.Store
	interval time.Duration
	log      *zap.Logger
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func NewTrustRequestSyncer(client *Client, st *store.Store, interval time.Duration, log *zap.Logger) *TrustRequestSyncer {
	if client == nil || st == nil || interval <= 0 {
		return nil
	}
	return &TrustRequestSyncer{
		client:   client,
		store:    st,
		interval: interval,
		log:      log,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (s *TrustRequestSyncer) Start() {
	if s == nil {
		return
	}

	go func() {
		defer close(s.doneCh)
		s.sync()

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.sync()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *TrustRequestSyncer) Close() {
	if s == nil {
		return
	}
	close(s.stopCh)
	<-s.doneCh
}

// trustRequestReason is the JSON structure embedded in the trust request reason field.
type trustRequestReason struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
	AgentProduct string   `json:"agent_product"`
	UserID       string   `json:"user_id"`
	OrgID        string   `json:"org_id"`
}

func (s *TrustRequestSyncer) sync() {
	ctx, cancel := context.WithTimeout(context.Background(), s.interval)
	defer cancel()

	items, err := s.client.GetPendingTrustRequests(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("clawsynapse trust request sync failed", zap.Error(err))
		}
		return
	}

	created := 0
	for _, item := range items {
		if s.store.HasTrustRequest(item.RequestID) {
			continue
		}

		// Parse agent profile from reason JSON
		var profile trustRequestReason
		if err := json.Unmarshal([]byte(item.Reason), &profile); err != nil {
			// Fallback: use raw reason as description, node ID as name
			profile.Name = item.From
			profile.Description = item.Reason
			profile.Role = "custom"
		}

		receivedAt := time.UnixMilli(item.ReceivedAtMs).UTC()
		if item.ReceivedAtMs <= 0 {
			receivedAt = time.Now().UTC()
		}

		if _, appErr := s.store.CreateJoinRequest(store.CreateJoinRequestInput{
			TrustRequestID: item.RequestID,
			UserID:         profile.UserID,
			OrgID:          profile.OrgID,
			NodeID:         item.From,
			Name:           profile.Name,
			Description:    profile.Description,
			Role:           profile.Role,
			Capabilities:   profile.Capabilities,
			AgentProduct:   profile.AgentProduct,
			ReceivedAt:     receivedAt,
		}); appErr != nil {
			if s.log != nil {
				s.log.Warn("failed to create join request",
					zap.String("trust_request_id", item.RequestID),
					zap.String("from", item.From),
					zap.String("error", appErr.Message),
				)
			}
			continue
		}
		created++
	}

	if s.log != nil && created > 0 {
		s.log.Info("clawsynapse trust sync created join requests", zap.Int("count", created))
	}
}

