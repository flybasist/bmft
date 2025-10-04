package repositories

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// Вспомогательная функция для создания тестовой БД
func setupTestDB(t *testing.T) *sql.DB {
	// Используем тестовую БД или in-memory SQLite для CI/CD
	// Для локального тестирования используем PostgreSQL из docker-compose
	dsn := "postgres://bmft:bmft@localhost:5432/bmft?sslmode=disable"

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping test: cannot connect to test db: %v", err)
		return nil
	}

	if err := db.Ping(); err != nil {
		t.Skipf("skipping test: test db not available: %v", err)
		return nil
	}

	return db
}

// Очистка тестовых данных после теста
func cleanupTestData(t *testing.T, db *sql.DB, userID int64) {
	_, err := db.Exec("DELETE FROM user_limits WHERE user_id = $1", userID)
	if err != nil {
		t.Logf("warning: failed to cleanup test data: %v", err)
	}
}

func TestGetOrCreate(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999001)
	testUsername := "test_user_1"
	defer cleanupTestData(t, db, testUserID)

	// Тест 1: Создание новой записи
	limit, err := repo.GetOrCreate(testUserID, testUsername)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	if limit.UserID != testUserID {
		t.Errorf("expected user_id %d, got %d", testUserID, limit.UserID)
	}

	if limit.Username != testUsername {
		t.Errorf("expected username %s, got %s", testUsername, limit.Username)
	}

	if limit.DailyLimit != 10 {
		t.Errorf("expected default daily_limit 10, got %d", limit.DailyLimit)
	}

	if limit.MonthlyLimit != 300 {
		t.Errorf("expected default monthly_limit 300, got %d", limit.MonthlyLimit)
	}

	if limit.DailyUsed != 0 {
		t.Errorf("expected daily_used 0, got %d", limit.DailyUsed)
	}

	if limit.MonthlyUsed != 0 {
		t.Errorf("expected monthly_used 0, got %d", limit.MonthlyUsed)
	}

	// Тест 2: Получение существующей записи (должно обновить username)
	newUsername := "updated_user"
	limit2, err := repo.GetOrCreate(testUserID, newUsername)
	if err != nil {
		t.Fatalf("GetOrCreate (second call) failed: %v", err)
	}

	if limit2.Username != newUsername {
		t.Errorf("expected updated username %s, got %s", newUsername, limit2.Username)
	}

	// Счётчики не должны измениться
	if limit2.DailyUsed != 0 {
		t.Errorf("expected daily_used to remain 0, got %d", limit2.DailyUsed)
	}
}

func TestCheckAndIncrement_Success(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999002)
	testUsername := "test_user_2"
	defer cleanupTestData(t, db, testUserID)

	// Создаём пользователя
	_, err := repo.GetOrCreate(testUserID, testUsername)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// Первый запрос — должен пройти
	allowed, info, err := repo.CheckAndIncrement(testUserID, testUsername)
	if err != nil {
		t.Fatalf("CheckAndIncrement failed: %v", err)
	}

	if !allowed {
		t.Error("expected request to be allowed")
	}

	if info.DailyUsed != 1 {
		t.Errorf("expected daily_used 1, got %d", info.DailyUsed)
	}

	if info.MonthlyUsed != 1 {
		t.Errorf("expected monthly_used 1, got %d", info.MonthlyUsed)
	}

	if info.DailyRemaining != 9 {
		t.Errorf("expected daily_remaining 9, got %d", info.DailyRemaining)
	}

	if info.MonthlyRemaining != 299 {
		t.Errorf("expected monthly_remaining 299, got %d", info.MonthlyRemaining)
	}

	// Второй запрос — тоже должен пройти
	allowed2, info2, err := repo.CheckAndIncrement(testUserID, testUsername)
	if err != nil {
		t.Fatalf("CheckAndIncrement (second) failed: %v", err)
	}

	if !allowed2 {
		t.Error("expected second request to be allowed")
	}

	if info2.DailyUsed != 2 {
		t.Errorf("expected daily_used 2, got %d", info2.DailyUsed)
	}
}

