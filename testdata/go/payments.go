package payments

type Receipt struct{ ID string }

// RefundPayment returns a receipt for a reversed payment.
func RefundPayment(id string) Receipt { return Receipt{ID: id} }
