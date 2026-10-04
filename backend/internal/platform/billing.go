package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func stripeCall(ctx context.Context, path string, form url.Values, key string) (map[string]any, error) {
	method := "POST"
	if form == nil {
		method = "GET"
		form = url.Values{}
	}
	req, e := http.NewRequestWithContext(ctx, method, "https://api.stripe.com/v1/"+path, strings.NewReader(form.Encode()))
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth(os.Getenv("STRIPE_SECRET_KEY"), "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Stripe-Version", "2026-07-29.dahlia")
	resp, e := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if e != nil {
		return nil, errors.New("Stripe is unavailable")
	}
	defer resp.Body.Close()
	var data map[string]any
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data) != nil || resp.StatusCode >= 300 {
		return nil, errors.New("Stripe rejected the request; check your billing configuration")
	}
	return data, nil
}
func (s *Server) billingStatus(w http.ResponseWriter, r *http.Request) {
	user, restricted, _, e := s.Core.Authenticate(r)
	workspace := r.PathValue("workspace")
	if e != nil || restricted != "" || !s.Core.HasAccess(r.Context(), user, workspace, true) {
		fail(w, 403, "Only workspace owners can access billing")
		return
	}
	var status string
	var credits int64
	accountLimit := 3
	e = s.Core.DB.QueryRow(r.Context(), "SELECT status,credits,account_limit FROM billing_accounts WHERE workspace_id=$1", workspace).Scan(&status, &credits, &accountLimit)
	if e != nil {
		status = "inactive"
	}
	write(w, 200, map[string]any{"mode": env("DEPLOYMENT_MODE", "selfhosted"), "status": status, "credits": credits, "included_accounts": 3, "account_limit": accountLimit, "configured": os.Getenv("STRIPE_WORKSPACE_PRICE_ID") != "" && os.Getenv("STRIPE_SECRET_KEY") != ""})
}
func (s *Server) billing(w http.ResponseWriter, r *http.Request) {
	user, restricted, _, e := s.Core.Authenticate(r)
	workspace := r.PathValue("workspace")
	if e != nil || restricted != "" || !s.Core.HasAccess(r.Context(), user, workspace, true) {
		fail(w, 403, "Only workspace owners can manage billing")
		return
	}
	if os.Getenv("DEPLOYMENT_MODE") != "saas" || os.Getenv("STRIPE_SECRET_KEY") == "" {
		fail(w, 503, "Hosted billing is not configured")
		return
	}
	action := r.PathValue("action")
	if action != "checkout" && action != "portal" && action != "topup" && action != "accounts" {
		fail(w, 404, "Unknown billing action")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 {
		fail(w, 400, "An Idempotency-Key header is required")
		return
	}
	key = workspace + ":" + action + ":" + key
	var customer, status string
	s.Core.DB.QueryRow(r.Context(), "SELECT coalesce(customer_id,''),status FROM billing_accounts WHERE workspace_id=$1", workspace).Scan(&customer, &status)
	if customer == "" {
		c, err := stripeCall(r.Context(), "customers", url.Values{"metadata[workspace_id]": {workspace}}, "customer:"+workspace)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		customer, _ = c["id"].(string)
		if customer == "" {
			fail(w, 502, "Invalid customer response")
			return
		}
		if _, err = s.Core.DB.Exec(r.Context(), "INSERT INTO billing_accounts(workspace_id,customer_id) VALUES($1,$2) ON CONFLICT(workspace_id) DO UPDATE SET customer_id=excluded.customer_id", workspace, customer); err != nil {
			fail(w, 500, "Could not save billing account")
			return
		}
	}
	if action == "accounts" {
		var in struct {
			Additional int `json:"additional_accounts"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Additional < 0 || in.Additional > 100 {
			fail(w, 400, "Additional accounts must be between 0 and 100")
			return
		}
		var subscription string
		s.Core.DB.QueryRow(r.Context(), "SELECT subscription_id FROM billing_accounts WHERE workspace_id=$1", workspace).Scan(&subscription)
		if subscription == "" {
			fail(w, 409, "Subscribe before adding account slots")
			return
		}
		var used int
		s.Core.DB.QueryRow(r.Context(), "SELECT count(*) FROM records WHERE workspace_id=$1 AND kind='accounts' AND data->>'channel'='linkedin'", workspace).Scan(&used)
		if used > 3+in.Additional {
			fail(w, 409, "Remove extra LinkedIn accounts before reducing paid slots")
			return
		}
		price := os.Getenv("STRIPE_ACCOUNT_PRICE_ID")
		if price == "" {
			fail(w, 503, "Additional account price is not configured")
			return
		}
		sub, err := stripeCall(r.Context(), "subscriptions/"+url.PathEscape(subscription), nil, key+":read")
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		itemID := ""
		if items, ok := sub["items"].(map[string]any); ok {
			if data, ok := items["data"].([]any); ok {
				for _, v := range data {
					item, _ := v.(map[string]any)
					p, _ := item["price"].(map[string]any)
					if p["id"] == price {
						itemID, _ = item["id"].(string)
					}
				}
			}
		}
		if itemID == "" && in.Additional == 0 {
			write(w, 200, map[string]bool{"updated": true})
			return
		}
		f := url.Values{"proration_behavior": {"always_invoice"}, "payment_behavior": {"pending_if_incomplete"}, "items[0][quantity]": {strconv.Itoa(in.Additional)}}
		if itemID != "" {
			f.Set("items[0][id]", itemID)
		} else {
			f.Set("items[0][price]", price)
		}
		if _, err = stripeCall(r.Context(), "subscriptions/"+url.PathEscape(subscription), f, key); err != nil {
			fail(w, 502, err.Error())
			return
		}
		write(w, 200, map[string]any{"updated": true, "message": "Account allowance updates after Stripe confirms the subscription change."})
		return
	}
	form := url.Values{"customer": {customer}}
	path := "checkout/sessions"
	if action == "portal" {
		path = "billing_portal/sessions"
		form.Set("return_url", s.Base+"/app/settings")
	} else {
		price := os.Getenv("STRIPE_WORKSPACE_PRICE_ID")
		mode := "subscription"
		if action == "topup" {
			price = os.Getenv("STRIPE_TOPUP_PRICE_ID")
			mode = "payment"
		}
		if price == "" {
			fail(w, 503, "Prices must be configured before checkout")
			return
		}
		if action == "checkout" && (status == "active" || status == "trialing") {
			fail(w, 409, "Manage the existing subscription in the billing portal")
			return
		}
		form.Set("mode", mode)
		form.Set("integration_identifier", "tullips-basgaeyd")
		form.Set("line_items[0][price]", price)
		form.Set("line_items[0][quantity]", "1")
		form.Set("success_url", s.Base+"/app/settings?checkout=complete")
		form.Set("cancel_url", s.Base+"/app/settings")
		form.Set("client_reference_id", workspace)
		form.Set("metadata[workspace_id]", workspace)
		form.Set("metadata[kind]", action)
		if action == "checkout" {
			form.Set("subscription_data[metadata][workspace_id]", workspace)
			var count int
			s.Core.DB.QueryRow(r.Context(), "SELECT count(*) FROM records WHERE workspace_id=$1 AND kind='accounts' AND data->>'channel'='linkedin'", workspace).Scan(&count)
			if count > 3 {
				extra := os.Getenv("STRIPE_ACCOUNT_PRICE_ID")
				if extra == "" {
					fail(w, 503, "Additional account price is not configured")
					return
				}
				form.Set("line_items[1][price]", extra)
				form.Set("line_items[1][quantity]", strconv.Itoa(count-3))
			}
		}
	}
	result, err := stripeCall(r.Context(), path, form, key)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	write(w, 200, map[string]any{"url": result["url"]})
}
func verifyStripe(payload []byte, header, secret string, now time.Time) bool {
	if secret == "" {
		return false
	}
	var timestamp string
	var signatures []string
	for _, p := range strings.Split(header, ",") {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] == "t" {
			timestamp = kv[1]
		}
		if kv[0] == "v1" {
			signatures = append(signatures, kv[1])
		}
	}
	t, e := strconv.ParseInt(timestamp, 10, 64)
	if e != nil || now.Unix()-t > 300 || t-now.Unix() > 300 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(payload)
	expected := mac.Sum(nil)
	for _, sig := range signatures {
		b, e := hex.DecodeString(sig)
		if e == nil && hmac.Equal(b, expected) {
			return true
		}
	}
	return false
}
func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil || !verifyStripe(body, r.Header.Get("Stripe-Signature"), os.Getenv("STRIPE_WEBHOOK_SECRET"), time.Now()) {
		fail(w, 400, "Invalid webhook signature")
		return
	}
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &event) != nil || event.ID == "" {
		fail(w, 400, "Invalid event")
		return
	}
	var object struct {
		ID            string `json:"id"`
		Customer      string `json:"customer"`
		Subscription  string `json:"subscription"`
		Status        string `json:"status"`
		PaymentStatus string `json:"payment_status"`
		BillingReason string `json:"billing_reason"`
		Items         struct {
			Data []struct {
				Quantity int `json:"quantity"`
				Price    struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
		Metadata map[string]string `json:"metadata"`
	}
	if json.Unmarshal(event.Data.Object, &object) != nil {
		fail(w, 400, "Invalid event object")
		return
	}
	tx, e := s.Core.DB.Begin(r.Context())
	if e != nil {
		fail(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	result, e := tx.Exec(r.Context(), "INSERT INTO stripe_events(id) VALUES($1) ON CONFLICT DO NOTHING", event.ID)
	if e != nil {
		fail(w, 500, "Could not record event")
		return
	}
	if result.RowsAffected() == 0 {
		write(w, 200, map[string]bool{"received": true})
		return
	}
	switch event.Type {
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		status := object.Status
		if event.Type == "customer.subscription.deleted" {
			status = "canceled"
		}
		limit := 3
		for _, item := range object.Items.Data {
			if item.Price.ID == os.Getenv("STRIPE_ACCOUNT_PRICE_ID") && item.Quantity > 0 {
				limit += item.Quantity
			}
		}
		_, e = tx.Exec(r.Context(), "UPDATE billing_accounts SET subscription_id=$1,status=$2,account_limit=$4,updated_at=now() WHERE customer_id=$3", object.ID, status, object.Customer, limit)
	case "invoice.paid":
		// Only a subscription cycle grants the monthly allowance, never proration invoices.
		if object.BillingReason == "subscription_cycle" || object.BillingReason == "subscription_create" {
			credits, _ := strconv.ParseInt(os.Getenv("MONTHLY_AI_CREDITS"), 10, 64)
			if credits > 0 {
				_, e = tx.Exec(r.Context(), "UPDATE billing_accounts SET credits=credits+$1,status=CASE WHEN status IN ('canceled','unpaid','incomplete_expired') THEN status ELSE 'active' END,updated_at=now() WHERE customer_id=$2", credits, object.Customer)
			}
		}
	case "invoice.payment_failed":
		_, e = tx.Exec(r.Context(), "UPDATE billing_accounts SET status='past_due',updated_at=now() WHERE customer_id=$1", object.Customer)
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		if object.PaymentStatus == "paid" && object.Metadata["kind"] == "topup" {
			// Both webhook event types can reference one checkout. A second semantic key prevents double crediting.
			grant, err := tx.Exec(r.Context(), "INSERT INTO stripe_events(id) VALUES($1) ON CONFLICT DO NOTHING", "topup:"+object.ID)
			e = err
			if err == nil && grant.RowsAffected() == 1 {
				credits, _ := strconv.ParseInt(os.Getenv("TOPUP_AI_CREDITS"), 10, 64)
				if credits > 0 {
					_, e = tx.Exec(r.Context(), "UPDATE billing_accounts SET credits=credits+$1,updated_at=now() WHERE customer_id=$2 AND workspace_id::text=$3", credits, object.Customer, object.Metadata["workspace_id"])
				}
			}
		}
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "Could not apply billing event")
		return
	}
	write(w, 200, map[string]bool{"received": true})
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
