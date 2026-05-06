package web

import (
	"net/http"

	"github.com/nassiharel/klim/internal/recommend"
	"github.com/nassiharel/klim/internal/registry"
)

// pageForYou renders the For You recommendations using the shared
// scoring algorithm in internal/recommend (also used by the TUI).
func (s *Server) pageForYou(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	recs := recommend.Compute(tools)
	favs, _ := s.loader.Favorites()
	if favs == nil {
		favs = map[string]bool{}
	}
	s.renderPage(w, r, "foryou.html", pageData{
		Title:     "For You",
		ActiveTab: "foryou",
		Data: forYouView{
			Recommendations: recs,
			Tools:           tools,
			Favorites:       favs,
			Total:           len(recs),
		},
	})
}

// forYouView wraps the shared recommendations along with the tools
// slice the template needs to look up display fields by index, and
// the user's current favorites for the toggle column.
type forYouView struct {
	Recommendations []recommend.Recommendation
	// Tools is the same slice Compute received, so the template can
	// resolve recommendation.ToolIdx → registry.Tool.
	Tools     []registry.Tool
	Favorites map[string]bool
	Total     int
}
