package repositories

import (
	"database/sql"
	"fmt"
)

// ============================================================================
// VIPRepository - управление VIP пользователями
// ============================================================================

// VIPRepository управляет VIP пользователями
type VIPRepository struct {
	db *sql.DB
}

// NewVIPRepository создаёт новый репозиторий VIP
func NewVIPRepository(db *sql.DB) *VIPRepository {
	return &VIPRepository{
		db: db,
	}
}

// IsVIP проверяет является ли пользователь VIP в данном чате/топике.
// Логика fallback: сначала проверяем VIP для конкретного топика,
// если нет - проверяем VIP для всего чата (thread_id = 0).
func (r *VIPRepository) IsVIP(chatID int64, threadID int, userID int64) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM chat_vips 
			WHERE chat_id = $1 AND thread_id = $2 AND user_id = $3
		)
	`

	// Сначала проверяем VIP для конкретного топика
	err := r.db.QueryRow(query, chatID, threadID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check VIP status: %w", err)
	}

	if !exists && threadID != 0 {
		// Если не VIP в топике, проверяем VIP для всего чата
		err = r.db.QueryRow(query, chatID, 0, userID).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("check VIP status (chat-wide): %w", err)
		}
	}

	return exists, nil
}

// GrantVIP выдаёт VIP статус пользователю в чате/топике.
// threadID = 0 означает VIP для всего чата, >0 - только для конкретного топика.
func (r *VIPRepository) GrantVIP(chatID int64, threadID int, userID, grantedBy int64, reason string) error {
	_, err := r.db.Exec(`
		INSERT INTO chat_vips (chat_id, thread_id, user_id, granted_by, reason)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (chat_id, thread_id, user_id) DO UPDATE
		SET granted_by = EXCLUDED.granted_by,
		    reason = EXCLUDED.reason,
		    granted_at = NOW()
	`, chatID, threadID, userID, grantedBy, reason)

	if err != nil {
		return fmt.Errorf("grant VIP: %w", err)
	}

	return nil
}

// RevokeVIP забирает VIP статус из чата/топика.
func (r *VIPRepository) RevokeVIP(chatID int64, threadID int, userID int64) error {
	result, err := r.db.Exec(`
		DELETE FROM chat_vips
		WHERE chat_id = $1 AND thread_id = $2 AND user_id = $3
	`, chatID, threadID, userID)

	if err != nil {
		return fmt.Errorf("revoke VIP: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user is not VIP")
	}

	return nil
}

// ListVIPs возвращает список всех VIP пользователей в чате/топике.
// threadID = 0 - список для всего чата, >0 - список для конкретного топика.
func (r *VIPRepository) ListVIPs(chatID int64, threadID int) ([]VIPInfo, error) {
	rows, err := r.db.Query(`
		SELECT 
			cv.user_id,
			cv.thread_id,
			COALESCE(u.username, ''),
			COALESCE(u.first_name, ''),
			cv.granted_at,
			COALESCE(cv.reason, '')
		FROM chat_vips cv
		LEFT JOIN users u ON cv.user_id = u.user_id
		WHERE cv.chat_id = $1 AND cv.thread_id = $2
		ORDER BY cv.granted_at DESC
	`, chatID, threadID)

	if err != nil {
		return nil, fmt.Errorf("list VIPs: %w", err)
	}
	defer rows.Close()

	var vips []VIPInfo
	for rows.Next() {
		var vip VIPInfo
		err := rows.Scan(
			&vip.UserID,
			&vip.ThreadID,
			&vip.Username,
			&vip.FirstName,
			&vip.GrantedAt,
			&vip.Reason,
		)
		if err != nil {
			continue // Пропускаем невалидные записи
		}
		vips = append(vips, vip)
	}

	return vips, nil
}

// VIPInfo содержит информацию о VIP пользователе
type VIPInfo struct {
	UserID    int64
	ThreadID  int // 0 = VIP для всего чата, >0 = VIP только в топике
	Username  string
	FirstName string
	GrantedAt string
	Reason    string
}
