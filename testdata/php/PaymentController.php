<?php
class PaymentController {
    public function refundPayment(string $id): array {
        return ['refund' => $id];
    }
}
