package httpapi

import "github.com/go-chi/chi/v5"

// mountTaskRoutes mounts the /tasks subtree onto r. Populated in Task 5 of
// the phase 06 plan. Until the handlers land this is a no-op so the rest of
// the surface stays exercisable.
func (s *Server) mountTaskRoutes(r chi.Router) {}
