CREATE TABLE IF NOT EXISTS accounts (
  id VARCHAR(64) NOT NULL,
  tenant_id VARCHAR(64) NOT NULL,
  username VARCHAR(64) NOT NULL,
  email VARCHAR(255) NULL,
  display_name VARCHAR(255) NULL,
  password_hash VARCHAR(255) NOT NULL,
  role_id VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  version INT NOT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  CONSTRAINT uq_accounts_tenant_username UNIQUE (tenant_id, username),
  CONSTRAINT uq_accounts_tenant_email UNIQUE (tenant_id, email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_accounts_login_lookup ON accounts (username, email);
CREATE INDEX idx_accounts_tenant_role ON accounts (tenant_id, role_id);
