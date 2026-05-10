-- +goose Up
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    title TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    ocr_settings JSONB NOT NULL DEFAULT '{}',
    page_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_documents_user_id ON documents(user_id);

CREATE TABLE pages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    page_number INT NOT NULL,
    raw_markdown TEXT
);
CREATE INDEX idx_pages_document_id ON pages(document_id);

CREATE TABLE blocks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    page_id UUID NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    data JSONB NOT NULL,
    "order" INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_blocks_page_id ON blocks(page_id);

-- +goose Down
DROP TABLE blocks;
DROP TABLE pages;
DROP TABLE documents;
