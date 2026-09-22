package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
)

func postJSON(handler http.Handler, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestPasswordResetByEmailedCode(t *testing.T) {
	mailer := &fakeMailer{configured: true, bodies: make(chan string, 4)}
	handler, _ := testHandlerWithMailer(t, nil, mailer)
	bootstrapAdministrator(t, handler)
	old := login(t, handler, "admin@example.com", "a secure first password")

	// Unknown addresses get the same answer and no email.
	if r := postJSON(handler, "/api/v1/password-resets", `{"email":"nobody@example.com"}`); r.Code != http.StatusAccepted {
		t.Fatalf("unknown email status = %d", r.Code)
	}
	if r := postJSON(handler, "/api/v1/password-resets", `{"email":"admin@example.com"}`); r.Code != http.StatusAccepted {
		t.Fatalf("request status = %d; %s", r.Code, r.Body)
	}
	var body string
	select {
	case body = <-mailer.bodies:
	case <-time.After(5 * time.Second):
		t.Fatal("no reset email sent")
	}
	code := regexp.MustCompile(`<strong>([A-Z2-9]{10})</strong>`).FindStringSubmatch(body)
	if code == nil {
		t.Fatalf("no code in email: %s", body)
	}
	// A second request right away is throttled: no new email, same answer.
	if r := postJSON(handler, "/api/v1/password-resets", `{"email":"admin@example.com"}`); r.Code != http.StatusAccepted {
		t.Fatalf("repeat request status = %d", r.Code)
	}

	if r := postJSON(handler, "/api/v1/password-resets/confirm", `{"email":"admin@example.com","code":"WRONGWRONG","newPassword":"a brand new password"}`); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong code status = %d", r.Code)
	}
	confirm := `{"email":"admin@example.com","code":"` + code[1] + `","newPassword":"a brand new password"}`
	if r := postJSON(handler, "/api/v1/password-resets/confirm", confirm); r.Code != http.StatusNoContent {
		t.Fatalf("confirm status = %d; %s", r.Code, r.Body)
	}
	if r := postJSON(handler, "/api/v1/password-resets/confirm", confirm); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reused code status = %d", r.Code)
	}
	login(t, handler, "admin@example.com", "a brand new password")
	// Existing sessions were signed out.
	if r := adminCall(t, handler, old.AccessToken, http.MethodGet, "/api/v1/me", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("old session status = %d", r.Code)
	}
	select {
	case extra := <-mailer.bodies:
		t.Fatalf("throttled request still sent an email: %s", extra)
	default:
	}
}

func TestPasswordResetBurnsCodeAfterTooManyGuesses(t *testing.T) {
	mailer := &fakeMailer{configured: true, bodies: make(chan string, 1)}
	handler, _ := testHandlerWithMailer(t, nil, mailer)
	bootstrapAdministrator(t, handler)
	postJSON(handler, "/api/v1/password-resets", `{"email":"admin@example.com"}`)
	code := regexp.MustCompile(`<strong>([A-Z2-9]{10})</strong>`).FindStringSubmatch(<-mailer.bodies)[1]
	for i := 0; i < 5; i++ {
		postJSON(handler, "/api/v1/password-resets/confirm", `{"email":"admin@example.com","code":"AAAAAAAAAA","newPassword":"a brand new password"}`)
	}
	if r := postJSON(handler, "/api/v1/password-resets/confirm", `{"email":"admin@example.com","code":"`+code+`","newPassword":"a brand new password"}`); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("burned code status = %d", r.Code)
	}
}

func TestPasswordResetNeedsMail(t *testing.T) {
	handler, _ := testHandlerWithMailer(t, nil, &fakeMailer{configured: false})
	if r := postJSON(handler, "/api/v1/password-resets", `{"email":"a@example.com"}`); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", r.Code)
	}
}
