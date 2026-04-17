CREATE TABLE IF NOT EXISTS accounts (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  username VARCHAR(64) NOT NULL,
  email VARCHAR(255),
  display_name VARCHAR(255),
  password_hash VARCHAR(255) NOT NULL,
  role_id VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  version INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT uq_accounts_tenant_username UNIQUE (tenant_id, username),
  CONSTRAINT uq_accounts_tenant_email UNIQUE (tenant_id, email)
);

CREATE INDEX IF NOT EXISTS idx_accounts_login_lookup ON accounts (username, email);
CREATE INDEX IF NOT EXISTS idx_accounts_tenant_role ON accounts (tenant_id, role_id);
