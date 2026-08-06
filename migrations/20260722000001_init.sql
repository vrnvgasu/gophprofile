-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS avatars
(
    id                UUID PRIMARY KEY   DEFAULT gen_random_uuid(),
    user_id           VARCHAR(255) NOT NULL,
    file_name         VARCHAR(255) NOT NULL,
    mime_type         VARCHAR(100) NOT NULL,
    size_bytes        BIGINT       NOT NULL,
    s3_key            VARCHAR(500) NOT NULL,
    thumbnail_s3_keys JSONB        NOT NULL DEFAULT '{}'::jsonb,
    width             INTEGER      NOT NULL DEFAULT 0,
    height            INTEGER      NOT NULL DEFAULT 0,
    upload_status     VARCHAR(50)  NOT NULL DEFAULT 'uploading',
    processing_status VARCHAR(50)  NOT NULL DEFAULT 'pending',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_avatars_user_id ON avatars (user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_avatars_status ON avatars (upload_status, processing_status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_avatars_status;
DROP INDEX IF EXISTS idx_avatars_user_id;
DROP TABLE IF EXISTS avatars;
-- +goose StatementEnd
