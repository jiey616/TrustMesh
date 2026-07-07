package store

import (
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

const (
	userStreamBufferSize    = 16
	maxSubscriptionsPerUser = 5 // max concurrent SSE connections per user
)

func (s *Store) SubscribeUser(userID string) (<-chan model.UserStreamEvent, func()) {
	ch := make(chan model.UserStreamEvent, userStreamBufferSize)

	s.streamMu.Lock()

	// Enforce max connections per user: if at limit, evict the oldest subscriber.
	if s.userSubscribers[userID] == nil {
		s.userSubscribers[userID] = make(map[chan model.UserStreamEvent]struct{})
	}
	if len(s.userSubscribers[userID]) >= maxSubscriptionsPerUser {
		// Evict the oldest channel (map iteration is random, but this is acceptable
		// since all channels are equivalent — the client will reconnect).
		for oldest := range s.userSubscribers[userID] {
			delete(s.userSubscribers[userID], oldest)
			close(oldest)
			if s.log != nil {
				s.log.Info("sse connection evicted to enforce per-user limit",
					zap.String("user_id", userID),
					zap.Int("limit", maxSubscriptionsPerUser),
				)
			}
			break
		}
	}

	s.userSubscribers[userID][ch] = struct{}{}
	s.streamMu.Unlock()

	return ch, func() {
		s.streamMu.Lock()
		if subscribers, ok := s.userSubscribers[userID]; ok {
			delete(subscribers, ch)
			if len(subscribers) == 0 {
				delete(s.userSubscribers, userID)
			}
		}
		s.streamMu.Unlock()
		close(ch)
	}
}

func (s *Store) publishUserEventUnsafe(userID, eventType string, payload map[string]any, at time.Time) {
	if userID == "" {
		return
	}

	event := model.UserStreamEvent{
		ID:         newID(),
		Type:       eventType,
		OccurredAt: at.UTC(),
		Payload:    copyMap(payload),
	}

	s.streamMu.RLock()
	defer s.streamMu.RUnlock()

	for ch := range s.userSubscribers[userID] {
		sendLatestUserEvent(ch, event)
	}
}

func sendLatestUserEvent(ch chan model.UserStreamEvent, event model.UserStreamEvent) {
	select {
	case ch <- event:
		return
	default:
	}

	select {
	case <-ch:
	default:
	}

	select {
	case ch <- event:
	default:
	}
}
