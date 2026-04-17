package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	mysqlDriver "github.com/go-sql-driver/mysql"

	"playground/internal/config"
)

func Open(cfg config.DatabaseConfig) (*sql.DB, error) {
	if strings.TrimSpace(cfg.Driver) != "mysql" {
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}

	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func AutoMigrate(ctx context.Context, db *sql.DB) error {
	queries := []string{
		`
		CREATE TABLE IF NOT EXISTS roles (
			id VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL,
			name VARCHAR(64) NOT NULL,
			description VARCHAR(255) NULL,
			created_at DATETIME(6) NOT NULL,
			updated_at DATETIME(6) NOT NULL,
			PRIMARY KEY (id),
			CONSTRAINT uq_roles_tenant_name UNIQUE (tenant_id, name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
		`CREATE INDEX idx_roles_tenant_name ON roles (tenant_id, name)`,
		`
		CREATE TABLE IF NOT EXISTS permissions (
			id VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL,
			code VARCHAR(64) NOT NULL,
			display_name VARCHAR(64) NOT NULL,
			description VARCHAR(255) NOT NULL,
			is_system TINYINT(1) NOT NULL DEFAULT 0,
			created_at DATETIME(6) NOT NULL,
			updated_at DATETIME(6) NOT NULL,
			PRIMARY KEY (id),
			CONSTRAINT uq_permissions_tenant_code UNIQUE (tenant_id, code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
		`
		CREATE TABLE IF NOT EXISTS role_permissions (
			role_id VARCHAR(64) NOT NULL,
			permission_code VARCHAR(64) NOT NULL,
			PRIMARY KEY (role_id, permission_code),
			CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
		`CREATE INDEX idx_permissions_tenant_code ON permissions (tenant_id, code)`,
		`CREATE UNIQUE INDEX idx_permissions_id ON permissions (id)`,
		`CREATE INDEX idx_role_permissions_permission ON role_permissions (permission_code)`,
		`
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
		`CREATE INDEX idx_accounts_login_lookup ON accounts (username, email)`,
		`CREATE INDEX idx_accounts_tenant_role ON accounts (tenant_id, role_id)`,
		`
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
		`CREATE INDEX idx_credential_records_owner_updated_at ON credential_records (tenant_id, owner_account_id, updated_at)`,
		`CREATE INDEX idx_credential_records_owner_title ON credential_records (tenant_id, owner_account_id, title)`,
		`
		CREATE TABLE IF NOT EXISTS uploads (
			id VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			owner_account_id VARCHAR(64) NOT NULL,
			filename VARCHAR(512) NOT NULL DEFAULT '',
			mime_type VARCHAR(128) NOT NULL DEFAULT '',
			size BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'uploading',
			storage_path VARCHAR(1024) NOT NULL DEFAULT '',
			created_at DATETIME(6) NOT NULL,
			completed_at DATETIME(6) NULL,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
		`CREATE INDEX idx_uploads_owner ON uploads (tenant_id, owner_account_id)`,
		`CREATE INDEX idx_uploads_status ON uploads (status)`,
	}

	for _, query := range queries {
		if _, err := db.ExecContext(ctx, query); err != nil && !isMySQLIgnorable(err, 1061) {
			return fmt.Errorf("auto migrate database: %w", err)
		}
	}

	if err := addColumnIfMissing(ctx, db, "accounts", "role_id", `ALTER TABLE accounts ADD COLUMN role_id VARCHAR(64) NULL AFTER password_hash`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, db, "credential_records", "owner_account_id", `ALTER TABLE credential_records ADD COLUMN owner_account_id VARCHAR(64) NULL AFTER tenant_id`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, db, "permissions", "id", `ALTER TABLE permissions ADD COLUMN id VARCHAR(64) NULL FIRST`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, db, "permissions", "tenant_id", `ALTER TABLE permissions ADD COLUMN tenant_id VARCHAR(64) NOT NULL DEFAULT 'default' AFTER id`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, db, "permissions", "is_system", `ALTER TABLE permissions ADD COLUMN is_system TINYINT(1) NOT NULL DEFAULT 0 AFTER description`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, db, "permissions", "created_at", `ALTER TABLE permissions ADD COLUMN created_at DATETIME(6) NOT NULL DEFAULT UTC_TIMESTAMP(6) AFTER is_system`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, db, "permissions", "updated_at", `ALTER TABLE permissions ADD COLUMN updated_at DATETIME(6) NOT NULL DEFAULT UTC_TIMESTAMP(6) AFTER created_at`); err != nil {
		return err
	}

	if err := backfillLegacyRolePermissions(ctx, db); err != nil {
		return err
	}
	if err := backfillLegacyPermissions(ctx, db); err != nil {
		return err
	}
	if err := backfillLegacyAccountRoles(ctx, db); err != nil {
		return err
	}
	if err := backfillCredentialOwners(ctx, db); err != nil {
		return err
	}

	if err := dropColumnIfExists(ctx, db, "roles", "permissions_json"); err != nil {
		return err
	}
	if err := dropColumnIfExists(ctx, db, "accounts", "roles_json"); err != nil {
		return err
	}
	if err := dropColumnIfExists(ctx, db, "accounts", "permissions_json"); err != nil {
		return err
	}
	if err := dropTableIfExists(ctx, db, "tenant_permissions"); err != nil {
		return err
	}
	if err := dropTableIfExists(ctx, db, "role_permission_bindings"); err != nil {
		return err
	}

	return nil
}

func addColumnIfMissing(ctx context.Context, db *sql.DB, tableName, columnName, query string) error {
	exists, err := columnExists(ctx, db, tableName, columnName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, query); err != nil && !isMySQLIgnorable(err, 1060) {
		return fmt.Errorf("add %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func dropColumnIfExists(ctx context.Context, db *sql.DB, tableName, columnName string) error {
	exists, err := columnExists(ctx, db, tableName, columnName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, tableName, columnName)); err != nil && !isMySQLIgnorable(err, 1091) {
		return fmt.Errorf("drop %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func backfillLegacyRolePermissions(ctx context.Context, db *sql.DB) error {
	var rows *sql.Rows

	exists, err := tableExists(ctx, db, "role_permission_bindings")
	if err == nil && exists {
		rows, queryErr := db.QueryContext(ctx, `SELECT role_id, permission_code FROM role_permission_bindings`)
		if queryErr != nil {
			return fmt.Errorf("query legacy role permission bindings: %w", queryErr)
		}
		defer rows.Close()

		for rows.Next() {
			var roleID string
			var permissionCode string
			if err := rows.Scan(&roleID, &permissionCode); err != nil {
				return fmt.Errorf("scan legacy role permission binding: %w", err)
			}
			if _, err := db.ExecContext(ctx, `
				INSERT IGNORE INTO role_permissions (role_id, permission_code)
				VALUES (?, ?)
			`, strings.TrimSpace(roleID), strings.ToLower(strings.TrimSpace(permissionCode))); err != nil {
				return fmt.Errorf("insert migrated role permission: %w", err)
			}
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate legacy role permission bindings: %w", err)
		}
	}

	exists, err = columnExists(ctx, db, "roles", "permissions_json")
	if err != nil || !exists {
		return err
	}

	rows, err = db.QueryContext(ctx, `SELECT id, permissions_json FROM roles`)
	if err != nil {
		return fmt.Errorf("query legacy role permissions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			roleID          string
			permissionsJSON []byte
		)
		if err := rows.Scan(&roleID, &permissionsJSON); err != nil {
			return fmt.Errorf("scan legacy role permissions: %w", err)
		}

		permissions, err := decodeJSONStringSlice(permissionsJSON)
		if err != nil {
			return fmt.Errorf("decode legacy role permissions: %w", err)
		}

		for _, permission := range uniqueStrings(permissions) {
			if _, err := db.ExecContext(ctx, `
				INSERT IGNORE INTO role_permissions (role_id, permission_code)
				VALUES (?, ?)
			`, strings.TrimSpace(roleID), permission); err != nil {
				return fmt.Errorf("insert legacy role permission: %w", err)
			}
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy role permissions: %w", err)
	}
	return nil
}

func backfillLegacyPermissions(ctx context.Context, db *sql.DB) error {
	tenants, err := loadKnownTenants(ctx, db)
	if err != nil {
		return err
	}
	if len(tenants) == 0 {
		tenants = []string{"default"}
	}

	type legacyPermission struct {
		Code        string
		DisplayName string
		Description string
		System      bool
	}
	legacyItems := make([]legacyPermission, 0)

	if exists, err := tableExists(ctx, db, "tenant_permissions"); err != nil {
		return err
	} else if exists {
		rows, queryErr := db.QueryContext(ctx, `
			SELECT code, display_name, description, is_system
			FROM tenant_permissions
		`)
		if queryErr != nil {
			return fmt.Errorf("query legacy tenant permissions: %w", queryErr)
		}
		defer rows.Close()

		for rows.Next() {
			var item legacyPermission
			if err := rows.Scan(&item.Code, &item.DisplayName, &item.Description, &item.System); err != nil {
				return fmt.Errorf("scan legacy tenant permission: %w", err)
			}
			legacyItems = append(legacyItems, item)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate legacy tenant permissions: %w", err)
		}
	} else {
		rows, queryErr := db.QueryContext(ctx, `
			SELECT code, display_name, description, is_system
			FROM permissions
			WHERE tenant_id IS NOT NULL AND tenant_id <> ''
		`)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var item legacyPermission
				if err := rows.Scan(&item.Code, &item.DisplayName, &item.Description, &item.System); err != nil {
					return fmt.Errorf("scan scoped permission: %w", err)
				}
				legacyItems = append(legacyItems, item)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterate scoped permissions: %w", err)
			}
		}
	}

	legacyItems = append(legacyItems,
		legacyPermission{Code: "accounts:read", DisplayName: "查看用户", Description: "允许读取后台用户与角色。", System: true},
		legacyPermission{Code: "accounts:write", DisplayName: "管理用户", Description: "允许创建、编辑、删除用户和角色。", System: true},
		legacyPermission{Code: "credentials:read", DisplayName: "查看密码库", Description: "允许读取自己的密码条目。", System: true},
		legacyPermission{Code: "credentials:write", DisplayName: "编辑密码库", Description: "允许新增、更新、删除自己的密码条目。", System: true},
		legacyPermission{Code: "credentials:reveal", DisplayName: "查看密码明文", Description: "允许解密并复制密码字段。", System: true},
	)

	for _, tenantID := range tenants {
		for _, item := range legacyItems {
			code := strings.ToLower(strings.TrimSpace(item.Code))
			if code == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, `
				INSERT IGNORE INTO permissions (
					id, tenant_id, code, display_name, description, is_system, created_at, updated_at
				) VALUES (
					CONCAT('perm_', REPLACE(UUID(), '-', '')), ?, ?, ?, ?, ?, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)
				)
			`, tenantID, code, strings.TrimSpace(item.DisplayName), strings.TrimSpace(item.Description), item.System); err != nil {
				return fmt.Errorf("backfill permission: %w", err)
			}
		}
	}
	return nil
}

func backfillLegacyAccountRoles(ctx context.Context, db *sql.DB) error {
	exists, err := columnExists(ctx, db, "accounts", "roles_json")
	if err != nil || !exists {
		return err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, tenant_id, roles_json
		FROM accounts
		WHERE role_id IS NULL OR role_id = ''
	`)
	if err != nil {
		return fmt.Errorf("query legacy account roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			accountID  string
			tenantID   string
			rolesJSON  []byte
			roleNames  []string
			targetName string
			roleID     string
		)
		if err := rows.Scan(&accountID, &tenantID, &rolesJSON); err != nil {
			return fmt.Errorf("scan legacy account roles: %w", err)
		}

		roleNames, err = decodeJSONStringSlice(rolesJSON)
		if err != nil {
			return fmt.Errorf("decode legacy account roles: %w", err)
		}
		if len(roleNames) == 0 {
			continue
		}

		targetName = normalizeLegacyRoleName(roleNames[0])
		if err := db.QueryRowContext(ctx, `
			SELECT id
			FROM roles
			WHERE tenant_id = ? AND LOWER(name) = ?
			LIMIT 1
		`, tenantID, targetName).Scan(&roleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return fmt.Errorf("resolve legacy account role: %w", err)
		}

		if _, err := db.ExecContext(ctx, `
			UPDATE accounts
			SET role_id = ?
			WHERE id = ?
		`, roleID, accountID); err != nil {
			return fmt.Errorf("assign legacy account role: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy account roles: %w", err)
	}
	return nil
}

func backfillCredentialOwners(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		UPDATE credential_records records
		INNER JOIN accounts users
			ON users.tenant_id = records.tenant_id
			AND LOWER(users.username) = LOWER(records.created_by)
		SET records.owner_account_id = users.id
		WHERE records.owner_account_id IS NULL OR records.owner_account_id = ''
	`)
	if err != nil {
		return fmt.Errorf("backfill credential owners: %w", err)
	}
	return nil
}

func normalizeLegacyRoleName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "admin", "super_admin":
		return "master"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func columnExists(ctx context.Context, db *sql.DB, tableName, columnName string) (bool, error) {
	var total int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND column_name = ?
	`, tableName, columnName).Scan(&total); err != nil {
		return false, fmt.Errorf("lookup column %s.%s: %w", tableName, columnName, err)
	}
	return total > 0, nil
}

func tableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var total int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name = ?
	`, tableName).Scan(&total); err != nil {
		return false, fmt.Errorf("lookup table %s: %w", tableName, err)
	}
	return total > 0, nil
}

func dropTableIfExists(ctx context.Context, db *sql.DB, tableName string) error {
	exists, err := tableExists(ctx, db, tableName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE %s`, tableName)); err != nil {
		return fmt.Errorf("drop table %s: %w", tableName, err)
	}
	return nil
}

func loadKnownTenants(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT tenant_id FROM roles
		UNION
		SELECT tenant_id FROM accounts
	`)
	if err != nil {
		return nil, fmt.Errorf("load known tenants: %w", err)
	}
	defer rows.Close()

	tenants := make([]string, 0)
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			return nil, fmt.Errorf("scan known tenant: %w", err)
		}
		tenants = append(tenants, strings.ToLower(strings.TrimSpace(tenantID)))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate known tenants: %w", err)
	}
	return uniqueStrings(tenants), nil
}

func decodeJSONStringSlice(value []byte) ([]string, error) {
	if len(value) == 0 {
		return nil, nil
	}

	var items []string
	if err := json.Unmarshal(value, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func isMySQLIgnorable(err error, codes ...uint16) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	for _, code := range codes {
		if mysqlErr.Number == code {
			return true
		}
	}
	return false
}
