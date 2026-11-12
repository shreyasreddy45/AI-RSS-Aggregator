package handlers

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/aiclient"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

// maxAIRequestBody caps request bodies forwarded to the AI service.
const maxAIRequestBody = 1 << 20

// forwardToAI sends a request to the Python AI service on behalf of the
// authenticated user and relays the upstream status and JSON body unchanged.
//
// The Go API remains the only public entry point: it validates the JWT first,
// and the AI service trusts the X-User-ID header because the call is also
// authenticated with the shared internal token.
func forwardToAI(a *app.App, w http.ResponseWriter, r *http.Request, user database.User, method, path string, query url.Values, body []byte) {
	if a.AI == nil {
		jsonutil.RespondWithError(w, http.StatusServiceUnavailable, "AI features are not configured")
		return
	}

	ctx, cancel := requestContextWithTimeout(r, a.Config.AIRequestTimeout)
	defer cancel()

	resp, err := a.AI.Do(ctx, method, path, user.ID, query, body)
	if err != nil {
		a.Logger.Error("AI service request failed", "path", path, "error", err)
		if errors.Is(err, aiclient.ErrUnavailable) {
			jsonutil.RespondWithError(w, http.StatusServiceUnavailable, "AI service is unavailable")
			return
		}
		jsonutil.RespondWithError(w, http.StatusBadGateway, "AI service request failed")
		return
	}

	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
}

func readAIBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.Body == nil {
		return nil, true
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAIRequestBody))
	if err != nil {
		jsonutil.RespondWithError(w, http.StatusBadRequest, "request body is too large or unreadable")
		return nil, false
	}
	return data, true
}

// AskRAG answers a question from the user's articles. The retrieval and
// generation run in the Python AI service.
//
//	@Summary	Ask a question
//	@Tags		RAG
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		q		query		string	false	"Question query"
//	@Param		request	body		object	false	"JSON request body: {\"query\": \"...\"}"
//	@Success	200		{object}	object
//	@Failure	400		{object}	map[string]string
//	@Failure	503		{object}	map[string]string
//	@Router		/ask [get]
//	@Router		/ask [post]
func AskRAG(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		if r.Method == http.MethodGet {
			forwardToAI(a, w, r, user, http.MethodGet, "/ask", r.URL.Query(), nil)
			return
		}
		body, ok := readAIBody(w, r)
		if !ok {
			return
		}
		forwardToAI(a, w, r, user, http.MethodPost, "/ask", r.URL.Query(), body)
	}
}

// AgentChat runs a query through the AI service's tool-using agent.
//
//	@Summary	Chat with AI agent
//	@Tags		Agent
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		object	true	"Agent query: {\"query\": \"...\"}"
//	@Success	200		{object}	object
//	@Failure	400		{object}	map[string]string
//	@Failure	503		{object}	map[string]string
//	@Router		/agent/chat [post]
func AgentChat(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		body, ok := readAIBody(w, r)
		if !ok {
			return
		}
		forwardToAI(a, w, r, user, http.MethodPost, "/agent/chat", r.URL.Query(), body)
	}
}

// GeneratePostEmbedding (re)generates the embedding for a post.
//
//	@Summary	Generate post embedding
//	@Tags		Posts
//	@Security	BearerAuth
//	@Produce	json
//	@Param		postID	path		string	true	"Post ID"
//	@Success	200		{object}	object
//	@Failure	400		{object}	map[string]string
//	@Failure	404		{object}	map[string]string
//	@Failure	503		{object}	map[string]string
//	@Router		/posts/{postID}/embeddings [post]
func GeneratePostEmbedding(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, "postID must be a valid UUID")
			return
		}
		forwardToAI(a, w, r, user, http.MethodPost, "/posts/"+postID.String()+"/embeddings", nil, nil)
	}
}
