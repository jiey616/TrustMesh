package store

import (
	"context"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// Cleanup configuration constants.
const (
	cleanupInterval       = 10 * time.Minute // run cleanup every 10 minutes
	maxEventAge           = 7 * 24 * time.Hour // keep events for 7 days
	maxEventsPerTask      = 1000               // max events per task before truncation
	maxEventsPerUser      = 2000               // max events per user before truncation
	maxEventsPerAgent     = 2000               // max events per agent before truncation
	maxNotificationsPerUser = 500              // max notifications per user
	maxProcessedMessages  = 10000              // max processed message dedup entries
	maxChatMessages       = 200                // max messages per agent chat session
	truncatedKeepCount    = 500                // when truncating, keep this many newest
)

// StartCleanupTicker runs a background goroutine that periodically cleans up
// stale in-memory data to prevent unbounded memory growth (OOM).
// It should be called once at application startup.
func (s *Store) StartCleanupTicker(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	s.log.Info("store cleanup ticker started", zap.Duration("interval", cleanupInterval))

	for {
		select {
		case <-ctx.Done():
			s.log.Info("store cleanup ticker stopped")
			return
		case <-ticker.C:
			s.runCleanup()
		}
	}
}

// runCleanup performs a single cleanup pass over all in-memory collections.
func (s *Store) runCleanup() {
	cutoff := time.Now().UTC().Add(-maxEventAge)

	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()

	// 1. Clean up taskEvents: remove events older than maxEventAge
	taskEventsCleaned := 0
	for taskID, events := range s.taskEvents {
		if len(events) == 0 {
			delete(s.taskEvents, taskID)
			continue
		}
		// Age-based cleanup
		filtered := filterEventsByAge(events, cutoff)
		// Count-based cap
		if len(filtered) > maxEventsPerTask {
			filtered = filtered[len(filtered)-truncatedKeepCount:]
		}
		if len(filtered) < len(events) {
			taskEventsCleaned += len(events) - len(filtered)
		}
		if len(filtered) == 0 {
			delete(s.taskEvents, taskID)
		} else {
			s.taskEvents[taskID] = filtered
		}
	}

	// 2. Clean up userEvents
	userEventsCleaned := 0
	for userID, events := range s.userEvents {
		if len(events) == 0 {
			delete(s.userEvents, userID)
			continue
		}
		filtered := filterUserEventsByAge(events, cutoff)
		if len(filtered) > maxEventsPerUser {
			filtered = filtered[len(filtered)-truncatedKeepCount:]
		}
		if len(filtered) < len(events) {
			userEventsCleaned += len(events) - len(filtered)
		}
		if len(filtered) == 0 {
			delete(s.userEvents, userID)
		} else {
			s.userEvents[userID] = filtered
		}
	}

	// 3. Clean up agentEvents
	agentEventsCleaned := 0
	for agentID, events := range s.agentEvents {
		if len(events) == 0 {
			delete(s.agentEvents, agentID)
			continue
		}
		filtered := filterAgentEventsByAge(events, cutoff)
		if len(filtered) > maxEventsPerAgent {
			filtered = filtered[len(filtered)-truncatedKeepCount:]
		}
		if len(filtered) < len(events) {
			agentEventsCleaned += len(events) - len(filtered)
		}
		if len(filtered) == 0 {
			delete(s.agentEvents, agentID)
		} else {
			s.agentEvents[agentID] = filtered
		}
	}

	// 4. Clean up notifications: keep only most recent N per user
	notificationsCleaned := 0
	for userID, ids := range s.userNotifications {
		if len(ids) <= maxNotificationsPerUser {
			continue
		}
		// Keep only the most recent N notification IDs
		keepCount := maxNotificationsPerUser
		removed := ids[:len(ids)-keepCount]
		for _, id := range removed {
			delete(s.notifications, id)
			notificationsCleaned++
		}
		s.userNotifications[userID] = ids[len(ids)-keepCount:]
	}

	// 5. Clean up processedMessages: cap total size
	processedMessagesCleaned := 0
	if len(s.processedMessages) > maxProcessedMessages {
		// Remove oldest entries (we don't track insertion order for processedMessages,
		// so we just clear the map and let new messages repopulate it).
		// This is safe because processedMessages is only a dedup cache — losing it
		// just means a small risk of reprocessing, which the system handles gracefully.
		s.processedMessages = make(map[string]processedMessage)
		processedMessagesCleaned = maxProcessedMessages
	}

	// 6. Clean up agent chat messages: cap per chat session
	chatMessagesCleaned := 0
	for _, chat := range s.agentChats {
		if len(chat.Messages) <= maxChatMessages {
			continue
		}
		// Keep only the most recent N messages
		keepCount := maxChatMessages
		chat.Messages = chat.Messages[len(chat.Messages)-keepCount:]
		chatMessagesCleaned += len(chat.Messages) - keepCount
	}

	elapsed := time.Since(start)
	if taskEventsCleaned > 0 || userEventsCleaned > 0 || agentEventsCleaned > 0 ||
		notificationsCleaned > 0 || processedMessagesCleaned > 0 || chatMessagesCleaned > 0 {
		s.log.Info("store cleanup completed",
			zap.Int("task_events_cleaned", taskEventsCleaned),
			zap.Int("user_events_cleaned", userEventsCleaned),
			zap.Int("agent_events_cleaned", agentEventsCleaned),
			zap.Int("notifications_cleaned", notificationsCleaned),
			zap.Int("processed_messages_cleaned", processedMessagesCleaned),
			zap.Int("chat_messages_cleaned", chatMessagesCleaned),
			zap.Duration("elapsed", elapsed),
		)
	}
}

// filterEventsByAge returns only events newer than the cutoff time.
func filterEventsByAge(events []model.Event, cutoff time.Time) []model.Event {
	if len(events) == 0 {
		return nil
	}
	// Events are appended in chronological order, so we can find the first
	// event that's newer than cutoff and slice from there.
	idx := -1
	for i, e := range events {
		if e.CreatedAt.After(cutoff) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil // all events are older than cutoff
	}
	return events[idx:]
}

// filterUserEventsByAge returns only user events newer than the cutoff time.
func filterUserEventsByAge(events []*model.Event, cutoff time.Time) []*model.Event {
	if len(events) == 0 {
		return nil
	}
	idx := -1
	for i, e := range events {
		if e != nil && e.CreatedAt.After(cutoff) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	return events[idx:]
}

// filterAgentEventsByAge returns only agent events newer than the cutoff time.
func filterAgentEventsByAge(events []*model.Event, cutoff time.Time) []*model.Event {
	return filterUserEventsByAge(events, cutoff) // same logic
}
