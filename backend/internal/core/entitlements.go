package core

import (
	"context"
	"os"
)

func (s *Server) PaidActive(ctx context.Context, ws string) bool {
	if os.Getenv("DEPLOYMENT_MODE") != "saas" {
		return true
	}
	var status string
	if s.DB.QueryRow(ctx, `SELECT status FROM billing_accounts WHERE workspace_id=$1`, ws).Scan(&status) != nil {
		return false
	}
	return status == "active" || status == "trialing"
}
