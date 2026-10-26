import { sql } from 'drizzle-orm';
import { sqliteTable, text, integer, index, uniqueIndex, primaryKey, check } from 'drizzle-orm/sqlite-core';

export const users = sqliteTable('users', {
  id: text('id').primaryKey(), wallet: text('wallet'), displayName: text('display_name').notNull(),
  preferences: text('preferences').notNull().default('[]'), createdAt: text('created_at').notNull(),
}, t => [uniqueIndex('users_wallet').on(t.wallet)]);
export const sessions = sqliteTable('sessions', {
  token: text('token').primaryKey(), userID: text('user_id').notNull().references(() => users.id),
  platformID: text('platform_id').notNull().default(''), expires: integer('expires').notNull(),
}, t => [index('sessions_expiry').on(t.expires)]);
export const challenges = sqliteTable('challenges', {
  id: text('id').primaryKey(), sessionToken: text('session_token').notNull(), address: text('address').notNull(),
  message: text('message').notNull(), expires: integer('expires').notNull(), consumed: integer('consumed').notNull().default(0),
}, t => [index('challenges_expiry').on(t.expires)]);
export const posts = sqliteTable('posts', {
  id: text('id').primaryKey(), userID: text('user_id').notNull().references(() => users.id),
  appID: text('app_id').notNull(), title: text('title').notNull(), body: text('body').notNull(),
  sentiment: text('sentiment').notNull(), tags: text('tags').notNull(), txHash: text('tx_hash').notNull().default(''),
  proofStatus: text('proof_status').notNull().default('none'), proofMessage: text('proof_message').notNull().default(''),
  proofCheckedAt: text('proof_checked_at').notNull().default(''), proofData: text('proof_data').notNull().default('{}'),
  revision: integer('revision').notNull().default(1), deleted: integer('deleted').notNull().default(0),
  createdAt: text('created_at').notNull(), updatedAt: text('updated_at').notNull(),
  idempotencyKey: text('idempotency_key').notNull(), contentHash: text('content_hash').notNull(),
}, t => [index('posts_feed').on(t.deleted,t.createdAt), index('posts_author').on(t.userID,t.deleted,t.createdAt), index('posts_app').on(t.appID,t.deleted,t.createdAt), uniqueIndex('posts_idempotency').on(t.userID,t.idempotencyKey)]);
export const actions = sqliteTable('actions', {
  userID: text('user_id').notNull().references(() => users.id), postID: text('post_id').notNull(),
  kind: text('kind').notNull(), createdAt: text('created_at').notNull(),
}, t => [primaryKey({columns:[t.userID,t.postID,t.kind]}), index('actions_post').on(t.postID,t.kind)]);
export const events = sqliteTable('events', {
  userID: text('user_id').notNull(), eventID: text('event_id').notNull(), postID: text('post_id').notNull(),
  kind: text('kind').notNull(), createdAt: text('created_at').notNull(),
}, t => [primaryKey({columns:[t.userID,t.eventID]}),index('events_expiry').on(t.createdAt)]);
export const rates = sqliteTable('rates', {
  key: text('key').primaryKey(), count: integer('count').notNull(), expires: integer('expires').notNull(),
}, t => [index('rates_expiry').on(t.expires)]);
export const feedSnapshots = sqliteTable('feed_snapshots', {
  id: text('id').primaryKey(), userID: text('user_id').notNull(), filterHash: text('filter_hash').notNull(),
  items: text('items').notNull(), expires: integer('expires').notNull(),
}, t => [index('feed_expiry').on(t.expires)]);
export const collections = sqliteTable('collections', {
  id: text('id').primaryKey(), userID: text('user_id').notNull().references(() => users.id),
  title: text('title').notNull(), description: text('description').notNull(), instructions: text('instructions').notNull(),
  layout: text('layout').notNull(), theme: text('theme').notNull(), appIDs: text('app_ids').notNull(), postIDs: text('post_ids').notNull(),
  version: integer('version').notNull(), html: text('html').notNull(), createdAt: text('created_at').notNull(), updatedAt: text('updated_at').notNull(),
}, t => [index('collections_owner').on(t.userID,t.updatedAt)]);
export const collectionVersions = sqliteTable('collection_versions', {
  collectionID: text('collection_id').notNull().references(() => collections.id), version: integer('version').notNull(),
  data: text('data').notNull(), createdAt: text('created_at').notNull(),
}, t => [primaryKey({columns:[t.collectionID,t.version]})]);

// 工作台项目与运行状态独立于社区内容，版本只追加，业务数据单独保留。
export const studioProjects = sqliteTable('studio_projects', {
  id: text('id').primaryKey(), userID: text('user_id').notNull().references(() => users.id), title: text('title').notNull(),
  version: integer('version').notNull().default(0), revision: integer('revision').notNull().default(0),
  activeRun: text('active_run').notNull().default(''), state: text('state').notNull().default('{}'), stateRevision: integer('state_revision').notNull().default(0),
  createdAt: text('created_at').notNull(), updatedAt: text('updated_at').notNull(),
}, t => [index('studio_projects_owner').on(t.userID,t.updatedAt)]);
export const studioRuns = sqliteTable('studio_runs', {
  id: text('id').primaryKey(), projectID: text('project_id').notNull().references(() => studioProjects.id),
  requestKey: text('request_key').notNull(), requestHash: text('request_hash').notNull(), prompt: text('prompt').notNull(),
  baseRevision: integer('base_revision').notNull(), baseVersion: integer('base_version').notNull(), status: text('status').notNull(),
  message: text('message').notNull().default(''), provider: text('provider').notNull().default(''), model: text('model').notNull().default(''),
  resultVersion: integer('result_version').notNull().default(0), deadline: integer('deadline').notNull(),
  createdAt: text('created_at').notNull(), updatedAt: text('updated_at').notNull(),
}, t => [uniqueIndex('studio_runs_request').on(t.projectID,t.requestKey), index('studio_runs_project').on(t.projectID,t.createdAt), index('studio_runs_deadline').on(t.status,t.deadline), check('studio_runs_status',sql`${t.status} IN ('queued','running','succeeded','failed')`)]);
export const studioVersions = sqliteTable('studio_versions', {
  projectID: text('project_id').notNull().references(() => studioProjects.id), version: integer('version').notNull(), html: text('html').notNull(),
  runID: text('run_id').notNull().references(() => studioRuns.id), createdAt: text('created_at').notNull(),
}, t => [primaryKey({columns:[t.projectID,t.version]}),uniqueIndex('studio_versions_run').on(t.runID)]);
