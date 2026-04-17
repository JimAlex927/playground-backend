ALTER TABLE credential_records
  ADD COLUMN owner_account_id VARCHAR(64) NULL AFTER tenant_id;

UPDATE credential_records records
INNER JOIN accounts users
  ON users.tenant_id = records.tenant_id
  AND LOWER(users.username) = LOWER(records.created_by)
SET records.owner_account_id = users.id
WHERE records.owner_account_id IS NULL OR records.owner_account_id = '';

CREATE INDEX idx_credential_records_owner_updated_at ON credential_records (tenant_id, owner_account_id, updated_at);
CREATE INDEX idx_credential_records_owner_title ON credential_records (tenant_id, owner_account_id, title);
