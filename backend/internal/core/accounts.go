package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tullips/tullips/backend/internal/integrations"
	"net/http"
)

func (s *Server) accountData(ctx context.Context, ws, id string) (map[string]any, map[string]any, error) {
	var raw []byte
	e := s.DB.QueryRow(ctx, `SELECT data FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='accounts'`, id, ws).Scan(&raw)
	if e != nil {
		return nil, nil, e
	}
	var data map[string]any
	json.Unmarshal(raw, &data)
	secret := map[string]any{}
	if ciphertext := str(data, "secrets"); ciphertext != "" {
		plain, e := Decrypt(s.EncryptionKey, ciphertext)
		if e != nil {
			return nil, nil, e
		}
		if e = json.Unmarshal([]byte(plain), &secret); e != nil {
			return nil, nil, e
		}
	}
	return data, secret, nil
}
func (s *Server) saveAccount(ctx context.Context, id string, data, secret map[string]any) error {
	b, _ := json.Marshal(secret)
	sealed, e := Encrypt(s.EncryptionKey, string(b))
	if e != nil {
		return e
	}
	data["secrets"] = sealed
	raw, _ := json.Marshal(data)
	_, e = s.DB.Exec(ctx, `UPDATE records SET data=$2,updated_at=now() WHERE id=$1`, id, raw)
	return e
}
func (s *Server) connectAccount(w http.ResponseWriter, r *http.Request, ws, id, action string) {
	if r.Method != "POST" {
		fail(w, 405, "method not allowed")
		return
	}
	d, e := decode(w, r)
	if e != nil {
		fail(w, 400, "invalid JSON")
		return
	}
	data, secret, e := s.accountData(r.Context(), ws, id)
	if e != nil {
		s.result(w, e, nil)
		return
	}
	if str(data, "channel") == "linkedin" && action == "sync" {
		n, err := s.SyncLinkedIn(r.Context(), ws, id)
		s.result(w, err, map[string]int{"synced": n})
		return
	}
	if str(data, "channel") == "email" && action == "sync" {
		n, err := s.SyncMail(r.Context(), ws, id)
		s.result(w, err, map[string]int{"synced": n})
		return
	}
	if str(data, "channel") == "email" && action == "connect" {
		if password := str(d, "password"); password != "" {
			secret["password"] = password
		}
		m, err := s.mailbox(r.Context(), id, data, secret)
		if err == nil {
			err = m.Check(r.Context())
		}
		if err != nil {
			fail(w, 502, "mailbox connection failed")
			return
		}
		data["status"] = "connected"
		err = s.saveAccount(r.Context(), id, data, secret)
		s.result(w, err, map[string]string{"status": "connected"})
		return
	}
	if str(data, "channel") != "linkedin" {
		fail(w, 400, "LinkedIn account required")
		return
	}
	client, e := integrations.NewLinkedIn()
	if e != nil {
		fail(w, 502, "could not initialize LinkedIn client")
		return
	}
	if session := str(secret, "session"); session != "" {
		if e = client.Restore([]byte(session)); e != nil {
			fail(w, 400, "invalid stored session")
			return
		}
	}
	switch action {
	case "connect":
		password := str(d, "password")
		if password == "" {
			password = str(secret, "password")
		}
		if password == "" || str(data, "email") == "" {
			fail(w, 400, "email and password required")
			return
		}
		secret["password"] = password
		e = client.Login(r.Context(), str(data, "email"), password)
	case "verify":
		if str(d, "code") == "" {
			fail(w, 400, "verification code required")
			return
		}
		e = client.Verify(r.Context(), str(secret, "checkpoint"), str(d, "code"))
	default:
		fail(w, 404, "not found")
		return
	}
	session, _ := client.Session()
	secret["session"] = string(session)
	if errors.Is(e, integrations.ErrCheckpoint) {
		data["status"] = "verification_required"
		secret["checkpoint"] = client.CheckpointURL()
	} else if e != nil {
		data["status"] = "disconnected"
	} else {
		data["status"] = "connected"
		if identity, identityErr := client.Identity(r.Context()); identityErr == nil {
			data["mailbox_urn"] = identity
		} else {
			data["status"] = "identity_required"
		}
		delete(secret, "checkpoint")
	}
	if saveErr := s.saveAccount(r.Context(), id, data, secret); saveErr != nil {
		s.result(w, saveErr, nil)
		return
	}
	if e != nil && !errors.Is(e, integrations.ErrCheckpoint) {
		fail(w, 502, "LinkedIn connection failed; verify credentials or reconnect after checking LinkedIn")
		return
	}
	JSON(w, 200, map[string]any{"id": id, "status": data["status"]})
}
func (s *Server) AccountData(ctx context.Context, ws, id string) (map[string]any, map[string]any, error) {
	return s.accountData(ctx, ws, id)
}
func (s *Server) SaveAccount(ctx context.Context, id string, data, secret map[string]any) error {
	return s.saveAccount(ctx, id, data, secret)
}

// reconnect uses the account's retained encrypted password once; challenges always return to the user.
func (s *Server) reconnect(ctx context.Context, id string, data, secret map[string]any, client *integrations.LinkedIn) error {
	if str(secret, "password") == "" {
		return integrations.ErrCheckpoint
	}
	e := client.Login(ctx, str(data, "email"), str(secret, "password"))
	session, _ := client.Session()
	secret["session"] = string(session)
	if errors.Is(e, integrations.ErrCheckpoint) {
		data["status"] = "verification_required"
		secret["checkpoint"] = client.CheckpointURL()
	} else if e != nil {
		data["status"] = "disconnected"
	} else {
		data["status"] = "connected"
	}
	if saveErr := s.saveAccount(ctx, id, data, secret); saveErr != nil {
		return saveErr
	}
	return e
}
