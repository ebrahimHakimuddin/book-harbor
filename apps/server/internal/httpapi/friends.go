package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
	"github.com/bookharbor/bookharbor/apps/server/internal/reading"
	"github.com/bookharbor/bookharbor/apps/server/internal/social"
)

const friendActivityLimit = 5

type friendActivityBookResponse struct {
	BookID     string  `json:"bookId"`
	Title      string  `json:"title"`
	CoverURL   string  `json:"coverUrl"`
	Percentage float64 `json:"percentage"`
	UpdatedAt  string  `json:"updatedAt"`
}

type friendResponse struct {
	UserID           string                       `json:"userId"`
	DisplayName      string                       `json:"displayName"`
	Email            string                       `json:"email"`
	ActivityVisible  bool                         `json:"activityVisible"`
	CurrentlyReading []friendActivityBookResponse `json:"currentlyReading"`
	FinishedThisYear *int                         `json:"finishedThisYear"`
	GoalBooks        *int                         `json:"goalBooks"`
}

type friendRequestResponse struct {
	UserID      string    `json:"userId"`
	DisplayName string    `json:"displayName"`
	Email       string    `json:"email"`
	Direction   string    `json:"direction"`
	CreatedAt   time.Time `json:"createdAt"`
}

type socialSettingsResponse struct {
	ActivityVisible  bool `json:"activityVisible"`
	GoalYear         int  `json:"goalYear"`
	GoalBooks        int  `json:"goalBooks"`
	FinishedThisYear int  `json:"finishedThisYear"`
}

// friends handles GET /api/v1/friends: the caller's accepted friends, each with as
// much current activity as the friend has opted to share.
func (s *server) friends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, "GET")
		return
	}
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	friendships, err := s.social.ListFriends(r.Context(), principal.User.ID)
	if err != nil {
		s.logger.Error("list friends", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list friends")
		return
	}

	items := make([]friendResponse, 0, len(friendships))
	for _, friendship := range friendships {
		response, err := s.newFriendResponse(r, principal.User.ID, friendship.FriendID)
		if err != nil {
			s.logger.Error("build friend response", "error", err, "friendId", friendship.FriendID)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list friends")
			return
		}
		items = append(items, response)
	}
	writeJSON(w, http.StatusOK, struct {
		Items []friendResponse `json:"items"`
	}{Items: items})
}

func (s *server) newFriendResponse(r *http.Request, viewerID, friendID string) (friendResponse, error) {
	friendUser, err := s.users.GetUser(r.Context(), friendID)
	if err != nil {
		return friendResponse{}, err
	}
	settings, err := s.social.GetSettings(r.Context(), friendID)
	if err != nil {
		return friendResponse{}, err
	}
	response := friendResponse{
		UserID: friendUser.ID, DisplayName: friendUser.DisplayName, Email: friendUser.Email,
		ActivityVisible: settings.ActivityVisible, CurrentlyReading: []friendActivityBookResponse{},
	}
	canView, err := s.social.CanViewActivity(r.Context(), viewerID, friendID)
	if err != nil {
		return friendResponse{}, err
	}
	if !canView {
		return response, nil
	}

	snapshot, err := s.reading.SnapshotForUser(r.Context(), friendID, 20)
	if err != nil {
		return friendResponse{}, err
	}
	for _, progress := range snapshot {
		if progress.Percentage <= 0 || progress.Percentage >= reading.FinishedThreshold {
			continue
		}
		if len(response.CurrentlyReading) >= friendActivityLimit {
			break
		}
		book, err := s.library.Get(r.Context(), progress.BookID)
		if err != nil {
			continue // the book may have been deleted since this progress was recorded
		}
		response.CurrentlyReading = append(response.CurrentlyReading, friendActivityBookResponse{
			BookID: book.ID, Title: book.Title, CoverURL: book.CoverURL,
			Percentage: progress.Percentage, UpdatedAt: progress.OccurredAt.Format(time.RFC3339Nano),
		})
	}

	finished, err := s.reading.FinishedCount(r.Context(), friendID, time.Now().UTC().Year())
	if err != nil {
		return friendResponse{}, err
	}
	response.FinishedThisYear = &finished
	if settings.GoalBooks > 0 && settings.GoalYear == time.Now().UTC().Year() {
		goal := settings.GoalBooks
		response.GoalBooks = &goal
	}
	return response, nil
}

