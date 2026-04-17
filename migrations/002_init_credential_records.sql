CREATE TABLE IF NOT EXISTS credential_records (
  id VARCHAR(64) NOT NULL,
  tenant_id VARCHAR(64) NOT NULL,
  owner_account_id VARCHAR(64) NOT NULL,
  title VARCHAR(120) NOT NULL,
  username VARCHAR(120) NOT NULL,
  website VARCHAR(255) NULL,
  category VARCHAR(64) NULL,
  notes TEXT NULL,
  password_envelope TEXT NOT NULL,
  created_by VARCHAR(64) NULL,
  updated_by VARCHAR(64) NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_credential_records_owner_updated_at ON credential_records (tenant_id, owner_account_id, updated_at);
CREATE INDEX idx_credential_records_owner_title ON credential_records (tenant_id, owner_account_id, title);