func TestCheckAndIncrement_DailyExceeded(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999003)
	testUsername := "test_user_3"
	defer cleanupTestData(t, db, testUserID)

	// Создаём пользователя
	_, err := repo.GetOrCreate(testUserID, testUsername)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// Устанавливаем дневной лимит = 2
	err = repo.SetDailyLimit(testUserID, 2)
	if err != nil {
		t.Fatalf("SetDailyLimit failed: %v", err)
	}

	// Делаем 2 успешных запроса
	repo.CheckAndIncrement(testUserID, testUsername)
	repo.CheckAndIncrement(testUserID, testUsername)

	// Третий запрос должен быть заблокирован
	allowed, info, err := repo.CheckAndIncrement(testUserID, testUsername)
	if err != nil {
		t.Fatalf("CheckAndIncrement failed: %v", err)
	}

	if allowed {
		t.Error("expected request to be blocked (daily limit exceeded)")
	}

	if info.DailyUsed != 2 {
		t.Errorf("expected daily_used 2, got %d", info.DailyUsed)
	}

	if info.DailyRemaining != 0 {
		t.Errorf("expected daily_remaining 0, got %d", info.DailyRemaining)
	}
}

func TestCheckAndIncrement_MonthlyExceeded(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999004)
	testUsername := "test_user_4"
	defer cleanupTestData(t, db, testUserID)

	// Создаём пользователя
	_, err := repo.GetOrCreate(testUserID, testUsername)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// Устанавливаем месячный лимит = 2
	err = repo.SetMonthlyLimit(testUserID, 2)
	if err != nil {
		t.Fatalf("SetMonthlyLimit failed: %v", err)
	}

	// Делаем 2 успешных запроса
	repo.CheckAndIncrement(testUserID, testUsername)
	repo.CheckAndIncrement(testUserID, testUsername)

	// Третий запрос должен быть заблокирован
	allowed, info, err := repo.CheckAndIncrement(testUserID, testUsername)
	if err != nil {
		t.Fatalf("CheckAndIncrement failed: %v", err)
	}

	if allowed {
		t.Error("expected request to be blocked (monthly limit exceeded)")
	}

	if info.MonthlyUsed != 2 {
		t.Errorf("expected monthly_used 2, got %d", info.MonthlyUsed)
	}

	if info.MonthlyRemaining != 0 {
		t.Errorf("expected monthly_remaining 0, got %d", info.MonthlyRemaining)
	}
}

func TestSetDailyLimit(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999005)
	defer cleanupTestData(t, db, testUserID)

	// Устанавливаем лимит для несуществующего пользователя
	// Должно создать запись и установить лимит
	err := repo.SetDailyLimit(testUserID, 50)
	if err != nil {
		t.Fatalf("SetDailyLimit failed: %v", err)
	}

	// Проверяем что лимит установлен
	info, err := repo.GetLimitInfo(testUserID)
	if err != nil {
		t.Fatalf("GetLimitInfo failed: %v", err)
	}

	if info.DailyLimit != 50 {
		t.Errorf("expected daily_limit 50, got %d", info.DailyLimit)
	}

	// Обновляем лимит
	err = repo.SetDailyLimit(testUserID, 100)
	if err != nil {
		t.Fatalf("SetDailyLimit (update) failed: %v", err)
	}

	info2, err := repo.GetLimitInfo(testUserID)
	if err != nil {
		t.Fatalf("GetLimitInfo (after update) failed: %v", err)
	}

	if info2.DailyLimit != 100 {
		t.Errorf("expected updated daily_limit 100, got %d", info2.DailyLimit)
	}
}

func TestSetMonthlyLimit(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999006)
	defer cleanupTestData(t, db, testUserID)

	// Устанавливаем месячный лимит
	err := repo.SetMonthlyLimit(testUserID, 500)
	if err != nil {
		t.Fatalf("SetMonthlyLimit failed: %v", err)
	}

	info, err := repo.GetLimitInfo(testUserID)
	if err != nil {
		t.Fatalf("GetLimitInfo failed: %v", err)
	}

	if info.MonthlyLimit != 500 {
		t.Errorf("expected monthly_limit 500, got %d", info.MonthlyLimit)
	}
}

