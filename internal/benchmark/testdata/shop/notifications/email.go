package notifications

import "fmt"

// SendReceiptEmail notifies the customer after a successful charge.
func SendReceiptEmail(to, chargeID string, amount int) {
	fmt.Printf("mail %s: receipt %s for %d\n", to, chargeID, amount)
}
