-- Social post refs are idempotency keys. Historical duplicates can each hold
-- part of the publication record, so merge their useful fields into the most
-- authoritative row before enforcing uniqueness.
WITH source AS (
  SELECT posts.*,
    CASE
      WHEN lower(btrim(status)) IN ('posted', 'uploaded', 'published', 'succeeded', 'success') THEN 6
      WHEN posted_at IS NOT NULL THEN 5
      WHEN lower(btrim(status)) IN ('queued', 'scheduled') THEN 4
      WHEN url <> '' THEN 3
      WHEN status = '' THEN 2
      ELSE 1
    END AS publication_rank,
    CASE
      WHEN lower(btrim(status)) IN ('posted', 'uploaded', 'published', 'succeeded', 'success') THEN 4
      WHEN lower(btrim(status)) IN ('queued', 'scheduled') THEN 3
      WHEN status = '' THEN 2
      ELSE 1
    END AS status_rank
  FROM social_posts posts
), merged AS (
  SELECT id, ref,
    count(*) OVER (PARTITION BY ref) AS duplicate_count,
    row_number() OVER (
      PARTITION BY ref
      ORDER BY publication_rank DESC, updated_at DESC, created_at DESC, id DESC
    ) AS position,
    first_value(nullif(text, '')) OVER (
      PARTITION BY ref
      ORDER BY (text <> '') DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_text,
    first_value(nullif(posted_to, '')) OVER (
      PARTITION BY ref
      ORDER BY (posted_to <> '') DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_posted_to,
    first_value(nullif(kind, '')) OVER (
      PARTITION BY ref
      ORDER BY (kind <> '') DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_kind,
    first_value(nullif(status, '')) OVER (
      PARTITION BY ref
      ORDER BY status_rank DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_status,
    first_value(recording_id) OVER (
      PARTITION BY ref
      ORDER BY (recording_id IS NOT NULL) DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_recording_id,
    first_value(conf_talk_id) OVER (
      PARTITION BY ref
      ORDER BY (conf_talk_id IS NOT NULL) DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_conf_talk_id,
    first_value(nullif(url, '')) OVER (
      PARTITION BY ref
      ORDER BY (url <> '') DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_url,
    first_value(nullif(reply_url, '')) OVER (
      PARTITION BY ref
      ORDER BY (reply_url <> '') DESC, publication_rank DESC, updated_at DESC, id DESC
    ) AS merged_reply_url,
    first_value(error) OVER (
      PARTITION BY ref
      ORDER BY publication_rank DESC, updated_at DESC, created_at DESC, id DESC
    ) AS merged_error,
    first_value(error_fingerprint) OVER (
      PARTITION BY ref
      ORDER BY publication_rank DESC, updated_at DESC, created_at DESC, id DESC
    ) AS merged_error_fingerprint,
    bool_or(publication_rank >= 5) OVER (PARTITION BY ref) AS has_success,
    max(scheduled_at) OVER (PARTITION BY ref) AS merged_scheduled_at,
    max(posted_at) OVER (PARTITION BY ref) AS merged_posted_at,
    max(notified_at) OVER (PARTITION BY ref) AS merged_notified_at,
    min(created_at) OVER (PARTITION BY ref) AS merged_created_at
  FROM source
), survivors AS (
  UPDATE social_posts posts
  SET text = coalesce(merged.merged_text, posts.text),
    posted_to = coalesce(merged.merged_posted_to, posts.posted_to),
    kind = coalesce(merged.merged_kind, posts.kind),
    status = coalesce(merged.merged_status, posts.status),
    recording_id = coalesce(merged.merged_recording_id, posts.recording_id),
    conf_talk_id = coalesce(merged.merged_conf_talk_id, posts.conf_talk_id),
    url = coalesce(merged.merged_url, posts.url),
    reply_url = coalesce(merged.merged_reply_url, posts.reply_url),
    error = CASE WHEN merged.has_success THEN '' ELSE merged.merged_error END,
    error_fingerprint = CASE WHEN merged.has_success THEN '' ELSE merged.merged_error_fingerprint END,
    scheduled_at = coalesce(merged.merged_scheduled_at, posts.scheduled_at),
    posted_at = coalesce(merged.merged_posted_at, posts.posted_at),
    notified_at = coalesce(merged.merged_notified_at, posts.notified_at),
    created_at = merged.merged_created_at
  FROM merged
  WHERE merged.position = 1
    AND merged.duplicate_count > 1
    AND posts.id = merged.id
  RETURNING posts.id, posts.ref
)
DELETE FROM social_posts posts
USING survivors
WHERE posts.ref = survivors.ref
  AND posts.id <> survivors.id;

DROP INDEX IF EXISTS social_posts_ref_idx;
CREATE UNIQUE INDEX social_posts_ref_unique_idx ON social_posts (ref);

ALTER TABLE social_posts
  ADD COLUMN publication_claim_token uuid,
  ADD COLUMN publication_claim_expires_at timestamptz;

ALTER TABLE social_posts
  ADD CONSTRAINT social_posts_publication_claim_pair_check
  CHECK (
    (publication_claim_token IS NULL) =
    (publication_claim_expires_at IS NULL)
  );
