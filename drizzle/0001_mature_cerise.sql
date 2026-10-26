CREATE TABLE `studio_projects` (
	`id` text PRIMARY KEY NOT NULL,
	`user_id` text NOT NULL,
	`title` text NOT NULL,
	`version` integer DEFAULT 0 NOT NULL,
	`revision` integer DEFAULT 0 NOT NULL,
	`active_run` text DEFAULT '' NOT NULL,
	`state` text DEFAULT '{}' NOT NULL,
	`state_revision` integer DEFAULT 0 NOT NULL,
	`created_at` text NOT NULL,
	`updated_at` text NOT NULL,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE INDEX `studio_projects_owner` ON `studio_projects` (`user_id`,`updated_at`);--> statement-breakpoint
CREATE TABLE `studio_runs` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`request_key` text NOT NULL,
	`request_hash` text NOT NULL,
	`prompt` text NOT NULL,
	`base_revision` integer NOT NULL,
	`base_version` integer NOT NULL,
	`status` text NOT NULL,
	`message` text DEFAULT '' NOT NULL,
	`provider` text DEFAULT '' NOT NULL,
	`model` text DEFAULT '' NOT NULL,
	`result_version` integer DEFAULT 0 NOT NULL,
	`deadline` integer NOT NULL,
	`created_at` text NOT NULL,
	`updated_at` text NOT NULL,
	FOREIGN KEY (`project_id`) REFERENCES `studio_projects`(`id`) ON UPDATE no action ON DELETE no action,
	CONSTRAINT "studio_runs_status" CHECK("studio_runs"."status" IN ('queued','running','succeeded','failed'))
);
--> statement-breakpoint
CREATE UNIQUE INDEX `studio_runs_request` ON `studio_runs` (`project_id`,`request_key`);--> statement-breakpoint
CREATE INDEX `studio_runs_project` ON `studio_runs` (`project_id`,`created_at`);--> statement-breakpoint
CREATE INDEX `studio_runs_deadline` ON `studio_runs` (`status`,`deadline`);--> statement-breakpoint
CREATE TABLE `studio_versions` (
	`project_id` text NOT NULL,
	`version` integer NOT NULL,
	`html` text NOT NULL,
	`run_id` text NOT NULL,
	`created_at` text NOT NULL,
	PRIMARY KEY(`project_id`, `version`),
	FOREIGN KEY (`project_id`) REFERENCES `studio_projects`(`id`) ON UPDATE no action ON DELETE no action,
	FOREIGN KEY (`run_id`) REFERENCES `studio_runs`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE UNIQUE INDEX `studio_versions_run` ON `studio_versions` (`run_id`);