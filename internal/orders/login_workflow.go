package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/infrai-examples/ecommerce-otp-order-service/internal/infrai"
)

var ErrInvalidCode = errors.New("phone code was not verified")

type Stage string

const (
	StageCheckout    Stage = "checkout_confirmed"
	StageFulfillment Stage = "fulfillment_queued"
	StageReceipt     Stage = "receipt_issued"
	StageUpdate      Stage = "customer_update_ready"
)

type OrderEvent struct {
	Stage Stage  `json:"stage"`
	Note  string `json:"note"`
}

type LoginResult struct {
	CustomerID string       `json:"customer_id"`
	OrderID    string       `json:"order_id"`
	Events     []OrderEvent `json:"events"`
}

type LoginWorkflow struct {
	sms infrai.SMSClient
}

func NewLoginWorkflow(sms infrai.SMSClient) *LoginWorkflow {
	return &LoginWorkflow{sms: sms}
}

func (w *LoginWorkflow) RequestCode(ctx context.Context, phone, requestID string) error {
	if phone == "" || requestID == "" {
		return errors.New("phone and request_id are required")
	}
	return w.sms.SendOTP(ctx, phone, requestID)
}

func (w *LoginWorkflow) VerifyAndLoadOrder(ctx context.Context, phone, code, customerID, orderID string) (LoginResult, error) {
	if phone == "" || code == "" || customerID == "" || orderID == "" {
		return LoginResult{}, errors.New("phone, code, customer_id, and order_id are required")
	}
	verified, err := w.sms.VerifyOTP(ctx, phone, code)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify login code: %w", err)
	}
	if !verified {
		return LoginResult{}, ErrInvalidCode
	}

	return LoginResult{
		CustomerID: customerID,
		OrderID:    orderID,
		Events: []OrderEvent{
			{Stage: StageCheckout, Note: "payment accepted"},
			{Stage: StageFulfillment, Note: "warehouse pick queued"},
			{Stage: StageReceipt, Note: "receipt attached to order"},
			{Stage: StageUpdate, Note: "shipment update available"},
		},
	}, nil
}
