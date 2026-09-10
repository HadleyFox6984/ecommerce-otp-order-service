package orders

import (
	"context"
	"errors"
	"testing"
)

type fakeSMS struct {
	verified bool
	err      error
}

func (f fakeSMS) SendOTP(context.Context, string, string) error { return f.err }
func (f fakeSMS) VerifyOTP(context.Context, string, string) (bool, error) {
	return f.verified, f.err
}

func TestVerifyAndLoadOrder(t *testing.T) {
	tests := []struct {
		name       string
		sms        fakeSMS
		wantErr    error
		wantEvents int
	}{
		{name: "verified customer sees order pipeline", sms: fakeSMS{verified: true}, wantEvents: 4},
		{name: "rejected code hides order pipeline", sms: fakeSMS{verified: false}, wantErr: ErrInvalidCode},
		{name: "provider error hides order pipeline", sms: fakeSMS{err: errors.New("transport error")}, wantErr: errors.New("any")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := NewLoginWorkflow(tt.sms)
			got, err := workflow.VerifyAndLoadOrder(context.Background(), "+15551234567", "123456", "cus_42", "ord_9001")
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected an error")
				}
				if len(got.Events) != 0 {
					t.Fatalf("order events leaked after rejected login: %v", got.Events)
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyAndLoadOrder() error = %v", err)
			}
			if len(got.Events) != tt.wantEvents {
				t.Fatalf("event count = %d, want %d", len(got.Events), tt.wantEvents)
			}
			if got.Events[0].Stage != StageCheckout || got.Events[3].Stage != StageUpdate {
				t.Fatalf("unexpected pipeline order: %v", got.Events)
			}
		})
	}
}