func TestResetDailyIfNeeded(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999007)
	testUsername := "test_user_7"
	defer cleanupTestData(t, db, testUserID)

	// Создаём пользователя и используем 5 запросов
	repo.GetOrCreate(testUserID, testUsername)
	for i := 0; i < 5; i++ {
		repo.CheckAndIncrement(testUserID, testUsername)
	}

	// Проверяем что использовано 5 запросов
	info, _ := repo.GetLimitInfo(testUserID)
	if info.DailyUsed != 5 {
		t.Errorf("expected daily_used 5, got %d", info.DailyUsed)
	}

	// Меняем last_reset_daily на вчера (эмуляция прошедших 24 часов)
	yesterday := time.Now().Add(-25 * time.Hour)
	_, err := db.Exec(
		"UPDATE user_limits SET last_reset_daily = $1 WHERE user_id = $2",
		yesterday, testUserID,
	)
	if err != nil {
		t.Fatalf("failed to update last_reset_daily: %v", err)
	}

	// Вызываем сброс
	err = repo.ResetDailyIfNeeded(testUserID)
	if err != nil {
		t.Fatalf("ResetDailyIfNeeded failed: %v", err)
	}

	// Проверяем что счётчик сбросился
	info2, _ := repo.GetLimitInfo(testUserID)
	if info2.DailyUsed != 0 {
		t.Errorf("expected daily_used 0 after reset, got %d", info2.DailyUsed)
	}

	// Monthly не должен измениться
	if info2.MonthlyUsed != 5 {
		t.Errorf("expected monthly_used to remain 5, got %d", info2.MonthlyUsed)
	}
}

func TestResetMonthlyIfNeeded(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999008)
	testUsername := "test_user_8"
	defer cleanupTestData(t, db, testUserID)

	// Создаём пользователя и используем 50 запросов
	repo.GetOrCreate(testUserID, testUsername)
	for i := 0; i < 50; i++ {
		repo.CheckAndIncrement(testUserID, testUsername)
		// Сбрасываем дневной счётчик чтобы не блокировать
		if i > 0 && i%10 == 0 {
			db.Exec("UPDATE user_limits SET daily_used = 0 WHERE user_id = $1", testUserID)
		}
	}

	// Проверяем что использовано 50 запросов за месяц
	info, _ := repo.GetLimitInfo(testUserID)
	if info.MonthlyUsed < 50 {
		t.Errorf("expected monthly_used >= 50, got %d", info.MonthlyUsed)
	}

	// Меняем last_reset_monthly на 31 день назад
	lastMonth := time.Now().Add(-31 * 24 * time.Hour)
	_, err := db.Exec(
		"UPDATE user_limits SET last_reset_monthly = $1 WHERE user_id = $2",
		lastMonth, testUserID,
	)
	if err != nil {
		t.Fatalf("failed to update last_reset_monthly: %v", err)
	}

	// Вызываем сброс
	err = repo.ResetMonthlyIfNeeded(testUserID)
	if err != nil {
		t.Fatalf("ResetMonthlyIfNeeded failed: %v", err)
	}

	// Проверяем что месячный счётчик сбросился
	info2, _ := repo.GetLimitInfo(testUserID)
	if info2.MonthlyUsed != 0 {
		t.Errorf("expected monthly_used 0 after reset, got %d", info2.MonthlyUsed)
	}
}

func TestGetLimitInfo_NonExistentUser(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	logger := zap.NewNop()
	repo := NewLimitRepository(db, logger)

	testUserID := int64(999999999)

	// Получаем информацию о несуществующем пользователе
	// Должны вернуться дефолтные значения
	info, err := repo.GetLimitInfo(testUserID)
	if err != nil {
		t.Fatalf("GetLimitInfo failed: %v", err)
	}

	if info.DailyLimit != 10 {
		t.Errorf("expected default daily_limit 10, got %d", info.DailyLimit)
	}

	if info.MonthlyLimit != 300 {
		t.Errorf("expected default monthly_limit 300, got %d", info.MonthlyLimit)
	}

	if info.DailyUsed != 0 {
		t.Errorf("expected daily_used 0, got %d", info.DailyUsed)
	}

	if info.MonthlyUsed != 0 {
		t.Errorf("expected monthly_used 0, got %d", info.MonthlyUsed)
	}

	if info.DailyRemaining != 10 {
		t.Errorf("expected daily_remaining 10, got %d", info.DailyRemaining)
	}

	if info.MonthlyRemaining != 300 {
		t.Errorf("expected monthly_remaining 300, got %d", info.MonthlyRemaining)
	}
}
