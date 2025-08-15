package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"auth-service/app/domain"
	"auth-service/app/port"

	"github.com/google/uuid"
)

// SessionSyncUsecase はセッション状態をKratosと完全同期
type SessionSyncUsecase struct {
	authGateway port.AuthGateway
	sessionRepo port.SessionRepository
	logger      *slog.Logger
}

// NewSessionSyncUsecase creates a new SessionSyncUsecase
func NewSessionSyncUsecase(
	authGateway port.AuthGateway,
	sessionRepo port.SessionRepository,
	logger *slog.Logger,
) *SessionSyncUsecase {
	return &SessionSyncUsecase{
		authGateway: authGateway,
		sessionRepo: sessionRepo,
		logger:      logger.With("component", "session_sync_usecase"),
	}
}

// SyncSessionWithKratos はKratosセッションと内部セッション状態を同期
func (u *SessionSyncUsecase) SyncSessionWithKratos(ctx context.Context, sessionID string) error {
	syncId := fmt.Sprintf("SYNC-%d", time.Now().UnixNano())
	u.logger.Info("🔄 Starting session synchronization with Kratos",
		"sync_id", syncId,
		"session_id", sessionID)

	// 1. Kratosからセッション情報を取得
	kratosSession, err := u.authGateway.GetSession(ctx, sessionID)
	if err != nil {
		u.logger.Error("failed to get session from Kratos",
			"sync_id", syncId,
			"session_id", sessionID,
			"error", err)
		return fmt.Errorf("failed to get Kratos session: %w", err)
	}

	// 2. 内部セッション状態を取得
	internalSession, err := u.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		u.logger.Warn("internal session not found, creating new one",
			"sync_id", syncId,
			"session_id", sessionID)
		
		// 内部セッションが存在しない場合は新規作成
		return u.createInternalSessionFromKratos(ctx, kratosSession, syncId)
	}

	// 3. 不整合があれば修正
	inconsistencies := u.detectSessionInconsistencies(kratosSession, internalSession)
	if len(inconsistencies) > 0 {
		u.logger.Warn("session inconsistencies detected",
			"sync_id", syncId,
			"session_id", sessionID,
			"inconsistency_count", len(inconsistencies),
			"inconsistencies", inconsistencies)

		err := u.resolveSessionInconsistencies(ctx, kratosSession, internalSession, inconsistencies, syncId)
		if err != nil {
			u.logger.Error("failed to resolve session inconsistencies",
				"sync_id", syncId,
				"session_id", sessionID,
				"error", err)
			return fmt.Errorf("failed to resolve session inconsistencies: %w", err)
		}
	}

	// 4. フロー状態も同期
	err = u.syncFlowStates(ctx, kratosSession, internalSession, syncId)
	if err != nil {
		u.logger.Error("failed to sync flow states",
			"sync_id", syncId,
			"session_id", sessionID,
			"error", err)
		return fmt.Errorf("failed to sync flow states: %w", err)
	}

	u.logger.Info("✅ Session synchronization completed successfully",
		"sync_id", syncId,
		"session_id", sessionID)

	return nil
}

// SyncAllActiveSessions synchronizes all active sessions with Kratos
func (u *SessionSyncUsecase) SyncAllActiveSessions(ctx context.Context) error {
	syncId := fmt.Sprintf("SYNC-ALL-%d", time.Now().UnixNano())
	u.logger.Info("🔄 Starting bulk session synchronization", "sync_id", syncId)

	// Get all active internal sessions
	activeSessions, err := u.sessionRepo.GetActiveSessions(ctx)
	if err != nil {
		u.logger.Error("failed to get active sessions",
			"sync_id", syncId,
			"error", err)
		return fmt.Errorf("failed to get active sessions: %w", err)
	}

	successCount := 0
	errorCount := 0

	for _, session := range activeSessions {
		err := u.SyncSessionWithKratos(ctx, session.KratosSessionID)
		if err != nil {
			u.logger.Error("failed to sync individual session",
				"sync_id", syncId,
				"session_id", session.ID,
				"error", err)
			errorCount++
		} else {
			successCount++
		}
	}

	u.logger.Info("Bulk session synchronization completed",
		"sync_id", syncId,
		"total_sessions", len(activeSessions),
		"success_count", successCount,
		"error_count", errorCount)

	if errorCount > 0 {
		return fmt.Errorf("bulk sync completed with errors: %d/%d sessions failed", 
			errorCount, len(activeSessions))
	}

	return nil
}

