package api

func (s *Server) routes() {
	s.r.Get("/funds", s.handleFundsList())
	s.r.Get("/funds/rank", s.handleFundsRank())
	s.r.Get("/funds/{code}", s.handleFundDetails())
	s.r.Get("/funds/{code}/analytics", s.handleFundAnalytics())
	s.r.Post("/sync/trigger", s.handleSyncTrigger())
	s.r.Get("/sync/status", s.handleSyncStatus())
}
