package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

func updateProspectWorkflow(ctx context.Context, tx pgx.Tx, prospect string, newFlow any) error {
	encoded, e := json.Marshal(newFlow)
	if e != nil {
		return e
	}
	var replacement []map[string]any
	if json.Unmarshal(encoded, &replacement) != nil || len(replacement) == 0 {
		return errors.New("workflow must contain steps")
	}
	rows, e := tx.Query(ctx, `SELECT id,step,workflow FROM enrollments WHERE prospect_id=$1 AND status='active' FOR UPDATE`, prospect)
	if e != nil {
		return e
	}
	type enrollment struct {
		id   string
		step int
		raw  []byte
	}
	var all []enrollment
	for rows.Next() {
		var v enrollment
		if e = rows.Scan(&v.id, &v.step, &v.raw); e != nil {
			rows.Close()
			return e
		}
		all = append(all, v)
	}
	rows.Close()
	for _, v := range all {
		var previous []map[string]any
		json.Unmarshal(v.raw, &previous)
		if v.step > len(replacement) {
			return errors.New("cannot remove already executed workflow steps")
		}
		copy(replacement[:v.step], previous[:v.step])
		b, _ := json.Marshal(replacement)
		if _, e = tx.Exec(ctx, `UPDATE enrollments SET workflow=$2 WHERE id=$1`, v.id, b); e != nil {
			return e
		}
		if v.step < len(replacement) {
			step, _ := json.Marshal(replacement[v.step])
			if _, e = tx.Exec(ctx, `UPDATE jobs SET payload=jsonb_set(payload,'{step}',$2::jsonb) WHERE payload->>'enrollment_id'=$1 AND status IN ('pending','awaiting_approval','blocked')`, v.id, step); e != nil {
				return e
			}
		}
	}
	return nil
}
