package comment_test

// GitLab MR comment poster tests. Mirrors the GitHub stub-server
// pattern: an httptest server impersonates GitLab's REST API so the
// suite never reaches gitlab.com.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c3xdev/c3x/internal/comment"
)

func TestAutoDetectGitLabFromCIEnv(t *testing.T) {
	t.Setenv("CI_PROJECT_ID", "12345")
	t.Setenv("CI_MERGE_REQUEST_IID", "42")
	t.Setenv("CI_API_V4_URL", "https://gitlab.internal/api/v4")
	target, baseURL, err := comment.AutoDetectGitLab()
	if err != nil {
		t.Fatalf("AutoDetectGitLab: %v", err)
	}
	if target.ProjectID != "12345" || target.MR != 42 {
		t.Errorf("target = %+v", target)
	}
	if baseURL != "https://gitlab.internal/api/v4" {
		t.Errorf("baseURL = %q, want self-hosted from env", baseURL)
	}
}

func TestAutoDetectGitLabFallsBackToCloudBaseURL(t *testing.T) {
	t.Setenv("CI_PROJECT_ID", "1")
	t.Setenv("CI_MERGE_REQUEST_IID", "1")
	t.Setenv("CI_API_V4_URL", "")
	_, baseURL, err := comment.AutoDetectGitLab()
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != comment.DefaultGitLabBaseURL {
		t.Errorf("expected default gitlab.com base URL, got %q", baseURL)
	}
}

func TestAutoDetectGitLabRejectsNonMRPipeline(t *testing.T) {
	t.Setenv("CI_PROJECT_ID", "1")
	t.Setenv("CI_MERGE_REQUEST_IID", "")
	if _, _, err := comment.AutoDetectGitLab(); err == nil {
		t.Error("expected error when not on an MR pipeline")
	}
}

func TestGitLabPosterRejectsEmptyToken(t *testing.T) {
	_, err := comment.NewGitLabPoster("", "", comment.GitLabTarget{ProjectID: "1", MR: 1})
	if err == nil {
		t.Error("expected error on empty token")
	}
}

func TestGitLabPosterRejectsIncompleteTarget(t *testing.T) {
	_, err := comment.NewGitLabPoster("tok", "", comment.GitLabTarget{})
	if err == nil {
		t.Error("expected error on zero target")
	}
}

func TestGitLabPostCreatesNoteWhenNoneExists(t *testing.T) {
	var posts, gets int
	var createdBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/notes"):
			gets++
			_, _ = io.WriteString(w, "[]")
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			posts++
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdBody = body.Body
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":99}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	defer srv.Close()

	poster, err := comment.NewGitLabPoster("tok", srv.URL,
		comment.GitLabTarget{ProjectID: "acme%2Fwidgets", MR: 7})
	if err != nil {
		t.Fatal(err)
	}
	if err := poster.Post(context.Background(), "hello"); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if gets != 1 || posts != 1 {
		t.Errorf("expected 1 GET + 1 POST, got %d / %d", gets, posts)
	}
	if !strings.Contains(createdBody, comment.Marker) {
		t.Errorf("posted body missing marker: %q", createdBody)
	}
}

func TestGitLabPostUpdatesExistingMarkedNote(t *testing.T) {
	var put bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/notes"):
			body := comment.Marker + "\n(stale)"
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 17, "body": body},
			})
		case r.Method == http.MethodPut:
			put = true
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":17}`)
		case r.Method == http.MethodPost:
			t.Errorf("created a new note; expected an update via PUT")
			w.WriteHeader(http.StatusConflict)
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	defer srv.Close()

	poster, _ := comment.NewGitLabPoster("tok", srv.URL,
		comment.GitLabTarget{ProjectID: "1", MR: 1})
	if err := poster.Post(context.Background(), "fresh"); err != nil {
		t.Fatal(err)
	}
	if !put {
		t.Error("expected PUT against the marked existing note")
	}
}