// createInternalSessionFromKratos creates a new internal session based on Kratos session
func (u *SessionSyncUsecase) createInternalSessionFromKratos(ctx context.Context, kratosSession *domain.KratosSession, syncId string) error {
	u.logger.Info("creating new internal session from Kratos session",
		"sync_id", syncId,
		"kratos_session_id", kratosSession.ID,
		"user_id", kratosSession.Identity.ID)

	// Parse UUID from Kratos identity ID
	userID, err := uuid.Parse(kratosSession.Identity.ID)
	if err != nil {
		return fmt.Errorf("invalid user ID from Kratos: %w", err)
	}

	// Parse session ID if UUID format, otherwise generate new one
	var sessionID uuid.UUID
	if parsedID, err := uuid.Parse(kratosSession.ID); err == nil {
		sessionID = parsedID
	} else {
		sessionID = uuid.New()
	}

	internalSession := &domain.Session{
		ID:              sessionID,
		UserID:          userID,
		KratosSessionID: kratosSession.ID,
		Active:          kratosSession.Active,
		ExpiresAt:       kratosSession.ExpiresAt,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		LastActivityAt:  time.Now(),
		LastSyncAt:      time.Now(),
		SyncStatus:      domain.SessionSyncStatusSynced,
	}

	err = u.sessionRepo.Create(ctx, internalSession)
	if err != nil {
		u.logger.Error("failed to create internal session",
			"sync_id", syncId,
			"session_id", kratosSession.ID,
			"error", err)
		return fmt.Errorf("failed to create internal session: %w", err)
	}

	u.logger.Info("✅ Internal session created successfully",
		"sync_id", syncId,
		"session_id", kratosSession.ID)

	return nil
}

// detectSessionInconsistencies detects inconsistencies between Kratos and internal sessions
func (u *SessionSyncUsecase) detectSessionInconsistencies(kratosSession *domain.KratosSession, internalSession *domain.Session) []string {
	var inconsistencies []string

	// Check active status
	if kratosSession.Active != internalSession.Active {
		inconsistencies = append(inconsistencies, fmt.Sprintf(
			"active_status_mismatch: kratos=%t, internal=%t", 
			kratosSession.Active, internalSession.Active))
	}

	// Check user ID (convert Kratos ID to UUID for comparison)
	kratosUserID, err := uuid.Parse(kratosSession.Identity.ID)
	if err == nil && kratosUserID != internalSession.UserID {
		inconsistencies = append(inconsistencies, fmt.Sprintf(
			"user_id_mismatch: kratos=%s, internal=%s", 
			kratosSession.Identity.ID, internalSession.UserID.String()))
	}

	// Check expiration time (allow small time differences due to time synchronization)
	timeDiff := kratosSession.ExpiresAt.Sub(internalSession.ExpiresAt)
	if timeDiff < -time.Minute || timeDiff > time.Minute {
		inconsistencies = append(inconsistencies, fmt.Sprintf(
			"expires_at_mismatch: kratos=%s, internal=%s, diff=%s", 
			kratosSession.ExpiresAt.Format(time.RFC3339),
			internalSession.ExpiresAt.Format(time.RFC3339),
			timeDiff.String()))
	}

	// Check sync status
	if internalSession.SyncStatus != domain.SessionSyncStatusSynced {
		inconsistencies = append(inconsistencies, fmt.Sprintf(
			"sync_status_outdated: %s", internalSession.SyncStatus))
	}

	return inconsistencies
}

