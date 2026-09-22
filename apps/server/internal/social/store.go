// Package social records friendships and the per-user settings that gate how much
// reading activity a friend may see.
package social

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSelfFriend         = errors.New("cannot friend yourself")
	ErrUserNotFound       = errors.New("user not found")
	ErrAlreadyFriends     = errors.New("already friends")
	ErrRequestAlreadySent = errors.New("friend request already sent")
	ErrRequestNotFound    = errors.New("friend request not found")
	ErrFriendshipNotFound = errors.New("friendship not found")
	ErrInvalidGoal        = errors.New("goal must be a positive number of books for a valid year")
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Friendship is always caller-relative: UserID is the caller, FriendID is the other party.
type Friendship struct {
	UserID, FriendID     string
	CreatedAt, UpdatedAt time.Time
}

// FriendRequest is always caller-relative: OtherUserID is the other party of a pending
// request; RequestedBy is who sent it (== the caller for an outgoing request, or ==
// OtherUserID for an incoming one).
type FriendRequest struct {
	OtherUserID string
	RequestedBy string
	CreatedAt   time.Time
}

type Settings struct {
	ActivityVisible     bool
	GoalYear, GoalBooks int
	UpdatedAt           time.Time
}

func NewStore(db *sql.DB) *Store {
	return newStoreWithClock(db, time.Now)
}

func newStoreWithClock(db *sql.DB, now func() time.Time) *Store {
	return &Store{db: db, now: now}
}

// SendRequest creates a pending friend request from fromUserID to toUserID. If toUserID
// already has a pending request to fromUserID, the friendship is accepted immediately
// rather than erroring, since both parties already wanted it.
func (s *Store) SendRequest(ctx context.Context, fromUserID, toUserID string) error {
	if fromUserID == toUserID {
		return ErrSelfFriend
	}
	lo, hi := canonicalPair(fromUserID, toUserID)
	now := s.now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin friend request: %w", err)
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, toUserID).Scan(&exists); err != nil {
		return fmt.Errorf("check friend exists: %w", err)
	}
	if exists == 0 {
		return ErrUserNotFound
	}

	var status, requestedBy string
	err = tx.QueryRowContext(ctx, `SELECT status, requested_by FROM friendships WHERE user_id = ? AND friend_id = ?`, lo, hi).
		Scan(&status, &requestedBy)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO friendships (user_id, friend_id, status, requested_by, created_at, updated_at)
			VALUES (?, ?, 'pending', ?, ?, ?)
		`, lo, hi, fromUserID, now, now); err != nil {
			return fmt.Errorf("insert friend request: %w", err)
		}
	case err != nil:
		return fmt.Errorf("read friendship: %w", err)
	case status == "accepted":
		return ErrAlreadyFriends
	case requestedBy == fromUserID:
		return ErrRequestAlreadySent
	default:
		// toUserID already asked us first: both sides want this, so accept rather than error.
		if _, err := tx.ExecContext(ctx, `
			UPDATE friendships SET status = 'accepted', updated_at = ? WHERE user_id = ? AND friend_id = ?
		`, now, lo, hi); err != nil {
			return fmt.Errorf("accept mutual friend request: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit friend request: %w", err)
	}
	return nil
}

// AcceptRequest accepts a pending request sent by fromUserID to userID. Only the
// recipient may accept.
func (s *Store) AcceptRequest(ctx context.Context, userID, fromUserID string) error {
	lo, hi := canonicalPair(userID, fromUserID)
	now := s.now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE friendships SET status = 'accepted', updated_at = ?
		WHERE user_id = ? AND friend_id = ? AND status = 'pending' AND requested_by = ?
	`, now, lo, hi, fromUserID)
	if err != nil {
		return fmt.Errorf("accept friend request: %w", err)
	}
	return requireAffected(result, ErrRequestNotFound)
}

// DeclineRequest removes a pending request sent by fromUserID to userID (the recipient).
func (s *Store) DeclineRequest(ctx context.Context, userID, fromUserID string) error {
	return s.deletePendingRequest(ctx, userID, fromUserID, fromUserID)
}

// CancelRequest removes a pending request userID sent to toUserID.
func (s *Store) CancelRequest(ctx context.Context, userID, toUserID string) error {
	return s.deletePendingRequest(ctx, userID, toUserID, userID)
}

func (s *Store) deletePendingRequest(ctx context.Context, userID, otherUserID, requestedBy string) error {
	lo, hi := canonicalPair(userID, otherUserID)
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM friendships WHERE user_id = ? AND friend_id = ? AND status = 'pending' AND requested_by = ?
	`, lo, hi, requestedBy)
	if err != nil {
		return fmt.Errorf("remove friend request: %w", err)
	}
	return requireAffected(result, ErrRequestNotFound)
}

// RemoveFriend deletes an accepted friendship. Either party may call this.
func (s *Store) RemoveFriend(ctx context.Context, userID, friendID string) error {
	lo, hi := canonicalPair(userID, friendID)
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM friendships WHERE user_id = ? AND friend_id = ? AND status = 'accepted'
	`, lo, hi)
	if err != nil {
		return fmt.Errorf("remove friend: %w", err)
	}
	return requireAffected(result, ErrFriendshipNotFound)
}

func requireAffected(result sql.Result, notFound error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if affected == 0 {
		return notFound
	}
	return nil
}

