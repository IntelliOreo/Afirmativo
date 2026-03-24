package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleCheckoutSessionStatus_IncludesZeroCouponCurrentUses(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		resolveCheckoutForPollFn: func(_ context.Context, _ string, _ time.Time, _ BuildFulfillmentFunc) (*PollResult, error) {
			return &PollResult{
				Payment: &Payment{
					ID:          "11111111-1111-1111-1111-111111111111",
					ProductType: ProductTypeCouponPack10,
					Status:      StatusProvisioned,
				},
				CouponCode:        "PACK10-ABCD2345",
				CouponMaxUses:     10,
				CouponCurrentUses: 0,
			}, nil
		},
	}

	h := NewHandler(NewService(Deps{Store: store}, Settings{}))
	req := httptest.NewRequest(http.MethodGet, "/api/payment/checkout-sessions/cs_test_123", nil)
	req.SetPathValue("id", "cs_test_123")
	rr := httptest.NewRecorder()

	h.HandleCheckoutSessionStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rr.Body.String())
	}
	if got["coupon_current_uses"] != float64(0) {
		t.Fatalf("coupon_current_uses = %#v, want 0", got["coupon_current_uses"])
	}
}
