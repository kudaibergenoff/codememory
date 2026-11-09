package payments

import "errors"

// ErrRefundTooLarge is returned when a refund exceeds the captured charge.
var ErrRefundTooLarge = errors.New("refund exceeds the captured amount")

// RefundPayment reverses all or part of a captured charge.
// A partial refund is allowed while the total refunded stays within the limit.
func RefundPayment(chargeID string, amount int) (Receipt, error) {
	charge, err := loadCharge(chargeID)
	if err != nil {
		return Receipt{}, err
	}
	if charge.Refunded+amount > charge.Amount {
		return Receipt{}, ErrRefundTooLarge
	}
	charge.Refunded += amount
	return Receipt{ChargeID: chargeID, Amount: -amount}, nil
}