// ListFriends returns userID's accepted friends, most recently changed first.
func (s *Store) ListFriends(ctx context.Context, userID string) ([]Friendship, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, friend_id, created_at, updated_at FROM friendships
		WHERE (user_id = ? OR friend_id = ?) AND status = 'accepted'
		ORDER BY updated_at DESC
	`, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	defer rows.Close()

	friends := make([]Friendship, 0)
	for rows.Next() {
		var rowUserID, rowFriendID, createdAt, updatedAt string
		if err := rows.Scan(&rowUserID, &rowFriendID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan friend: %w", err)
		}
		friend := Friendship{UserID: userID, FriendID: otherOf(rowUserID, rowFriendID, userID)}
		if friend.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse friend created time: %w", err)
		}
		if friend.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, fmt.Errorf("parse friend updated time: %w", err)
		}
		friends = append(friends, friend)
	}
	return friends, rows.Err()
}

// ListIncomingRequests returns pending requests other users sent to userID.
func (s *Store) ListIncomingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	return s.listRequests(ctx, userID, `status = 'pending' AND requested_by != ?`, userID)
}

// ListOutgoingRequests returns pending requests userID sent to other users.
func (s *Store) ListOutgoingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	return s.listRequests(ctx, userID, `status = 'pending' AND requested_by = ?`, userID)
}

func (s *Store) listRequests(ctx context.Context, userID, directionFilter string, directionArg string) ([]FriendRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, friend_id, requested_by, created_at FROM friendships
		WHERE (user_id = ? OR friend_id = ?) AND `+directionFilter+`
		ORDER BY created_at
	`, userID, userID, directionArg)
	if err != nil {
		return nil, fmt.Errorf("list friend requests: %w", err)
	}
	defer rows.Close()

	requests := make([]FriendRequest, 0)
	for rows.Next() {
		var rowUserID, rowFriendID, requestedBy, createdAt string
		if err := rows.Scan(&rowUserID, &rowFriendID, &requestedBy, &createdAt); err != nil {
			return nil, fmt.Errorf("scan friend request: %w", err)
		}
		request := FriendRequest{OtherUserID: otherOf(rowUserID, rowFriendID, userID), RequestedBy: requestedBy}
		if request.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse friend request time: %w", err)
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

// AreFriends reports whether a and b are accepted friends.
func (s *Store) AreFriends(ctx context.Context, a, b string) (bool, error) {
	lo, hi := canonicalPair(a, b)
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM friendships WHERE user_id = ? AND friend_id = ?`, lo, hi).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check friendship: %w", err)
	}
	return status == "accepted", nil
}

// CanViewActivity reports whether viewerID may see targetID's reading activity:
// they must be accepted friends, and targetID must have opted in to sharing.
func (s *Store) CanViewActivity(ctx context.Context, viewerID, targetID string) (bool, error) {
	lo, hi := canonicalPair(viewerID, targetID)
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM friendships WHERE user_id = ? AND friend_id = ?`, lo, hi).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check friendship: %w", err)
	}
	if status != "accepted" {
		return false, nil
	}
	settings, err := s.GetSettings(ctx, targetID)
	if err != nil {
		return false, err
	}
	return settings.ActivityVisible, nil
}

// GetSettings returns userID's social settings, or the zero-value defaults
// (private, no goal) if they have never set any.
func (s *Store) GetSettings(ctx context.Context, userID string) (Settings, error) {
	var visible bool
	var goalYear, goalBooks sql.NullInt64
	var updatedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT activity_visible, annual_goal_year, annual_goal_books, updated_at
		FROM user_social_settings WHERE user_id = ?
	`, userID).Scan(&visible, &goalYear, &goalBooks, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read social settings: %w", err)
	}
	settings := Settings{ActivityVisible: visible, GoalYear: int(goalYear.Int64), GoalBooks: int(goalBooks.Int64)}
	if updatedAt.Valid {
		if settings.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt.String); err != nil {
			return Settings{}, fmt.Errorf("parse settings time: %w", err)
		}
	}
	return settings, nil
}

// UpdateSettings replaces userID's visibility and goal. goalBooks == 0 clears the goal
// regardless of goalYear; a positive goalBooks requires a positive goalYear.
func (s *Store) UpdateSettings(ctx context.Context, userID string, visible bool, goalYear, goalBooks int) (Settings, error) {
	if goalBooks < 0 || (goalBooks > 0 && goalYear <= 0) {
		return Settings{}, ErrInvalidGoal
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	var goalYearValue, goalBooksValue any
	if goalBooks > 0 {
		goalYearValue, goalBooksValue = goalYear, goalBooks
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_social_settings (user_id, activity_visible, annual_goal_year, annual_goal_books, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			activity_visible = excluded.activity_visible,
			annual_goal_year = excluded.annual_goal_year,
			annual_goal_books = excluded.annual_goal_books,
			updated_at = excluded.updated_at
	`, userID, visible, goalYearValue, goalBooksValue, now)
	if err != nil {
		return Settings{}, fmt.Errorf("update social settings: %w", err)
	}
	return s.GetSettings(ctx, userID)
}

// canonicalPair orders two user IDs so a friendship's storage row is independent of who
// queries or writes it.
func canonicalPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

// otherOf returns whichever of a friendship row's two sides is not self.
func otherOf(userID, friendID, self string) string {
	if userID == self {
		return friendID
	}
	return userID
}
