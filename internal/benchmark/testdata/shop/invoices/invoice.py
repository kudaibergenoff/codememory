"""Invoice creation and discounts."""


def create_invoice(customer, items, discount_percent=0):
    """Create an invoice; a percentage discount is applied to the subtotal."""
    subtotal = sum(item["price"] * item["qty"] for item in items)
    return {"customer": customer, "total": apply_discount(subtotal, discount_percent)}


def apply_discount(amount, percent):
    if not 0 <= percent <= 100:
        raise ValueError("discount percentage must be between 0 and 100")
    return amount * (100 - percent) / 100
