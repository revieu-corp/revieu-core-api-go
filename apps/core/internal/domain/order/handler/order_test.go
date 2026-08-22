package handler

import (
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
)

func TestOrderResponseIncludesMerchantScanURL(t *testing.T) {
	h := NewOrderHandler(nil, "https://merchant.revieu.test")
	response, err := h.orderResponse(model.Order{}, []model.Voucher{{
		ID:        42,
		Code:      "ORDER-VOUCHER",
		ScanToken: "scan-token-order-response",
	}})
	if err != nil {
		t.Fatalf("order response returned error: %v", err)
	}

	if len(response.Vouchers) != 1 {
		t.Fatalf("expected one voucher response, got %d", len(response.Vouchers))
	}
	want := "https://merchant.revieu.test/merchant/vouchers/scan?t=scan-token-order-response"
	if response.Vouchers[0].ScanURL != want {
		t.Fatalf("unexpected scan_url: got %q want %q", response.Vouchers[0].ScanURL, want)
	}
}