// friendRequests handles GET (list) and POST (send) on /api/v1/friends/requests.
func (s *server) friendRequests(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listFriendRequests(w, r, principal.User.ID)
	case http.MethodPost:
		s.sendFriendRequest(w, r, principal.User)
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

func (s *server) listFriendRequests(w http.ResponseWriter, r *http.Request, userID string) {
	incoming, err := s.social.ListIncomingRequests(r.Context(), userID)
	if err != nil {
		s.logger.Error("list incoming friend requests", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list friend requests")
		return
	}
	outgoing, err := s.social.ListOutgoingRequests(r.Context(), userID)
	if err != nil {
		s.logger.Error("list outgoing friend requests", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list friend requests")
		return
	}
	incomingResponses, err := s.newFriendRequestResponses(r, incoming, "incoming")
	if err != nil {
		s.logger.Error("build incoming friend requests", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list friend requests")
		return
	}
	outgoingResponses, err := s.newFriendRequestResponses(r, outgoing, "outgoing")
	if err != nil {
		s.logger.Error("build outgoing friend requests", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list friend requests")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Incoming []friendRequestResponse `json:"incoming"`
		Outgoing []friendRequestResponse `json:"outgoing"`
	}{Incoming: incomingResponses, Outgoing: outgoingResponses})
}

func (s *server) newFriendRequestResponses(r *http.Request, requests []social.FriendRequest, direction string) ([]friendRequestResponse, error) {
	responses := make([]friendRequestResponse, 0, len(requests))
	for _, request := range requests {
		user, err := s.users.GetUser(r.Context(), request.OtherUserID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, friendRequestResponse{
			UserID: user.ID, DisplayName: user.DisplayName, Email: user.Email,
			Direction: direction, CreatedAt: request.CreatedAt,
		})
	}
	return responses, nil
}

func (s *server) sendFriendRequest(w http.ResponseWriter, r *http.Request, from identity.User) {
	var request struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	target, err := s.users.FindByEmail(r.Context(), request.Email)
	if errors.Is(err, identity.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "friend_not_found", "no account uses that email address")
		return
	}
	if err != nil {
		s.logger.Error("look up friend request target", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to send friend request")
		return
	}
	if err := s.social.SendRequest(r.Context(), from.ID, target.ID); err != nil {
		s.writeSocialError(w, err)
		return
	}
	s.record(r, "friend.request", "user", target.ID, target.Email)
	writeJSON(w, http.StatusCreated, struct {
		UserID      string `json:"userId"`
		DisplayName string `json:"displayName"`
	}{UserID: target.ID, DisplayName: target.DisplayName})
}

// friendRequestAction handles the /api/v1/friends/requests/ prefix:
// POST .../{userID}/accept, and DELETE .../{userID} (decline or cancel).
func (s *server) friendRequestAction(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/friends/requests/")
	if otherUserID, isAccept := strings.CutSuffix(rest, "/accept"); isAccept {
		if otherUserID == "" || strings.Contains(otherUserID, "/") {
			notFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, "POST")
			return
		}
		if err := s.social.AcceptRequest(r.Context(), principal.User.ID, otherUserID); err != nil {
			s.writeSocialError(w, err)
			return
		}
		s.record(r, "friend.accept", "user", otherUserID, "")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	otherUserID := rest
	if otherUserID == "" || strings.Contains(otherUserID, "/") {
		notFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w, "DELETE")
		return
	}
	// The caller could be declining a request they received or cancelling one they sent;
	// try both since either is a legitimate reason to remove the same pending row.
	action := "friend.decline"
	err := s.social.DeclineRequest(r.Context(), principal.User.ID, otherUserID)
	if errors.Is(err, social.ErrRequestNotFound) {
		action = "friend.cancel"
		err = s.social.CancelRequest(r.Context(), principal.User.ID, otherUserID)
	}
	if err != nil {
		s.writeSocialError(w, err)
		return
	}
	s.record(r, action, "user", otherUserID, "")
	w.WriteHeader(http.StatusNoContent)
}

// friend handles DELETE /api/v1/friends/{userID}: unfriend.
func (s *server) friend(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	friendID, ok := singlePathValue(r.URL.Path, "/api/v1/friends/")
	if !ok {
		notFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		s.friendProfile(w, r, principal.User.ID, friendID)
		return
	}
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w, "GET, DELETE")
		return
	}
	if err := s.social.RemoveFriend(r.Context(), principal.User.ID, friendID); err != nil {
		s.writeSocialError(w, err)
		return
	}
	s.record(r, "friend.remove", "user", friendID, "")
	w.WriteHeader(http.StatusNoContent)
}

// socialSettings handles GET and PUT on /api/v1/me/social-settings.
func (s *server) socialSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := s.social.GetSettings(r.Context(), principal.User.ID)
		if err != nil {
			s.logger.Error("get social settings", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to read settings")
			return
		}
		response, err := s.newSocialSettingsResponseWithStats(r, principal.User.ID, settings)
		if err != nil {
			s.logger.Error("get reading stats", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to read settings")
			return
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPut:
		var request struct {
			ActivityVisible bool `json:"activityVisible"`
			GoalYear        int  `json:"goalYear"`
			GoalBooks       int  `json:"goalBooks"`
		}
		if err := decodeJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
			return
		}
		settings, err := s.social.UpdateSettings(r.Context(), principal.User.ID, request.ActivityVisible, request.GoalYear, request.GoalBooks)
		if err != nil {
			s.writeSocialError(w, err)
			return
		}
		s.record(r, "social_settings.update", "user", principal.User.ID, "")
		response, err := s.newSocialSettingsResponseWithStats(r, principal.User.ID, settings)
		if err != nil {
			s.logger.Error("get reading stats", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to read settings")
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		writeMethodNotAllowed(w, "GET, PUT")
	}
}

func newSocialSettingsResponse(settings social.Settings) socialSettingsResponse {
	return socialSettingsResponse{ActivityVisible: settings.ActivityVisible, GoalYear: settings.GoalYear, GoalBooks: settings.GoalBooks}
}

func (s *server) newSocialSettingsResponseWithStats(r *http.Request, userID string, settings social.Settings) (socialSettingsResponse, error) {
	response := newSocialSettingsResponse(settings)
	finished, err := s.reading.FinishedCount(r.Context(), userID, time.Now().UTC().Year())
	if err != nil {
		return socialSettingsResponse{}, err
	}
	response.FinishedThisYear = finished
	return response, nil
}

func (s *server) writeSocialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, social.ErrSelfFriend):
		writeError(w, http.StatusBadRequest, "cannot_friend_self", "you cannot send yourself a friend request")
	case errors.Is(err, social.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "friend_not_found", "no account uses that email address")
	case errors.Is(err, social.ErrAlreadyFriends):
		writeError(w, http.StatusConflict, "already_friends", "you are already friends")
	case errors.Is(err, social.ErrRequestAlreadySent):
		writeError(w, http.StatusConflict, "request_already_sent", "a friend request is already pending")
	case errors.Is(err, social.ErrRequestNotFound):
		writeError(w, http.StatusNotFound, "request_not_found", "no matching friend request was found")
	case errors.Is(err, social.ErrFriendshipNotFound):
		writeError(w, http.StatusNotFound, "friendship_not_found", "you are not friends with that user")
	case errors.Is(err, social.ErrInvalidGoal):
		writeError(w, http.StatusUnprocessableEntity, "invalid_goal", "goal must be a positive number of books for a valid year")
	default:
		s.logger.Error("social request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to complete the request")
	}
}

