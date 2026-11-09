package payments

// Receipt is the customer-facing record of a charge or refund.
type Receipt struct {
	ChargeID string
	Amount   int
}
