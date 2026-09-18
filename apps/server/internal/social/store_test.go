package social

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
)

func TestSendRequestAndAcceptCreatesFriendship(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()

	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	incoming, err := store.ListIncomingRequests(ctx, "usr_b")
	if err != nil || len(incoming) != 1 || incoming[0].OtherUserID != "usr_a" || incoming[0].RequestedBy != "usr_a" {
		t.Fatalf("ListIncomingRequests(usr_b) = %#v, %v", incoming, err)
	}
	outgoing, err := store.ListOutgoingRequests(ctx, "usr_a")
	if err != nil || len(outgoing) != 1 || outgoing[0].OtherUserID != "usr_b" {
		t.Fatalf("ListOutgoingRequests(usr_a) = %#v, %v", outgoing, err)
	}

	if err := store.AcceptRequest(ctx, "usr_b", "usr_a"); err != nil {
		t.Fatalf("AcceptRequest() error = %v", err)
	}
	friendsA, err := store.ListFriends(ctx, "usr_a")
	if err != nil || len(friendsA) != 1 || friendsA[0].FriendID != "usr_b" {
		t.Fatalf("ListFriends(usr_a) = %#v, %v", friendsA, err)
	}
	friendsB, err := store.ListFriends(ctx, "usr_b")
	if err != nil || len(friendsB) != 1 || friendsB[0].FriendID != "usr_a" {
		t.Fatalf("ListFriends(usr_b) = %#v, %v", friendsB, err)
	}
}

func TestSendRequestBothDirectionsAutoAccepts(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()

	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest(a->b) error = %v", err)
	}
	if err := store.SendRequest(ctx, "usr_b", "usr_a"); err != nil {
		t.Fatalf("SendRequest(b->a) error = %v", err)
	}
	friends, err := store.ListFriends(ctx, "usr_a")
	if err != nil || len(friends) != 1 {
		t.Fatalf("ListFriends(usr_a) = %#v, %v, want one accepted friend", friends, err)
	}
	incoming, _ := store.ListIncomingRequests(ctx, "usr_a")
	if len(incoming) != 0 {
		t.Fatalf("ListIncomingRequests(usr_a) = %#v, want none left pending", incoming)
	}
}

func TestSendRequestRejectsSelf(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	if err := store.SendRequest(context.Background(), "usr_a", "usr_a"); !errors.Is(err, ErrSelfFriend) {
		t.Fatalf("SendRequest(self) error = %v, want ErrSelfFriend", err)
	}
}

func TestSendRequestRejectsDuplicate(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); !errors.Is(err, ErrRequestAlreadySent) {
		t.Fatalf("SendRequest(duplicate) error = %v, want ErrRequestAlreadySent", err)
	}
	if err := store.AcceptRequest(ctx, "usr_b", "usr_a"); err != nil {
		t.Fatalf("AcceptRequest() error = %v", err)
	}
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); !errors.Is(err, ErrAlreadyFriends) {
		t.Fatalf("SendRequest(already friends) error = %v, want ErrAlreadyFriends", err)
	}
}

func TestSendRequestRejectsUnknownUser(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	if err := store.SendRequest(context.Background(), "usr_a", "usr_ghost"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("SendRequest(unknown) error = %v, want ErrUserNotFound", err)
	}
}

func TestDeclineRequestRemovesRow(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	if err := store.DeclineRequest(ctx, "usr_b", "usr_a"); err != nil {
		t.Fatalf("DeclineRequest() error = %v", err)
	}
	incoming, _ := store.ListIncomingRequests(ctx, "usr_b")
	if len(incoming) != 0 {
		t.Fatalf("ListIncomingRequests(usr_b) after decline = %#v, want none", incoming)
	}
	// A fresh request can be sent again -- decline leaves no tombstone.
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() after decline error = %v", err)
	}
}

func TestCancelRequestRemovesRow(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	if err := store.CancelRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("CancelRequest() error = %v", err)
	}
	outgoing, _ := store.ListOutgoingRequests(ctx, "usr_a")
	if len(outgoing) != 0 {
		t.Fatalf("ListOutgoingRequests(usr_a) after cancel = %#v, want none", outgoing)
	}
}

func TestAcceptRequestRejectsNonRecipient(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	// usr_a is the sender, not the recipient, so accepting their own request must fail.
	if err := store.AcceptRequest(ctx, "usr_a", "usr_b"); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("AcceptRequest(sender) error = %v, want ErrRequestNotFound", err)
	}
}

