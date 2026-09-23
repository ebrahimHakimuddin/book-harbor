package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// clientHeader is how the app and admin console name themselves: "<client>/<version>",
// such as "android/1.0.0" or "admin/1.0.0".
const clientHeader = "X-BookHarbor-Client"

// withVersionCheck refuses API requests from a client whose version doesn't match the
// server's (same major and minor), with 426 and which side to update. GET /instance stays
// open so a client can learn the server's version and say so. Requests without the header
// (scripts, older apps) and development builds are let through.
func (s *server) withVersionCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/instance" {
			if client, version, ok := strings.Cut(r.Header.Get(clientHeader), "/"); ok && !compatibleVersions(s.build.Version, version) {
				writeJSON(w, http.StatusUpgradeRequired, map[string]any{
					"code":    "version_mismatch",
					"message": versionMismatchMessage(client, s.build.Version, version),
					"details": map[string]string{"client": client, "clientVersion": version, "serverVersion": s.build.Version},
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// majorMinor reads "1.2.3" as (1, 2); ok is false for anything else, such as "dev".
func majorMinor(version string) (int, int, bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	return major, minor, err1 == nil && err2 == nil
}

// compatibleVersions is true when both share a major and minor version, or either one isn't
// a release version (a development build).
func compatibleVersions(server, client string) bool {
	sMajor, sMinor, sOK := majorMinor(server)
	cMajor, cMinor, cOK := majorMinor(client)
	return !sOK || !cOK || (sMajor == cMajor && sMinor == cMinor)
}

func versionMismatchMessage(client, server, version string) string {
	name := "app"
	if client == "admin" {
		name = "admin console"
	}
	sMajor, sMinor, _ := majorMinor(server)
	cMajor, cMinor, _ := majorMinor(version)
	if cMajor < sMajor || (cMajor == sMajor && cMinor < sMinor) {
		return "This " + name + " is version " + version + " but the server is " + server + ". Update the " + name + " to use this server."
	}
	return "This " + name + " is version " + version + " but the server is " + server + ". Update the server before using this " + name + "."
}
