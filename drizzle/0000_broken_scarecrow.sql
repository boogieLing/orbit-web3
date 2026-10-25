CREATE TABLE `actions` (
	`user_id` text NOT NULL,
	`post_id` text NOT NULL,
	`kind` text NOT NULL,
	`created_at` text NOT NULL,
	PRIMARY KEY(`user_id`, `post_id`, `kind`),
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE INDEX `actions_post` ON `actions` (`post_id`,`kind`);--> statement-breakpoint
CREATE TABLE `challenges` (
	`id` text PRIMARY KEY NOT NULL,
	`session_token` text NOT NULL,
	`address` text NOT NULL,
	`message` text NOT NULL,
	`expires` integer NOT NULL,
	`consumed` integer DEFAULT 0 NOT NULL
);
--> statement-breakpoint
CREATE INDEX `challenges_expiry` ON `challenges` (`expires`);--> statement-breakpoint
CREATE TABLE `collection_versions` (
	`collection_id` text NOT NULL,
	`version` integer NOT NULL,
	`data` text NOT NULL,
	`created_at` text NOT NULL,
	PRIMARY KEY(`collection_id`, `version`),
	FOREIGN KEY (`collection_id`) REFERENCES `collections`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE TABLE `collections` (
	`id` text PRIMARY KEY NOT NULL,
	`user_id` text NOT NULL,
	`title` text NOT NULL,
	`description` text NOT NULL,
	`instructions` text NOT NULL,
	`layout` text NOT NULL,
	`theme` text NOT NULL,
	`app_ids` text NOT NULL,
	`post_ids` text NOT NULL,
	`version` integer NOT NULL,
	`html` text NOT NULL,
	`created_at` text NOT NULL,
	`updated_at` text NOT NULL,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE INDEX `collections_owner` ON `collections` (`user_id`,`updated_at`);--> statement-breakpoint
CREATE TABLE `events` (
	`user_id` text NOT NULL,
	`event_id` text NOT NULL,
	`post_id` text NOT NULL,
	`kind` text NOT NULL,
	`created_at` text NOT NULL,
	PRIMARY KEY(`user_id`, `event_id`)
);
--> statement-breakpoint
CREATE INDEX `events_expiry` ON `events` (`created_at`);--> statement-breakpoint
CREATE TABLE `feed_snapshots` (
	`id` text PRIMARY KEY NOT NULL,
	`user_id` text NOT NULL,
	`filter_hash` text NOT NULL,
	`items` text NOT NULL,
	`expires` integer NOT NULL
);
--> statement-breakpoint
CREATE INDEX `feed_expiry` ON `feed_snapshots` (`expires`);--> statement-breakpoint
CREATE TABLE `posts` (
	`id` text PRIMARY KEY NOT NULL,
	`user_id` text NOT NULL,
	`app_id` text NOT NULL,
	`title` text NOT NULL,
	`body` text NOT NULL,
	`sentiment` text NOT NULL,
	`tags` text NOT NULL,
	`tx_hash` text DEFAULT '' NOT NULL,
	`proof_status` text DEFAULT 'none' NOT NULL,
	`proof_message` text DEFAULT '' NOT NULL,
	`proof_checked_at` text DEFAULT '' NOT NULL,
	`proof_data` text DEFAULT '{}' NOT NULL,
	`revision` integer DEFAULT 1 NOT NULL,
	`deleted` integer DEFAULT 0 NOT NULL,
	`created_at` text NOT NULL,
	`updated_at` text NOT NULL,
	`idempotency_key` text NOT NULL,
	`content_hash` text NOT NULL,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE INDEX `posts_feed` ON `posts` (`deleted`,`created_at`);--> statement-breakpoint
CREATE INDEX `posts_author` ON `posts` (`user_id`,`deleted`,`created_at`);--> statement-breakpoint
CREATE INDEX `posts_app` ON `posts` (`app_id`,`deleted`,`created_at`);--> statement-breakpoint
CREATE UNIQUE INDEX `posts_idempotency` ON `posts` (`user_id`,`idempotency_key`);--> statement-breakpoint
CREATE TABLE `rates` (
	`key` text PRIMARY KEY NOT NULL,
	`count` integer NOT NULL,
	`expires` integer NOT NULL
);
--> statement-breakpoint
CREATE INDEX `rates_expiry` ON `rates` (`expires`);--> statement-breakpoint
CREATE TABLE `sessions` (
	`token` text PRIMARY KEY NOT NULL,
	`user_id` text NOT NULL,
	`platform_id` text DEFAULT '' NOT NULL,
	`expires` integer NOT NULL,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE INDEX `sessions_expiry` ON `sessions` (`expires`);--> statement-breakpoint
CREATE TABLE `users` (
	`id` text PRIMARY KEY NOT NULL,
	`wallet` text,
	`display_name` text NOT NULL,
	`preferences` text DEFAULT '[]' NOT NULL,
	`created_at` text NOT NULL
);
--> statement-breakpoint
CREATE UNIQUE INDEX `users_wallet` ON `users` (`wallet`);