// resolveSessionInconsistencies resolves detected inconsistencies
func (u *SessionSyncUsecase) resolveSessionInconsistencies(
	ctx context.Context,
	kratosSession *domain.KratosSession,
	internalSession *domain.Session,
	inconsistencies []string,
	syncId string,
) error {
	u.logger.Info("resolving session inconsistencies",
		"sync_id", syncId,
		"session_id", internalSession.ID,
		"inconsistencies", inconsistencies)

	// Parse Kratos user ID
	kratosUserID, err := uuid.Parse(kratosSession.Identity.ID)
	if err != nil {
		return fmt.Errorf("invalid Kratos user ID: %w", err)
	}

	// Update internal session to match Kratos session
	updatedSession := &domain.Session{
		ID:                 internalSession.ID,
		UserID:            kratosUserID,                  // Update from Kratos
		KratosSessionID:   internalSession.KratosSessionID,
		Active:            kratosSession.Active,         // Update from Kratos
		ExpiresAt:         kratosSession.ExpiresAt,      // Update from Kratos
		CreatedAt:         internalSession.CreatedAt,    // Keep original
		UpdatedAt:         time.Now(),                   // Mark as updated
		LastActivityAt:    internalSession.LastActivityAt,
		IPAddress:         internalSession.IPAddress,
		UserAgent:         internalSession.UserAgent,
		DeviceInfo:        internalSession.DeviceInfo,
		SessionMetadata:   internalSession.SessionMetadata,
		LastSyncAt:        time.Now(),                   // Mark sync time
		SyncStatus:        domain.SessionSyncStatusSynced,
	}

	err = u.sessionRepo.Update(ctx, updatedSession)
	if err != nil {
		u.logger.Error("failed to update internal session",
			"sync_id", syncId,
			"session_id", internalSession.ID,
			"error", err)
		return fmt.Errorf("failed to update internal session: %w", err)
	}

	u.logger.Info("✅ Session inconsistencies resolved",
		"sync_id", syncId,
		"session_id", internalSession.ID,
		"resolved_count", len(inconsistencies))

	return nil
}

// syncFlowStates synchronizes flow states between Kratos and internal systems
func (u *SessionSyncUsecase) syncFlowStates(
	ctx context.Context,
	kratosSession *domain.KratosSession,
	internalSession *domain.Session,
	syncId string,
) error {
	u.logger.Debug("synchronizing flow states",
		"sync_id", syncId,
		"session_id", internalSession.ID)

	// For now, we just log that flow sync would happen here
	// In a full implementation, this would:
	// 1. Check for active flows in Kratos
	// 2. Compare with internal flow state
	// 3. Sync any mismatches
	// 4. Clean up expired flows

	u.logger.Debug("flow state synchronization completed",
		"sync_id", syncId,
		"session_id", internalSession.ID)

	return nil
}

// CheckSessionHealth performs health check on session synchronization
func (u *SessionSyncUsecase) CheckSessionHealth(ctx context.Context) (*domain.SessionHealthStatus, error) {
	healthId := fmt.Sprintf("HEALTH-%d", time.Now().UnixNano())
	u.logger.Debug("checking session health", "health_id", healthId)

	// Get statistics
	totalSessions, err := u.sessionRepo.GetSessionCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get session count: %w", err)
	}

	activeSessions, err := u.sessionRepo.GetActiveSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	// Calculate health metrics
	activeCount := len(activeSessions)
	outdatedSyncCount := 0
	
	for _, session := range activeSessions {
		// Check if session needs sync (older than 5 minutes)
		if time.Since(session.LastSyncAt) > 5*time.Minute {
			outdatedSyncCount++
		}
	}

	healthStatus := &domain.SessionHealthStatus{
		TotalSessions:     totalSessions,
		ActiveSessions:    activeCount,
		OutdatedSyncCount: outdatedSyncCount,
		HealthScore:       u.calculateHealthScore(activeCount, outdatedSyncCount),
		CheckedAt:         time.Now(),
	}

	u.logger.Info("session health check completed",
		"health_id", healthId,
		"total_sessions", totalSessions,
		"active_sessions", activeCount,
		"outdated_sync_count", outdatedSyncCount,
		"health_score", healthStatus.HealthScore)

	return healthStatus, nil
}

// calculateHealthScore calculates a health score based on session sync status
func (u *SessionSyncUsecase) calculateHealthScore(activeSessions, outdatedSyncCount int) float64 {
	if activeSessions == 0 {
		return 100.0 // Perfect score if no sessions to manage
	}

	syncedSessions := activeSessions - outdatedSyncCount
	return (float64(syncedSessions) / float64(activeSessions)) * 100.0
}