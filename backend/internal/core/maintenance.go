package core

import "context"

// Fail closed when maintenance state cannot be read.
func (s *Server) Maintenance(ctx context.Context) bool {
	var active bool
	err := s.DB.QueryRow(ctx, `SELECT maintenance FROM installation WHERE id`).Scan(&active)
	return err != nil || active
}
