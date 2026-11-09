package storage

import platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

// Migrations 返回 Notification 自有持久化结构。
// Inbox/Fanout 表暂不在此版本创建：FanoutWorkItem 与分页订阅查询契约尚未冻结，
// 不在存储层先行猜测队列状态、租约字段或游标语义。
func Migrations() []platformstorage.Migration {
	return []platformstorage.Migration{
		{
			Version:     1,
			Description: "create notification subscription delivery plan and delivery execution tables",
			Statements: []string{
				`CREATE TABLE notification_subscriptions (
					subscription_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					version BIGINT UNSIGNED NOT NULL,
					owner_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					event_type VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					filter_schema_version VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
					filters JSON NULL,
					recipient_scope VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					recipient_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
					channel_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					template_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					created_at DATETIME(6) NOT NULL,
					updated_at DATETIME(6) NOT NULL,
					effective_at DATETIME(6) NOT NULL,
					PRIMARY KEY (subscription_id, version),
					KEY idx_notification_subscriptions_event_status (event_type, status, subscription_id, version)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE TABLE notification_delivery_plans (
					delivery_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					source_module VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					event_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					subscription_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					subscription_version BIGINT UNSIGNED NOT NULL,
					channel_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					channel_config_version BIGINT UNSIGNED NOT NULL,
					template_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					template_version BIGINT UNSIGNED NOT NULL,
					recipient_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					payload_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					authorization_policy_versions JSON NOT NULL,
					authorization_refs JSON NOT NULL,
					plan_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					delivery_idempotency_key VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					plan_json JSON NOT NULL,
					created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
					PRIMARY KEY (delivery_id),
					UNIQUE KEY uk_notification_delivery_plan_identity (
						source_module, event_id, subscription_id, subscription_version,
						channel_ref, channel_config_version, template_id, template_version, recipient_ref
					),
					KEY idx_notification_delivery_plans_event (source_module, event_id),
					KEY idx_notification_delivery_plans_fingerprint (plan_fingerprint)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE TABLE notification_deliveries (
					delivery_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					source_module VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					event_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					subscription_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					subscription_version BIGINT UNSIGNED NOT NULL,
					channel_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					channel_config_version BIGINT UNSIGNED NOT NULL,
					template_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					template_version BIGINT UNSIGNED NOT NULL,
					recipient_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					payload_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					plan_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					delivery_idempotency_key VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					version BIGINT UNSIGNED NOT NULL,
					attempt_count INT UNSIGNED NOT NULL DEFAULT 0,
					max_attempts INT UNSIGNED NOT NULL,
					next_attempt_at DATETIME(6) NULL,
					deadline_at DATETIME(6) NOT NULL,
					reason_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
					last_error_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
					provider_message_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NULL,
					created_at DATETIME(6) NOT NULL,
					updated_at DATETIME(6) NOT NULL,
					PRIMARY KEY (delivery_id),
					UNIQUE KEY uk_notification_delivery_identity (
						source_module, event_id, subscription_id, subscription_version,
						channel_ref, channel_config_version, template_id, template_version, recipient_ref
					),
					UNIQUE KEY uk_notification_delivery_idempotency (channel_ref, recipient_ref, delivery_idempotency_key),
					KEY idx_notification_deliveries_due (status, next_attempt_at, deadline_at, created_at),
					KEY idx_notification_deliveries_subscription (subscription_id, created_at),
					CONSTRAINT fk_notification_delivery_plan FOREIGN KEY (delivery_id) REFERENCES notification_delivery_plans (delivery_id)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE TABLE notification_delivery_attempts (
					attempt_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					delivery_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					attempt_no INT UNSIGNED NOT NULL,
					worker_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					fencing_token BIGINT UNSIGNED NOT NULL,
					lease_expires_at DATETIME(6) NOT NULL,
					started_at DATETIME(6) NOT NULL,
					finished_at DATETIME(6) NULL,
					outcome VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NULL,
					provider_message_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NULL,
					error_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
					retryable TINYINT(1) NOT NULL DEFAULT 0,
					retry_after_at DATETIME(6) NULL,
					duration_ms BIGINT NULL,
					evidence_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NULL,
					PRIMARY KEY (attempt_id),
					UNIQUE KEY uk_notification_attempt_number (delivery_id, attempt_no),
					UNIQUE KEY uk_notification_attempt_fencing_token (delivery_id, fencing_token),
					KEY idx_notification_attempts_started (delivery_id, started_at),
					CONSTRAINT fk_notification_attempt_delivery FOREIGN KEY (delivery_id) REFERENCES notification_deliveries (delivery_id)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE TABLE notification_delivery_state_events (
					delivery_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					version BIGINT UNSIGNED NOT NULL,
					from_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					to_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					changed_at DATETIME(6) NOT NULL,
					reason_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
					evidence_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NULL,
					actor_ref VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NULL,
					request_id VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NULL,
					actor_reason VARCHAR(512) NULL,
					PRIMARY KEY (delivery_id, version),
					KEY idx_notification_delivery_events_time (delivery_id, changed_at),
					CONSTRAINT fk_notification_delivery_event FOREIGN KEY (delivery_id) REFERENCES notification_deliveries (delivery_id)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},
	}
}
