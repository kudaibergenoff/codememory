"""Stock reservation for orders."""

RESERVED = {}


def reserve_stock(order_id, sku, qty):
    RESERVED[order_id] = (sku, qty)


def release_stock(order_id):
    """Release reserved stock when an order is cancelled."""
    RESERVED.pop(order_id, None)