func TestRemoveFriendDeletesAcceptedFriendship(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	if err := store.AcceptRequest(ctx, "usr_b", "usr_a"); err != nil {
		t.Fatalf("AcceptRequest() error = %v", err)
	}
	if err := store.RemoveFriend(ctx, "usr_b", "usr_a"); err != nil {
		t.Fatalf("RemoveFriend() error = %v", err)
	}
	friends, _ := store.ListFriends(ctx, "usr_a")
	if len(friends) != 0 {
		t.Fatalf("ListFriends(usr_a) after remove = %#v, want none", friends)
	}
	if err := store.RemoveFriend(ctx, "usr_a", "usr_b"); !errors.Is(err, ErrFriendshipNotFound) {
		t.Fatalf("RemoveFriend(already removed) error = %v, want ErrFriendshipNotFound", err)
	}
}

func TestListFriendsOnlyReturnsAccepted(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()
	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	friends, err := store.ListFriends(ctx, "usr_a")
	if err != nil || len(friends) != 0 {
		t.Fatalf("ListFriends(usr_a) with pending only = %#v, %v, want none", friends, err)
	}
}

func TestCanViewActivityRequiresFriendshipAndVisibility(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()

	assertView := func(viewer, target string, want bool) {
		t.Helper()
		got, err := store.CanViewActivity(ctx, viewer, target)
		if err != nil {
			t.Fatalf("CanViewActivity(%s, %s) error = %v", viewer, target, err)
		}
		if got != want {
			t.Fatalf("CanViewActivity(%s, %s) = %v, want %v", viewer, target, got, want)
		}
	}

	// Not friends, visibility off (default).
	assertView("usr_a", "usr_b", false)

	// Not friends, visibility on.
	if _, err := store.UpdateSettings(ctx, "usr_b", true, 0, 0); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	assertView("usr_a", "usr_b", false)

	if err := store.SendRequest(ctx, "usr_a", "usr_b"); err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	if err := store.AcceptRequest(ctx, "usr_b", "usr_a"); err != nil {
		t.Fatalf("AcceptRequest() error = %v", err)
	}

	// Friends, visibility on.
	assertView("usr_a", "usr_b", true)

	// Friends, visibility off.
	if _, err := store.UpdateSettings(ctx, "usr_b", false, 0, 0); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	assertView("usr_a", "usr_b", false)
}

func TestUpdateSettingsValidatesGoal(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()

	if _, err := store.UpdateSettings(ctx, "usr_a", true, 0, -1); !errors.Is(err, ErrInvalidGoal) {
		t.Fatalf("UpdateSettings(negative goal) error = %v, want ErrInvalidGoal", err)
	}
	if _, err := store.UpdateSettings(ctx, "usr_a", true, 0, 12); !errors.Is(err, ErrInvalidGoal) {
		t.Fatalf("UpdateSettings(goal without year) error = %v, want ErrInvalidGoal", err)
	}
	settings, err := store.UpdateSettings(ctx, "usr_a", true, 2026, 12)
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if !settings.ActivityVisible || settings.GoalYear != 2026 || settings.GoalBooks != 12 {
		t.Fatalf("UpdateSettings() = %#v", settings)
	}
	// goalBooks == 0 clears the goal regardless of goalYear.
	settings, err = store.UpdateSettings(ctx, "usr_a", true, 0, 0)
	if err != nil {
		t.Fatalf("UpdateSettings(clear) error = %v", err)
	}
	if settings.GoalYear != 0 || settings.GoalBooks != 0 {
		t.Fatalf("UpdateSettings(clear) = %#v, want zeroed goal", settings)
	}
}

func TestConcurrentSendRequestKeepsSingleRow(t *testing.T) {
	store, db := testSocialStore(t)
	defer db.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.SendRequest(ctx, "usr_a", "usr_b")
		}()
	}
	wg.Wait()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM friendships WHERE user_id = 'usr_a' AND friend_id = 'usr_b'`).Scan(&count); err != nil {
		t.Fatalf("count friendships: %v", err)
	}
	if count != 1 {
		t.Fatalf("friendship rows = %d, want exactly 1", count)
	}
}

func testSocialStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	for _, id := range []string{"usr_a", "usr_b", "usr_c"} {
		if _, err := db.Exec(`
			INSERT INTO users (id, email, display_name, password_hash, role, created_at)
			VALUES (?, ?, ?, 'unused', 'reader', ?)
		`, id, id+"@example.com", id, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)); err != nil {
			db.Close()
			t.Fatalf("seed user %s: %v", id, err)
		}
	}
	return NewStore(db), db
}