type finishedBookResponse struct {
	BookID     string `json:"bookId"`
	Title      string `json:"title"`
	CoverURL   string `json:"coverUrl"`
	FinishedAt string `json:"finishedAt"`
}

type friendProfileResponse struct {
	friendResponse
	ReadingNow    []friendActivityBookResponse `json:"readingNow"`
	Finished      []finishedBookResponse       `json:"finished"`
	FinishedTotal *int                         `json:"finishedTotal"`
	// BooksInCommon counts books both people have started or finished.
	BooksInCommon *int `json:"booksInCommon"`
}

const friendProfileLimit = 50

// friendProfile serves GET /friends/{id}: a friend's reading, when they share it. Anyone who
// isn't an accepted friend gets 404, so the route can't be used to probe other accounts.
func (s *server) friendProfile(w http.ResponseWriter, r *http.Request, viewerID, friendID string) {
	friends, err := s.social.AreFriends(r.Context(), viewerID, friendID)
	if err != nil {
		s.logger.Error("check friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read friend")
		return
	}
	if !friends {
		notFound(w, r)
		return
	}
	base, err := s.newFriendResponse(r, viewerID, friendID)
	if err != nil {
		s.logger.Error("build friend profile", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read friend")
		return
	}
	profile := friendProfileResponse{friendResponse: base, ReadingNow: []friendActivityBookResponse{}, Finished: []finishedBookResponse{}}
	canView, err := s.social.CanViewActivity(r.Context(), viewerID, friendID)
	if err != nil || !canView {
		if err != nil {
			s.logger.Error("check activity visibility", "error", err)
		}
		writeJSON(w, http.StatusOK, profile)
		return
	}
	snapshot, err := s.reading.SnapshotForUser(r.Context(), friendID, 200)
	if err == nil {
		for _, progress := range snapshot {
			if progress.Percentage <= 0 || progress.Percentage >= reading.FinishedThreshold || len(profile.ReadingNow) >= friendProfileLimit {
				continue
			}
			if book, err := s.library.Get(r.Context(), progress.BookID); err == nil {
				profile.ReadingNow = append(profile.ReadingNow, friendActivityBookResponse{
					BookID: book.ID, Title: book.Title, CoverURL: book.CoverURL,
					Percentage: progress.Percentage, UpdatedAt: progress.OccurredAt.Format(time.RFC3339Nano),
				})
			}
		}
	}
	finished, total, err := s.reading.FinishedBooks(r.Context(), friendID, friendProfileLimit)
	if err == nil {
		profile.FinishedTotal = &total
		for _, item := range finished {
			if book, err := s.library.Get(r.Context(), item.BookID); err == nil {
				profile.Finished = append(profile.Finished, finishedBookResponse{
					BookID: book.ID, Title: book.Title, CoverURL: book.CoverURL, FinishedAt: item.FinishedAt.Format(time.RFC3339Nano),
				})
			}
		}
	}
	// ponytail: two bounded snapshots intersected in memory; fine at a household's library size.
	mine, err := s.reading.SnapshotForUser(r.Context(), viewerID, 1000)
	if err == nil {
		theirs, _ := s.reading.SnapshotForUser(r.Context(), friendID, 1000)
		started := make(map[string]bool, len(mine))
		for _, progress := range mine {
			if progress.Percentage > 0 {
				started[progress.BookID] = true
			}
		}
		common := 0
		for _, progress := range theirs {
			if progress.Percentage > 0 && started[progress.BookID] {
				common++
			}
		}
		profile.BooksInCommon = &common
	}
	writeJSON(w, http.StatusOK, profile)
}
