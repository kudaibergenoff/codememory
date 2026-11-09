export function refundPayment(paymentId: string): { refund: string } {
  return { refund: paymentId };
}
