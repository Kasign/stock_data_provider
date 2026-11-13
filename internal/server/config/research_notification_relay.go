package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	notificationschema "example.com/stock-analysis-flow/internal/notification/schema"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// ResearchNotificationRelay contains Server-owned delivery policy and the
// explicit resource limits required by Notification's immutable Schema Registry.
type ResearchNotificationRelay struct {
	WorkerID      string
	BatchSize     int
	PollInterval  time.Duration
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	EventLimits   notificationschema.ResourceLimits
}

func LoadResearchNotificationRelay(env platformconfig.Env) (ResearchNotificationRelay, error) {
	cfg := ResearchNotificationRelay{
		WorkerID:      strings.TrimSpace(env.String("RESEARCH_NOTIFICATION_RELAY_WORKER_ID", "server-research-notification-relay")),
		BatchSize:     20,
		PollInterval:  time.Second,
		LeaseDuration: 30 * time.Second,
		RetryDelay:    30 * time.Second,
	}
	var err error
	if cfg.BatchSize, err = env.Int("RESEARCH_NOTIFICATION_RELAY_BATCH_SIZE", cfg.BatchSize); err != nil {
		return ResearchNotificationRelay{}, err
	}
	if cfg.PollInterval, err = env.Duration("RESEARCH_NOTIFICATION_RELAY_POLL_INTERVAL", cfg.PollInterval); err != nil {
		return ResearchNotificationRelay{}, err
	}
	if cfg.LeaseDuration, err = env.Duration("RESEARCH_NOTIFICATION_RELAY_LEASE_DURATION", cfg.LeaseDuration); err != nil {
		return ResearchNotificationRelay{}, err
	}
	if cfg.RetryDelay, err = env.Duration("RESEARCH_NOTIFICATION_RELAY_RETRY_DELAY", cfg.RetryDelay); err != nil {
		return ResearchNotificationRelay{}, err
	}
	if cfg.EventLimits.MaxEnvelopeBytes, err = requiredPositiveInt(env, "NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES", 64<<20); err != nil {
		return ResearchNotificationRelay{}, err
	}
	if cfg.EventLimits.MaxPayloadBytes, err = requiredPositiveInt(env, "NOTIFICATION_EVENT_MAX_PAYLOAD_BYTES", cfg.EventLimits.MaxEnvelopeBytes); err != nil {
		return ResearchNotificationRelay{}, err
	}
	if cfg.EventLimits.MaxJSONDepth, err = requiredPositiveInt(env, "NOTIFICATION_EVENT_MAX_JSON_DEPTH", 128); err != nil {
		return ResearchNotificationRelay{}, err
	}
	if len(cfg.WorkerID) == 0 || len(cfg.WorkerID) > 128 || strings.TrimSpace(cfg.WorkerID) != cfg.WorkerID || !validRelayWorkerID(cfg.WorkerID) {
		return ResearchNotificationRelay{}, fmt.Errorf("RESEARCH_NOTIFICATION_RELAY_WORKER_ID is invalid")
	}
	if cfg.BatchSize < 1 || cfg.BatchSize > 100 {
		return ResearchNotificationRelay{}, fmt.Errorf("RESEARCH_NOTIFICATION_RELAY_BATCH_SIZE must be between 1 and 100")
	}
	if cfg.PollInterval < 10*time.Millisecond || cfg.PollInterval > time.Minute {
		return ResearchNotificationRelay{}, fmt.Errorf("RESEARCH_NOTIFICATION_RELAY_POLL_INTERVAL must be between 10ms and 1m")
	}
	if cfg.LeaseDuration < time.Second || cfg.LeaseDuration > 5*time.Minute {
		return ResearchNotificationRelay{}, fmt.Errorf("RESEARCH_NOTIFICATION_RELAY_LEASE_DURATION must be between 1s and 5m")
	}
	if cfg.RetryDelay <= 0 || cfg.RetryDelay > 24*time.Hour {
		return ResearchNotificationRelay{}, fmt.Errorf("RESEARCH_NOTIFICATION_RELAY_RETRY_DELAY must be positive and no greater than 24h")
	}
	if err := cfg.EventLimits.Validate(); err != nil {
		return ResearchNotificationRelay{}, fmt.Errorf("Notification Event resource limits are invalid: %w", err)
	}
	if cfg.EventLimits.MaxEnvelopeBytes > 64<<20 {
		return ResearchNotificationRelay{}, fmt.Errorf("NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES must not exceed 64 MiB")
	}
	if cfg.EventLimits.MaxJSONDepth > 128 {
		return ResearchNotificationRelay{}, fmt.Errorf("NOTIFICATION_EVENT_MAX_JSON_DEPTH must not exceed 128")
	}
	return cfg, nil
}

func requiredPositiveInt(env platformconfig.Env, key string, maximum int) (int, error) {
	raw, err := env.RequiredString(key)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > maximum {
		return 0, fmt.Errorf("configuration %s must be an integer between 1 and %d", key, maximum)
	}
	return value, nil
}

func validRelayWorkerID(value string) bool {
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}
