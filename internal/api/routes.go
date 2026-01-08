package api

func (s *Server) routes() {
	s.mux.HandleFunc("POST /sync/trigger", s.handleSyncTrigger())
}
