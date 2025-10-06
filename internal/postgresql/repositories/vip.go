package repositories

import (
	"database/sql"
	"fmt"

	"go.uber.org/zap"
)

// VIPRepository управляет VIP пользователями
type VIPRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewVIPRepository создаёт новый репозиторий VIP
func NewVIPRepository(db *sql.DB, logger *zap.Logger) *VIPRepository {
	return &VIPRepository{
		db:     db,
		logger: logger,
	}
}

// IsVIP проверяет является ли пользователь VIP в данном чате
func (r *VIPRepository) IsVIP(chatID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM chat_vips 
			WHERE chat_id = $1 AND user_id = $2
		)
	`, chatID, userID).Scan(&exists)
	
	if err != nil {
		r.logger.Error("failed to check VIP status",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return false, fmt.Errorf("check VIP status: %w", err)
	}
	
	return exists, nil
}

// GrantVIP выдаёт VIP статус пользователю
func (r *VIPRepository) GrantVIP(chatID, userID, grantedBy int64, reason string) error {
	_, err := r.db.Exec(`
		INSERT INTO chat_vips (chat_id, user_id, granted_by, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id, user_id) DO UPDATE
		SET granted_by = EXCLUDED.granted_by,
		    reason = EXCLUDED.reason,
		    granted_at = NOW()
	`, chatID, userID, grantedBy, reason)
	
	if err != nil {
		r.logger.Error("failed to grant VIP",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return fmt.Errorf("grant VIP: %w", err)
	}
	
	r.logger.Info("VIP granted",
		zap.Int64("chat_id", chatID),
		zap.Int64("user_id", userID),
		zap.Int64("granted_by", grantedBy),
		zap.String("reason", reason),
	)
	
	return nil
}

// RevokeVIP забирает VIP статус
func (r *VIPRepository) RevokeVIP(chatID, userID int64) error {
	result, err := r.db.Exec(`
		DELETE FROM chat_vips
		WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID)
	
	if err != nil {
		r.logger.Error("failed to revoke VIP",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return fmt.Errorf("revoke VIP: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user is not VIP")
	}
	
	r.logger.Info("VIP revoked",
		zap.Int64("chat_id", chatID),
		zap.Int64("user_id", userID),
	)
	
	return nil
}

// ListVIPs возвращает список всех VIP пользователей в чате
func (r *VIPRepository) ListVIPs(chatID int64) ([]VIPInfo, error) {
	rows, err := r.db.Query(`
		SELECT 
			cv.user_id,
			COALESCE(u.username, ''),
			COALESCE(u.first_name, ''),
			cv.granted_at,
			COALESCE(cv.reason, '')
		FROM chat_vips cv
		LEFT JOIN users u ON cv.user_id = u.user_id
		WHERE cv.chat_id = $1
		ORDER BY cv.granted_at DESC
	`, chatID)
	
	if err != nil {
		r.logger.Error("failed to list VIPs",
			zap.Int64("chat_id", chatID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("list VIPs: %w", err)
	}
	defer rows.Close()
	
	var vips []VIPInfo
	for rows.Next() {
		var vip VIPInfo
		err := rows.Scan(
			&vip.UserID,
			&vip.Username,
			&vip.FirstName,
			&vip.GrantedAt,
			&vip.Reason,
		)
		if err != nil {
			r.logger.Error("failed to scan VIP",
				zap.Error(err),
			)
			continue
		}
		vips = append(vips, vip)
	}
	
	return vips, nil
}

// VIPInfo содержит информацию о VIP пользователе
type VIPInfo struct {
	UserID    int64
	Username  string
	FirstName string
	GrantedAt string
	Reason    string
}
