package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
)

type roleMeta struct {
	Name        string
	Permissions []string
}

func loadRoleMetaByIDs(ctx context.Context, db *sql.DB, roleIDs []string) (map[string]roleMeta, error) {
	ids := uniqueStrings(roleIDs)
	if len(ids) == 0 {
		return map[string]roleMeta{}, nil
	}

	query := fmt.Sprintf(`
		SELECT r.id, r.name, bindings.permission_code
		FROM roles r
		LEFT JOIN role_permissions bindings ON bindings.role_id = r.id
		WHERE r.id IN (%s)
		ORDER BY r.name ASC, bindings.permission_code ASC
	`, placeholders(len(ids)))

	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load role permissions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]roleMeta, len(ids))
	for rows.Next() {
		var (
			roleID         string
			name           string
			permissionCode sql.NullString
		)
		if err := rows.Scan(&roleID, &name, &permissionCode); err != nil {
			return nil, fmt.Errorf("scan role permissions: %w", err)
		}

		meta := result[roleID]
		meta.Name = strings.TrimSpace(name)
		if permissionCode.Valid {
			meta.Permissions = append(meta.Permissions, strings.TrimSpace(permissionCode.String))
		}
		result[roleID] = meta
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role permissions: %w", err)
	}

	for roleID, meta := range result {
		meta.Permissions = uniqueStrings(meta.Permissions)
		result[roleID] = meta
	}

	return result, nil
}

func replaceRolePermissionsTx(ctx context.Context, tx *sql.Tx, roleID string, permissions []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = ?`, strings.TrimSpace(roleID)); err != nil {
		return fmt.Errorf("clear role permissions: %w", err)
	}

	items := uniqueStrings(permissions)
	for _, permission := range items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO role_permissions (role_id, permission_code)
			VALUES (?, ?)
		`, strings.TrimSpace(roleID), permission); err != nil {
			return fmt.Errorf("insert role permission: %w", err)
		}
	}

	return nil
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func uniqueStrings(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}

	slices.Sort(items)
	return slices.Compact(items)
}
