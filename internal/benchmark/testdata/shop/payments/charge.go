package payments

import "example.com/shop/notifications"

type Charge struct {
	ID       string
	Amount   int
	Refunded int
}

// ChargeCard captures a payment and, on success, sends the receipt email.
func ChargeCard(customerEmail string, amount int) (Receipt, error) {
	charge := Charge{ID: newID(), Amount: amount}
	receipt := Receipt{ChargeID: charge.ID, Amount: amount}
	notifications.SendReceiptEmail(customerEmail, receipt.ChargeID, amount)
	return receipt, nil
}

func loadCharge(id string) (*Charge, error) { return &Charge{ID: id}, nil }

func newID() string { return "ch_1" }
